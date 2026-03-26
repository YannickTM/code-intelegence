//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/queue"
)

// indexEndpoint returns the POST /index URL for a project.
func indexEndpoint(projectID string) string {
	return fmt.Sprintf("/v1/projects/%s/index", projectID)
}

// jobsEndpoint returns the GET /jobs URL for a project.
func jobsEndpoint(projectID string) string {
	return fmt.Sprintf("/v1/projects/%s/jobs", projectID)
}

// setupProjectForIndexing creates a user, SSH key, and project ready for indexing.
// Returns (projectID, token).
func setupProjectForIndexing(t *testing.T, a *app.App, name string) (string, string) {
	t.Helper()
	registerUser(t, a, name)
	token := loginUser(t, a, name)
	sshKeyID := createSSHKey(t, a, name+"-key", token)
	proj := createProject(t, a, name+"-proj", "https://github.com/example/"+name+".git", sshKeyID, token)
	return mustString(t, proj, "id"), token
}

func TestIndexJob_CreateReturns202(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	m := decodeJSON(t, w)
	if id := mustString(t, m, "id"); id == "" || id == "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("expected real job ID, got %q", id)
	}
	if s := mustString(t, m, "status"); s != "queued" {
		t.Fatalf("status = %q, want queued", s)
	}
	if s := mustString(t, m, "job_type"); s != "full" {
		t.Fatalf("job_type = %q, want full", s)
	}
	if s := mustString(t, m, "project_id"); s != projectID {
		t.Fatalf("project_id = %q, want %q", s, projectID)
	}
}

func TestIndexJob_ConfigPinning(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "bob")

	// Get the resolved embedding config to compare.
	resolvedW := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/settings/embedding/resolved", projectID),
		nil, authHeader(token))
	if resolvedW.Code != http.StatusOK {
		t.Fatalf("resolved embedding status = %d (body=%s)", resolvedW.Code, resolvedW.Body.String())
	}
	resolved := decodeJSON(t, resolvedW)
	expectedEmbeddingID := mustString(t, mustValueMap(t, resolved["config"]), "id")

	// Create a job.
	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	m := decodeJSON(t, w)
	actualEmbeddingID, ok := m["embedding_provider_config_id"].(string)
	if !ok || actualEmbeddingID == "" {
		t.Fatalf("embedding_provider_config_id missing or empty in response: %v", m["embedding_provider_config_id"])
	}
	if actualEmbeddingID != expectedEmbeddingID {
		t.Fatalf("embedding_provider_config_id = %q, want %q", actualEmbeddingID, expectedEmbeddingID)
	}

	// LLM config is optional — check it's present (default bootstrap) or nil.
	// Default LLM bootstrap provides a config, so it should be non-nil.
	if m["llm_provider_config_id"] == nil {
		t.Log("llm_provider_config_id is nil (acceptable if no LLM configured)")
	}
}

func TestIndexJob_DedupQueued(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "carol")

	// First request.
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	m1 := decodeJSON(t, w1)
	id1 := mustString(t, m1, "id")

	// Second request — should return same job.
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	m2 := decodeJSON(t, w2)
	id2 := mustString(t, m2, "id")

	if id1 != id2 {
		t.Fatalf("dedup failed: id1=%q, id2=%q", id1, id2)
	}
}

func TestIndexJob_DedupRunning(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "dave")

	// Create job.
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("create: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	m1 := decodeJSON(t, w1)
	id1 := mustString(t, m1, "id")

	// Set job to running via DB.
	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE indexing_jobs SET status = 'running', started_at = NOW() WHERE id = $1", id1)
	if err != nil {
		t.Fatalf("set running: %v", err)
	}

	// Second request — should return same running job.
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	m2 := decodeJSON(t, w2)
	if mustString(t, m2, "id") != id1 {
		t.Fatalf("dedup running failed: got %q, want %q", mustString(t, m2, "id"), id1)
	}
	if mustString(t, m2, "status") != "running" {
		t.Fatalf("status = %q, want running", mustString(t, m2, "status"))
	}
}

func TestIndexJob_CompletedDoesNotBlock(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "eve")

	// Create and complete a job.
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("create: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	id1 := mustString(t, decodeJSON(t, w1), "id")

	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE indexing_jobs SET status = 'completed', finished_at = NOW() WHERE id = $1", id1)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// New request should create a different job.
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	id2 := mustString(t, decodeJSON(t, w2), "id")
	if id1 == id2 {
		t.Fatalf("expected new job ID after completion, got same: %q", id1)
	}
}

