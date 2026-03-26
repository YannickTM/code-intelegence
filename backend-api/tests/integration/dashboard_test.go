//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// ----- Dashboard Summary Tests -----

func TestDashboard_Summary_EmptyState(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	assertInt(t, m, "projects_total", 0)
	assertInt(t, m, "jobs_active", 0)
	assertInt(t, m, "jobs_failed_24h", 0)
	assertInt(t, m, "query_count_24h", 0)
	assertInt(t, m, "p95_latency_ms_24h", 0)
}

func TestDashboard_Summary_Unauthenticated(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDashboard_Summary_WithActiveJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	// Insert a queued job.
	ctx := context.Background()
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status) VALUES ($1, 'full', 'queued')`,
		projID)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	assertInt(t, m, "projects_total", 1)
	assertInt(t, m, "jobs_active", 1)
	assertInt(t, m, "jobs_failed_24h", 0)
}

func TestDashboard_Summary_WithFailedJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, finished_at)
		 VALUES ($1, 'full', 'failed', NOW())`,
		projID)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	assertInt(t, m, "jobs_failed_24h", 1)
}

func TestDashboard_Summary_StaleFailedJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()
	stale := time.Now().Add(-25 * time.Hour)
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, finished_at)
		 VALUES ($1, 'full', 'failed', $2)`,
		projID, stale)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	assertInt(t, m, "jobs_failed_24h", 0)
}

func TestDashboard_Summary_QueryStats(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()
	for _, latency := range []int{10, 50, 100} {
		_, err := a.DB.Pool.Exec(ctx,
			`INSERT INTO query_log (project_id, tool_name, latency_ms) VALUES ($1, 'search', $2)`,
			projID, latency)
		if err != nil {
			t.Fatalf("insert query_log: %v", err)
		}
	}

	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	assertInt(t, m, "query_count_24h", 3)

	// p95 of [10, 50, 100] via percentile_cont interpolation = 95.
	p95 := int(m["p95_latency_ms_24h"].(float64))
	if p95 < 90 || p95 > 100 {
		t.Errorf("p95_latency_ms_24h = %d, want ~95 (between 90 and 100)", p95)
	}
}

func TestDashboard_Summary_MultiUserIsolation(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice: 1 project with a running job.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceKey := createSSHKey(t, a, "ak", aliceToken)
	aliceProj := createProject(t, a, "alice-proj", "git@github.com:alice/repo.git", aliceKey, aliceToken)
	aliceProjID := mustString(t, aliceProj, "id")

	ctx := context.Background()
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status) VALUES ($1, 'full', 'running')`,
		aliceProjID)
	if err != nil {
		t.Fatalf("insert alice job: %v", err)
	}

	// Bob: 1 project with a failed job.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobKey := createSSHKey(t, a, "bk", bobToken)
	bobProj := createProject(t, a, "bob-proj", "git@github.com:bob/repo.git", bobKey, bobToken)
	bobProjID := mustString(t, bobProj, "id")

	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, finished_at) VALUES ($1, 'full', 'failed', NOW())`,
		bobProjID)
	if err != nil {
		t.Fatalf("insert bob job: %v", err)
	}

	// Alice sees only her data.
	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("alice: status = %d (body=%s)", w.Code, w.Body.String())
	}
	am := decodeJSON(t, w)
	assertInt(t, am, "projects_total", 1)
	assertInt(t, am, "jobs_active", 1)
	assertInt(t, am, "jobs_failed_24h", 0)

	// Bob sees only his data.
	w = doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("bob: status = %d (body=%s)", w.Code, w.Body.String())
	}
	bm := decodeJSON(t, w)
	assertInt(t, bm, "projects_total", 1)
	assertInt(t, bm, "jobs_active", 0)
	assertInt(t, bm, "jobs_failed_24h", 1)
}

// ----- Project Health Fields Tests (via GET /v1/users/me/projects) -----

func TestDashboard_MyProjects_HealthFields_NeverIndexed(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)

	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := m["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}

	p := items[0].(map[string]any)

	// All health fields should be null.
	nullFields := []string{
		"index_git_commit", "index_branch", "index_activated_at",
		"active_job_id", "active_job_status",
		"failed_job_id", "failed_job_finished_at", "failed_job_type",
	}
	for _, f := range nullFields {
		v, exists := p[f]
		if !exists {
			t.Errorf("field %q missing from response", f)
		} else if v != nil {
			t.Errorf("field %q = %v, want nil", f, v)
		}
	}
}

func TestDashboard_MyProjects_HealthFields_WithActiveIndex(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	// Create the full snapshot chain: embedding_provider_configs → embedding_versions → index_snapshots.
	ctx := context.Background()

	var embConfigID, embVersionID string
	err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO embedding_provider_configs (name, provider, endpoint_url, model, dimensions, is_active, project_id)
		 VALUES ('Project Ollama', 'ollama', 'http://localhost:11434', 'jina/jina-embeddings-v2-base-en', 768, true, $1)
		 RETURNING id`, projID).Scan(&embConfigID)
	if err != nil {
		t.Fatalf("insert embedding_provider_configs: %v", err)
	}

	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO embedding_versions (embedding_provider_config_id, provider, model, dimensions, version_label)
		 VALUES ($1, 'ollama', 'jina/jina-embeddings-v2-base-en', 768, 'test-v1')
		 RETURNING id`, embConfigID).Scan(&embVersionID)
	if err != nil {
		t.Fatalf("insert embedding_versions: %v", err)
	}

	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO index_snapshots (project_id, branch, embedding_version_id, git_commit, is_active, status, activated_at)
		 VALUES ($1, 'main', $2, 'abc1234def', true, 'active', NOW())`,
		projID, embVersionID)
	if err != nil {
		t.Fatalf("insert index_snapshots: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := m["items"].([]any)
	p := items[0].(map[string]any)

	if p["index_git_commit"] != "abc1234def" {
		t.Errorf("index_git_commit = %v, want %q", p["index_git_commit"], "abc1234def")
	}
	if p["index_branch"] != "main" {
		t.Errorf("index_branch = %v, want %q", p["index_branch"], "main")
	}
	if p["index_activated_at"] == nil {
		t.Error("index_activated_at should not be nil")
	}
}

func TestDashboard_MyProjects_HealthFields_WithActiveJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status) VALUES ($1, 'incremental', 'running')`,
		projID)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := m["items"].([]any)
	p := items[0].(map[string]any)

	if p["active_job_id"] == nil {
		t.Error("active_job_id should not be nil when a running job exists")
	}
	if p["active_job_status"] != "running" {
		t.Errorf("active_job_status = %v, want %q", p["active_job_status"], "running")
	}
}

func TestDashboard_MyProjects_HealthFields_WithFailedJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, finished_at)
		 VALUES ($1, 'full', 'failed', NOW())`,
		projID)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := m["items"].([]any)
	p := items[0].(map[string]any)

	if p["failed_job_id"] == nil {
		t.Error("failed_job_id should not be nil")
	}
	if p["failed_job_finished_at"] == nil {
		t.Error("failed_job_finished_at should not be nil")
	}
	if p["failed_job_type"] != "full" {
		t.Errorf("failed_job_type = %v, want %q", p["failed_job_type"], "full")
	}
}

func TestDashboard_MyProjects_HealthFields_StaleFailedJob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()
	stale := time.Now().Add(-25 * time.Hour)
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, finished_at)
		 VALUES ($1, 'full', 'failed', $2)`,
		projID, stale)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := m["items"].([]any)
	p := items[0].(map[string]any)

	// Stale failed jobs (>24h) should not appear in health fields.
	staleFields := []string{"failed_job_id", "failed_job_finished_at", "failed_job_type"}
	for _, f := range staleFields {
		if p[f] != nil {
			t.Errorf("field %q = %v, want nil (failed job is >24h old)", f, p[f])
		}
	}
}

// ----- Test Helpers -----

func assertInt(t *testing.T, m map[string]any, key string, want int) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Errorf("response missing key %q", key)
		return
	}
	got, ok := v.(float64)
	if !ok {
		t.Errorf("%q = %T (%v), want number", key, v, v)
		return
	}
	if int(got) != want {
		t.Errorf("%q = %d, want %d", key, int(got), want)
	}
}
