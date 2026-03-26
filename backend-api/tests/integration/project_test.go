//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"myjungle/backend-api/internal/app"
)

// mustMap extracts a nested map from a JSON map or fails the test.
func mustMap(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("expected %q as map[string]any, got %T (%#v)", key, m[key], m[key])
	}
	return v
}

// mustString extracts a string value from a JSON map or fails the test.
func mustString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok {
		t.Fatalf("expected %q as string, got %T (%#v)", key, m[key], m[key])
	}
	return v
}

// createProject creates a project via the API, asserts 201, and returns the decoded response.
func createProject(t *testing.T, a *app.App, name, repoURL, sshKeyID, token string) map[string]any {
	t.Helper()
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       name,
		"repo_url":   repoURL,
		"ssh_key_id": sshKeyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create project %q: status = %d, want %d (body=%s)", name, w.Code, http.StatusCreated, w.Body.String())
	}
	return decodeJSON(t, w)
}

func TestProject_Create(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)

	if id, ok := m["id"].(string); !ok || id == "" {
		t.Error("id should be a non-empty string (UUID)")
	}
	if m["name"] != "my-project" {
		t.Errorf("name = %v, want %q", m["name"], "my-project")
	}
	if m["repo_url"] != "https://github.com/example/repo.git" {
		t.Errorf("repo_url = %v, want expected URL", m["repo_url"])
	}
	if m["default_branch"] != "main" {
		t.Errorf("default_branch = %v, want %q", m["default_branch"], "main")
	}
	if m["status"] != "active" {
		t.Errorf("status = %v, want %q", m["status"], "active")
	}
	if m["created_by"] == nil || m["created_by"] == "" {
		t.Error("created_by should be set")
	}

	// SSH key should be included in create response.
	sshKey, ok := m["ssh_key"].(map[string]any)
	if !ok {
		t.Fatal("ssh_key should be present in create response")
	}
	if sshKey["id"] != keyID {
		t.Errorf("ssh_key.id = %v, want %v", sshKey["id"], keyID)
	}
}

func TestProject_CreateWithCustomBranchAndStatus(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":           "custom-project",
		"repo_url":       "https://github.com/example/repo.git",
		"ssh_key_id":     keyID,
		"default_branch": "develop",
		"status":         "paused",
	}, authHeader(token))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["default_branch"] != "develop" {
		t.Errorf("default_branch = %v, want %q", m["default_branch"], "develop")
	}
	if m["status"] != "paused" {
		t.Errorf("status = %v, want %q", m["status"], "paused")
	}
}

func TestProject_CreateValidation(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Missing all required fields.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{}, authHeader(token))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	details, ok := m["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details in validation error")
	}
	if details["name"] == nil {
		t.Error("expected name validation error")
	}
	if details["repo_url"] == nil {
		t.Error("expected repo_url validation error")
	}
	if details["ssh_key_id"] == nil {
		t.Error("expected ssh_key_id validation error")
	}
}