func TestIndexJob_FailedDoesNotBlock(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "frank")

	// Create and fail a job.
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("create: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	id1 := mustString(t, decodeJSON(t, w1), "id")

	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE indexing_jobs SET status = 'failed', finished_at = NOW() WHERE id = $1", id1)
	if err != nil {
		t.Fatalf("fail: %v", err)
	}

	// New request should create a different job.
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	id2 := mustString(t, decodeJSON(t, w2), "id")
	if id1 == id2 {
		t.Fatalf("expected new job ID after failure, got same: %q", id1)
	}
}

func TestIndexJob_DifferentProjectsCreateSeparateJobs(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projID1, token := setupProjectForIndexing(t, a, "grace")
	sshKeyID2 := createSSHKey(t, a, "grace-key2", token)
	proj2 := createProject(t, a, "grace-proj2", "https://github.com/example/grace2.git", sshKeyID2, token)
	projID2 := mustString(t, proj2, "id")

	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projID1), nil, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("proj1: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	id1 := mustString(t, decodeJSON(t, w1), "id")

	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projID2), nil, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("proj2: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	id2 := mustString(t, decodeJSON(t, w2), "id")

	if id1 == id2 {
		t.Fatalf("expected different job IDs for different projects, got same: %q", id1)
	}
}

func TestIndexJob_DifferentJobTypesCreateSeparateJobs(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "hank")

	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("full: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	id1 := mustString(t, decodeJSON(t, w1), "id")

	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "incremental",
	}, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("incremental: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	id2 := mustString(t, decodeJSON(t, w2), "id")

	if id1 == id2 {
		t.Fatalf("expected different job IDs for different job types, got same: %q", id1)
	}
}

func TestIndexJob_NoSSHKeyReturns409(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "ivan")
	token := loginUser(t, a, "ivan")
	// Create project without SSH key — create with one, then remove it.
	sshKeyID := createSSHKey(t, a, "ivan-key", token)
	proj := createProject(t, a, "ivan-proj", "https://github.com/example/ivan.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")

	// Remove SSH key from project.
	wDel := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), nil, authHeader(token))
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("remove ssh key: status = %d (body=%s)", wDel.Code, wDel.Body.String())
	}

	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}

	m := decodeJSON(t, w)
	if msg, ok := m["error"].(string); !ok || !contains(msg, "SSH key") {
		t.Fatalf("error = %v, want message about SSH key", m["error"])
	}
}

func TestIndexJob_NoEmbeddingConfigReturns409(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "judy")

	// Deactivate all embedding provider configs so resolution fails.
	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE embedding_provider_configs SET is_active = FALSE")
	if err != nil {
		t.Fatalf("deactivate embedding configs: %v", err)
	}

	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}

	m := decodeJSON(t, w)
	if msg, ok := m["error"].(string); !ok || !contains(msg, "embedding") {
		t.Fatalf("error = %v, want message about embedding provider", m["error"])
	}
}

func TestIndexJob_InvalidJobTypeReturns400(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "karl")

	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "invalid",
	}, authHeader(token))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestIndexJob_EmptyBodyDefaultsFull(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "laura")

	// Send with no body.
	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	m := decodeJSON(t, w)
	if s := mustString(t, m, "job_type"); s != "full" {
		t.Fatalf("job_type = %q, want full", s)
	}
}

func TestIndexJob_ListJobs(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "mike")

	// Create two jobs (full + incremental).
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("create full: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "incremental",
	}, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("create incremental: status = %d (body=%s)", w2.Code, w2.Body.String())
	}

	// List jobs.
	w := doRequest(t, a, http.MethodGet, jobsEndpoint(projectID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	if total := int(m["total"].(float64)); total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	items := mustItems(t, m)
	if len(items) != 2 {
		t.Fatalf("items count = %d, want 2", len(items))
	}

	// Verify ordered by created_at DESC (incremental created second should be first).
	first := mustValueMap(t, items[0])
	if mustString(t, first, "job_type") != "incremental" {
		t.Fatalf("first item job_type = %q, want incremental (DESC order)", mustString(t, first, "job_type"))
	}
}

func TestIndexJob_ListJobsPagination(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	projectID, token := setupProjectForIndexing(t, a, "nancy")

	// Create 3 jobs: full, then complete it, then full again, then incremental.
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{"job_type": "full"}, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("create 1: status = %d", w1.Code)
	}
	id1 := mustString(t, decodeJSON(t, w1), "id")
	// Complete first job to allow another full job.
	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE indexing_jobs SET status = 'completed', finished_at = NOW() WHERE id = $1", id1)
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}

	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{"job_type": "full"}, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("create 2: status = %d", w2.Code)
	}

	w3 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{"job_type": "incremental"}, authHeader(token))
	if w3.Code != http.StatusAccepted {
		t.Fatalf("create 3: status = %d", w3.Code)
	}

	// Paginate: limit=1, offset=0 — should return 1 item.
	w := doRequest(t, a, http.MethodGet, jobsEndpoint(projectID)+"?limit=1&offset=0", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list page1: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if total := int(m["total"].(float64)); total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	items := mustItems(t, m)
	if len(items) != 1 {
		t.Fatalf("page1 items = %d, want 1", len(items))
	}
	if lim := int(m["limit"].(float64)); lim != 1 {
		t.Fatalf("limit = %d, want 1", lim)
	}
	if off := int(m["offset"].(float64)); off != 0 {
		t.Fatalf("offset = %d, want 0", off)
	}

	// Paginate: limit=1, offset=1 — should return 1 different item.
	w = doRequest(t, a, http.MethodGet, jobsEndpoint(projectID)+"?limit=1&offset=1", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list page2: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m2 := decodeJSON(t, w)
	items2 := mustItems(t, m2)
	if len(items2) != 1 {
		t.Fatalf("page2 items = %d, want 1", len(items2))
	}

	// Verify different items.
	p1ID := mustString(t, mustValueMap(t, items[0]), "id")
	p2ID := mustString(t, mustValueMap(t, items2[0]), "id")
	if p1ID == p2ID {
		t.Fatalf("pagination returned same item on different pages: %q", p1ID)
	}
}

// --- Enqueue tests (Task 19) ---

// fakePublisher is a test double for queue.JobEnqueuer.
type fakePublisher struct {
	mu    sync.RWMutex
	calls atomic.Int64
	err   error // if non-nil, EnqueueIndexJob returns this
	last  struct {
		jobID       string
		workflow     string
		projectID   string
		requestedBy string
	}
}

func (f *fakePublisher) EnqueueIndexJob(_ context.Context, jobID, workflow, projectID, requestedBy string) error {
	f.calls.Add(1)
	f.mu.Lock()
	f.last.jobID = jobID
	f.last.workflow = workflow
	f.last.projectID = projectID
	f.last.requestedBy = requestedBy
	f.mu.Unlock()
	return f.err
}

// loadLast returns a snapshot of the last enqueue call's arguments.
func (f *fakePublisher) loadLast() (jobID, workflow, projectID, requestedBy string) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.last.jobID, f.last.workflow, f.last.projectID, f.last.requestedBy
}

func (f *fakePublisher) Close() error { return nil }

var _ queue.JobEnqueuer = (*fakePublisher)(nil)

// withPublisher sets a publisher on the test app for the duration of the test,
// restoring the previous value when the test finishes.
func withPublisher(t *testing.T, a *app.App, p queue.JobEnqueuer) {
	t.Helper()
	a.SetPublisher(p)
	t.Cleanup(func() { a.SetPublisher(nil) })
}

func TestIndexJob_EnqueueSuccess(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	pub := &fakePublisher{}
	withPublisher(t, a, pub)

	projectID, token := setupProjectForIndexing(t, a, "enq-ok")

	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	if pub.calls.Load() != 1 {
		t.Fatalf("enqueue calls = %d, want 1", pub.calls.Load())
	}
	m := decodeJSON(t, w)
	jobID := mustString(t, m, "id")
	gotJobID, gotWorkflow, gotProjectID, gotRequestedBy := pub.loadLast()
	if gotJobID != jobID {
		t.Fatalf("enqueued job_id = %q, want %q", gotJobID, jobID)
	}
	if gotWorkflow != "full-index" {
		t.Fatalf("workflow = %q, want full-index", gotWorkflow)
	}
	if gotProjectID != projectID {
		t.Fatalf("enqueued project_id = %q, want %q", gotProjectID, projectID)
	}
	if gotRequestedBy == "" {
		t.Fatal("requested_by is empty, expected user:uuid")
	}
}