func TestProject_CreateInvalidSSHKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": "00000000-0000-0000-0000-000000000000",
	}, authHeader(token))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}

	// Verify the transaction rolled back — no project should exist.
	w = doRequest(t, a, http.MethodGet, "/v1/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	data, ok := m["data"].([]any)
	if !ok {
		t.Fatalf("expected data as []any, got %T", m["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected 0 projects after rollback, got %d", len(data))
	}
}

func TestProject_CreateUnauthenticated(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": "00000000-0000-0000-0000-000000000000",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestProject_List(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates 3 projects.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceKeyID := createSSHKey(t, a, "alice-key", aliceToken)

	for i := 0; i < 3; i++ {
		w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
			"name":       fmt.Sprintf("alice-project-%d", i),
			"repo_url":   fmt.Sprintf("https://github.com/alice/repo-%d.git", i),
			"ssh_key_id": aliceKeyID,
		}, authHeader(aliceToken))
		if w.Code != http.StatusCreated {
			t.Fatalf("create alice-project-%d: status = %d (body=%s)", i, w.Code, w.Body.String())
		}
	}

	// Bob creates 2 projects — Alice must not see these.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobKeyID := createSSHKey(t, a, "bob-key", bobToken)

	for i := 0; i < 2; i++ {
		w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
			"name":       fmt.Sprintf("bob-project-%d", i),
			"repo_url":   fmt.Sprintf("https://github.com/bob/repo-%d.git", i),
			"ssh_key_id": bobKeyID,
		}, authHeader(bobToken))
		if w.Code != http.StatusCreated {
			t.Fatalf("create bob-project-%d: status = %d (body=%s)", i, w.Code, w.Body.String())
		}
	}

	// Alice should only see her 3 projects.
	w := doRequest(t, a, http.MethodGet, "/v1/projects", nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("alice list: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	data, ok := m["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 3 {
		t.Errorf("alice data length = %d, want 3", len(data))
	}
	total, ok := m["total"].(float64)
	if !ok || int(total) != 3 {
		t.Errorf("alice total = %v, want 3", m["total"])
	}

	// Bob should only see his 2 projects.
	w = doRequest(t, a, http.MethodGet, "/v1/projects", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("bob list: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m = decodeJSON(t, w)
	data, ok = m["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 2 {
		t.Errorf("bob data length = %d, want 2", len(data))
	}
	total, ok = m["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("bob total = %v, want 2", m["total"])
	}
}

func TestProject_ListPagination(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create 5 projects.
	for i := 0; i < 5; i++ {
		w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
			"name":       fmt.Sprintf("project-%d", i),
			"repo_url":   fmt.Sprintf("https://github.com/example/repo-%d.git", i),
			"ssh_key_id": keyID,
		}, authHeader(token))
		if w.Code != http.StatusCreated {
			t.Fatalf("create project-%d: status = %d (body=%s)", i, w.Code, w.Body.String())
		}
	}

	// First page: limit=2, offset=0.
	w := doRequest(t, a, http.MethodGet, "/v1/projects?limit=2&offset=0", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	data, ok := m["data"].([]any)
	if !ok {
		t.Fatalf("expected data as []any, got %T", m["data"])
	}
	if len(data) != 2 {
		t.Errorf("page 1: data length = %d, want 2", len(data))
	}
	totalF, ok := m["total"].(float64)
	if !ok {
		t.Fatalf("expected total as float64, got %T", m["total"])
	}
	if int(totalF) != 5 {
		t.Errorf("total = %v, want 5", m["total"])
	}

	// Second page: limit=2, offset=2.
	w = doRequest(t, a, http.MethodGet, "/v1/projects?limit=2&offset=2", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	data, ok = m["data"].([]any)
	if !ok {
		t.Fatalf("expected data as []any, got %T", m["data"])
	}
	if len(data) != 2 {
		t.Errorf("page 2: data length = %d, want 2", len(data))
	}
}

func TestProject_Get(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Get project.
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projectID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["id"] != projectID {
		t.Errorf("id = %v, want %v", m["id"], projectID)
	}
	if m["name"] != "my-project" {
		t.Errorf("name = %v, want %q", m["name"], "my-project")
	}
}

func TestProject_GetNotFound(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Since project doesn't exist, RBAC middleware returns 404 (hides existence per ADR-008).
	w := doRequest(t, a, http.MethodGet, "/v1/projects/00000000-0000-0000-0000-000000000000", nil, authHeader(token))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestProject_Update(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Partial update — name only.
	w = doRequest(t, a, http.MethodPatch, fmt.Sprintf("/v1/projects/%s", projectID), map[string]any{
		"name": "updated-project",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["name"] != "updated-project" {
		t.Errorf("name = %v, want %q", m["name"], "updated-project")
	}
	// Other fields should remain unchanged.
	if m["repo_url"] != "https://github.com/example/repo.git" {
		t.Errorf("repo_url = %v, want original URL", m["repo_url"])
	}
	if m["default_branch"] != "main" {
		t.Errorf("default_branch = %v, want %q", m["default_branch"], "main")
	}
}

func TestProject_UpdateFullFields(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Full update.
	w = doRequest(t, a, http.MethodPatch, fmt.Sprintf("/v1/projects/%s", projectID), map[string]any{
		"name":           "renamed",
		"repo_url":       "https://github.com/example/new-repo.git",
		"default_branch": "develop",
		"status":         "paused",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["name"] != "renamed" {
		t.Errorf("name = %v, want %q", m["name"], "renamed")
	}
	if m["repo_url"] != "https://github.com/example/new-repo.git" {
		t.Errorf("repo_url = %v, want new URL", m["repo_url"])
	}
	if m["default_branch"] != "develop" {
		t.Errorf("default_branch = %v, want %q", m["default_branch"], "develop")
	}
	if m["status"] != "paused" {
		t.Errorf("status = %v, want %q", m["status"], "paused")
	}
}

func TestProject_Delete(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Delete project.
	w = doRequest(t, a, http.MethodDelete, fmt.Sprintf("/v1/projects/%s", projectID), nil, authHeader(token))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify project is gone (RBAC middleware returns 404 per ADR-008).
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projectID), nil, authHeader(token))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status after delete = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestProject_GetSSHKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project with SSH key.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Get SSH key.
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["id"] != keyID {
		t.Errorf("id = %v, want %v", m["id"], keyID)
	}
	if m["name"] != "deploy-key" {
		t.Errorf("name = %v, want %q", m["name"], "deploy-key")
	}
	if m["fingerprint"] == nil || m["fingerprint"] == "" {
		t.Error("fingerprint should be set")
	}
	if m["public_key"] == nil || m["public_key"] == "" {
		t.Error("public_key should be set")
	}
	if m["key_type"] == nil || m["key_type"] == "" {
		t.Error("key_type should be set")
	}
}

func TestProject_SetSSHKey_Reassign(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID1 := createSSHKey(t, a, "deploy-key-1", token)
	keyID2 := createSSHKey(t, a, "deploy-key-2", token)

	// Create project with key 1.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID1,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Reassign to key 2.
	w = doRequest(t, a, http.MethodPut, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), map[string]any{
		"ssh_key_id": keyID2,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["id"] != keyID2 {
		t.Errorf("ssh key id = %v, want %v", m["id"], keyID2)
	}

	// Verify via GET.
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("get ssh key: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["id"] != keyID2 {
		t.Errorf("get ssh key: id = %v, want %v", m["id"], keyID2)
	}
}

func TestProject_SetSSHKey_Generate(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project with initial key.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Generate and assign new key.
	w = doRequest(t, a, http.MethodPut, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), map[string]any{
		"generate": true,
		"name":     "auto-generated-key",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	// Should be a new key, not the original.
	if m["id"] == keyID {
		t.Error("expected a new key ID, got original")
	}
	if m["name"] != "auto-generated-key" {
		t.Errorf("name = %v, want %q", m["name"], "auto-generated-key")
	}
}

func TestProject_SetSSHKey_MutualExclusion(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Providing both generate=true and ssh_key_id must fail.
	w = doRequest(t, a, http.MethodPut, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), map[string]any{
		"generate":   true,
		"name":       "new-key",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestProject_RemoveSSHKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Create project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "my-project",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	projectID := mustString(t, created, "id")

	// Remove SSH key assignment.
	w = doRequest(t, a, http.MethodDelete, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), nil, authHeader(token))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify no key is assigned.
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), nil, authHeader(token))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- Additional validation and edge-case tests ---

func TestProject_CreateWithSCPStyleRepoURL(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "scp-project",
		"repo_url":   "git@github.com:org/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["repo_url"] != "git@github.com:org/repo.git" {
		t.Errorf("repo_url = %v, want SCP-style URL", m["repo_url"])
	}
}

func TestProject_CreateEmptyName(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "",
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	details := mustMap(t, m, "details")
	if details["name"] == nil {
		t.Error("expected name validation error")
	}
}

func TestProject_CreateNameTooLong(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	longName := strings.Repeat("a", 101)
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       longName,
		"repo_url":   "https://github.com/example/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	details := mustMap(t, m, "details")
	if details["name"] == nil {
		t.Error("expected name validation error for exceeding 100 chars")
	}
}

func TestProject_CreateInvalidRepoURL(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "bad-url-project",
		"repo_url":   "not-a-url",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	details := mustMap(t, m, "details")
	if details["repo_url"] == nil {
		t.Error("expected repo_url validation error")
	}
}

func TestProject_CreateInvalidRepoURLScheme(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// ftp:// is not an allowed repo URL scheme.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "ftp-project",
		"repo_url":   "ftp://example.com/repo.git",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestProject_CreateInvalidSCPRepoURL(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	// Malformed SCP URL with spaces/special chars must be rejected.
	w := doRequest(t, a, http.MethodPost, "/v1/projects", map[string]any{
		"name":       "bad-scp-project",
		"repo_url":   "git@ injected :org/repo",
		"ssh_key_id": keyID,
	}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestProject_UpdateInvalidStatus(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	proj := createProject(t, a, "my-project", "https://github.com/example/repo.git", keyID, token)
	projectID := mustString(t, proj, "id")

	w := doRequest(t, a, http.MethodPatch, fmt.Sprintf("/v1/projects/%s", projectID), map[string]any{
		"status": "deleted",
	}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	details := mustMap(t, m, "details")
	if details["status"] == nil {
		t.Error("expected status validation error for invalid value")
	}
}

func TestProject_UpdateEmptyBody(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	proj := createProject(t, a, "my-project", "https://github.com/example/repo.git", keyID, token)
	projectID := mustString(t, proj, "id")

	// Empty PATCH body — should be a no-op and succeed.
	w := doRequest(t, a, http.MethodPatch, fmt.Sprintf("/v1/projects/%s", projectID), map[string]any{}, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["name"] != "my-project" {
		t.Errorf("name = %v, want %q (should be unchanged)", m["name"], "my-project")
	}
}

func TestProject_ListPaginationCap(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	createProject(t, a, "proj-1", "https://github.com/example/repo-1.git", keyID, token)

	// limit=300 should be capped to 200.
	w := doRequest(t, a, http.MethodGet, "/v1/projects?limit=300", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	limitF, ok := m["limit"].(float64)
	if !ok {
		t.Fatalf("expected limit as float64, got %T", m["limit"])
	}
	if int(limitF) != 200 {
		t.Errorf("limit = %v, want 200 (should be capped)", m["limit"])
	}
}

func TestProject_SetSSHKey_CrossUser(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceKeyID := createSSHKey(t, a, "alice-key", aliceToken)
	proj := createProject(t, a, "alice-project", "https://github.com/example/repo.git", aliceKeyID, aliceToken)
	projectID := mustString(t, proj, "id")

	// Bob creates an SSH key in his own library.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobKeyID := createSSHKey(t, a, "bob-key", bobToken)

	// Alice tries to reassign the project to Bob's key — should fail.
	// The key doesn't belong to Alice (created_by != alice), so GetSSHKey returns not found.
	w := doRequest(t, a, http.MethodPut, fmt.Sprintf("/v1/projects/%s/ssh-key", projectID), map[string]any{
		"ssh_key_id": bobKeyID,
	}, authHeader(aliceToken))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d — should not allow cross-user key assignment (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- Health fields tests (Task 14) ---

func TestProject_Get_HealthFields_NeverIndexed(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)

	// Base fields must be present.
	if m["id"] != projID {
		t.Errorf("id = %v, want %v", m["id"], projID)
	}
	if m["name"] != "proj1" {
		t.Errorf("name = %v, want %q", m["name"], "proj1")
	}

	// All health fields should be null for a never-indexed project.
	nullFields := []string{
		"index_git_commit", "index_branch", "index_activated_at",
		"active_job_id", "active_job_status",
		"failed_job_id", "failed_job_finished_at", "failed_job_type",
	}
	for _, f := range nullFields {
		v, exists := m[f]
		if !exists {
			t.Errorf("field %q missing from response", f)
		} else if v != nil {
			t.Errorf("field %q = %v, want nil", f, v)
		}
	}
}

func TestProject_Get_HealthFields_WithActiveIndex(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

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

	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["index_git_commit"] != "abc1234def" {
		t.Errorf("index_git_commit = %v, want %q", m["index_git_commit"], "abc1234def")
	}
	if m["index_branch"] != "main" {
		t.Errorf("index_branch = %v, want %q", m["index_branch"], "main")
	}
	if m["index_activated_at"] == nil {
		t.Error("index_activated_at should not be nil")
	}
}

func TestProject_Get_HealthFields_WithActiveJob(t *testing.T) {
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

	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["active_job_id"] == nil {
		t.Error("active_job_id should not be nil when a running job exists")
	}
	if m["active_job_status"] != "running" {
		t.Errorf("active_job_status = %v, want %q", m["active_job_status"], "running")
	}
}

func TestProject_Get_HealthFields_WithRecentFailedJob(t *testing.T) {
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

	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["failed_job_id"] == nil {
		t.Error("failed_job_id should not be nil")
	}
	if m["failed_job_finished_at"] == nil {
		t.Error("failed_job_finished_at should not be nil")
	}
	if m["failed_job_type"] != "full" {
		t.Errorf("failed_job_type = %v, want %q", m["failed_job_type"], "full")
	}
}

func TestProject_Get_HealthFields_StaleFailedJob(t *testing.T) {
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

	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)

	// Stale failed jobs (>24h) should not appear in health fields.
	staleFields := []string{"failed_job_id", "failed_job_finished_at", "failed_job_type"}
	for _, f := range staleFields {
		if m[f] != nil {
			t.Errorf("field %q = %v, want nil (job is >24h old)", f, m[f])
		}
	}
}

// TestProject_Get_HealthFields_LatestRowWins verifies that when multiple active
// snapshots or running jobs exist, the LATERAL subqueries return the most recent
// row (ORDER BY … DESC, id DESC LIMIT 1).
func TestProject_Get_HealthFields_LatestRowWins(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", token)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ctx := context.Background()

	// -- Create embedding prerequisites for snapshots --
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

	// -- Insert two active snapshots with distinct activated_at --
	olderTime := time.Now().Add(-2 * time.Hour)
	newerTime := time.Now().Add(-1 * time.Hour)

	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO index_snapshots (project_id, branch, embedding_version_id, git_commit, is_active, status, activated_at)
		 VALUES ($1, 'main', $2, 'older_commit', true, 'active', $3)`,
		projID, embVersionID, olderTime)
	if err != nil {
		t.Fatalf("insert older snapshot: %v", err)
	}

	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO index_snapshots (project_id, branch, embedding_version_id, git_commit, is_active, status, activated_at)
		 VALUES ($1, 'develop', $2, 'newer_commit', true, 'active', $3)`,
		projID, embVersionID, newerTime)
	if err != nil {
		t.Fatalf("insert newer snapshot: %v", err)
	}

	// -- Insert two running jobs with distinct created_at --
	var olderJobID string
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, created_at)
		 VALUES ($1, 'full', 'running', $2)
		 RETURNING id`, projID, olderTime).Scan(&olderJobID)
	if err != nil {
		t.Fatalf("insert older job: %v", err)
	}

	var newerJobID string
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO indexing_jobs (project_id, job_type, status, created_at)
		 VALUES ($1, 'incremental', 'running', $2)
		 RETURNING id`, projID, newerTime).Scan(&newerJobID)
	if err != nil {
		t.Fatalf("insert newer job: %v", err)
	}

	// -- Fetch and verify --
	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)

	// Snapshot: the newer one should win.
	if m["index_git_commit"] != "newer_commit" {
		t.Errorf("index_git_commit = %v, want %q (latest snapshot)", m["index_git_commit"], "newer_commit")
	}
	if m["index_branch"] != "develop" {
		t.Errorf("index_branch = %v, want %q (latest snapshot)", m["index_branch"], "develop")
	}

	// Active job: the newer one should win.
	if m["active_job_id"] != newerJobID {
		t.Errorf("active_job_id = %v, want %q (latest job)", m["active_job_id"], newerJobID)
	}
}