func TestIndexJob_EnqueueWorkflowMapping(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	pub := &fakePublisher{}
	withPublisher(t, a, pub)

	projectID, token := setupProjectForIndexing(t, a, "enq-wf")

	// full → full-index
	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w.Code != http.StatusAccepted {
		t.Fatalf("full: status = %d (body=%s)", w.Code, w.Body.String())
	}
	if _, gotWF, _, _ := pub.loadLast(); gotWF != "full-index" {
		t.Fatalf("workflow = %q, want full-index", gotWF)
	}

	// incremental → incremental-index
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "incremental",
	}, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("incremental: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	if _, gotWF, _, _ := pub.loadLast(); gotWF != "incremental-index" {
		t.Fatalf("workflow = %q, want incremental-index", gotWF)
	}
}

func TestIndexJob_DedupDoesNotReenqueue(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	pub := &fakePublisher{}
	withPublisher(t, a, pub)

	projectID, token := setupProjectForIndexing(t, a, "enq-dedup")

	// First request enqueues.
	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first: status = %d (body=%s)", w1.Code, w1.Body.String())
	}
	if pub.calls.Load() != 1 {
		t.Fatalf("after first: enqueue calls = %d, want 1", pub.calls.Load())
	}

	// Second request (dedup hit) should NOT enqueue again.
	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second: status = %d (body=%s)", w2.Code, w2.Body.String())
	}
	if pub.calls.Load() != 1 {
		t.Fatalf("after dedup: enqueue calls = %d, want 1 (no re-enqueue)", pub.calls.Load())
	}
}

func TestIndexJob_EnqueueFailureMarksJobFailed(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	pub := &fakePublisher{err: errors.New("connection refused")}
	withPublisher(t, a, pub)

	projectID, token := setupProjectForIndexing(t, a, "enq-fail")

	w := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), map[string]any{
		"job_type": "full",
	}, authHeader(token))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	// Verify the job was created but then marked as failed.
	rows, err := a.DB.Pool.Query(context.Background(),
		"SELECT status, error_details FROM indexing_jobs WHERE project_id = $1 ORDER BY created_at DESC LIMIT 1",
		projectID)
	if err != nil {
		t.Fatalf("query job: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no job row found")
	}
	var status string
	var errDetailsRaw []byte
	if err := rows.Scan(&status, &errDetailsRaw); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if status != "failed" {
		t.Fatalf("job status = %q, want failed", status)
	}

	// Verify error_details contains enqueue stage info.
	var errDetails []map[string]string
	if err := json.Unmarshal(errDetailsRaw, &errDetails); err != nil {
		t.Fatalf("unmarshal error_details: %v (raw=%s)", err, errDetailsRaw)
	}
	if len(errDetails) == 0 {
		t.Fatal("error_details is empty")
	}
	if errDetails[0]["stage"] != "enqueue" {
		t.Fatalf("error_details[0].stage = %q, want enqueue", errDetails[0]["stage"])
	}
	if errDetails[0]["message"] == "" {
		t.Fatal("error_details[0].message is empty")
	}
}

func TestIndexJob_EnqueueFailureDoesNotBlockNewJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// First request fails enqueue.
	failPub := &fakePublisher{err: errors.New("redis down")}
	withPublisher(t, a, failPub)

	projectID, token := setupProjectForIndexing(t, a, "enq-retry")

	w1 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w1.Code != http.StatusInternalServerError {
		t.Fatalf("first: status = %d, want 500 (body=%s)", w1.Code, w1.Body.String())
	}

	// Now switch to a working publisher and retry.
	okPub := &fakePublisher{}
	a.SetPublisher(okPub)

	w2 := doRequest(t, a, http.MethodPost, indexEndpoint(projectID), nil, authHeader(token))
	if w2.Code != http.StatusAccepted {
		t.Fatalf("retry: status = %d, want %d (body=%s)", w2.Code, http.StatusAccepted, w2.Body.String())
	}
	if okPub.calls.Load() != 1 {
		t.Fatalf("retry enqueue calls = %d, want 1", okPub.calls.Load())
	}
	m := decodeJSON(t, w2)
	if mustString(t, m, "status") != "queued" {
		t.Fatalf("retry job status = %q, want queued", mustString(t, m, "status"))
	}
}
