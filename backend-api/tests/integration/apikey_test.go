//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"myjungle/backend-api/internal/auth"
)

// ---------- Project key tests ----------

func TestAPIKey_CreateProjectKey_Admin(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Owner creates a project key.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "ci-token", "role": "write"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	resp := decodeJSON(t, w)
	if resp["key_type"] != "project" {
		t.Errorf("key_type = %v, want project", resp["key_type"])
	}
	if resp["name"] != "ci-token" {
		t.Errorf("name = %v, want ci-token", resp["name"])
	}
	if resp["role"] != "write" {
		t.Errorf("role = %v, want write", resp["role"])
	}
	if resp["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", resp["project_id"], projID)
	}
	if _, ok := resp["plaintext_key"].(string); !ok {
		t.Error("plaintext_key should be a string")
	}
	if _, ok := resp["id"].(string); !ok {
		t.Error("id should be a string")
	}
	if _, ok := resp["key_prefix"].(string); !ok {
		t.Error("key_prefix should be a string")
	}
	if _, ok := resp["created_at"].(string); !ok {
		t.Error("created_at should be a string")
	}
}

func TestAPIKey_CreateProjectKey_MemberForbidden(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Add bob as member (not admin).
	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Member tries to create project key → 403.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "ci-token"},
		authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAPIKey_ListProjectKeys(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create two project keys.
	if resp := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "key-a"},
		authHeader(aliceToken)); resp.Code != http.StatusCreated {
		t.Fatalf("setup: create key-a got %d, want %d", resp.Code, http.StatusCreated)
	}
	if resp := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "key-b"},
		authHeader(aliceToken)); resp.Code != http.StatusCreated {
		t.Fatalf("setup: create key-b got %d, want %d", resp.Code, http.StatusCreated)
	}

	// Add bob as member (not admin).
	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Member lists project keys → 403 (admin required).
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}

	// Owner (alice) can still list keys → 200.
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("owner list: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	// Verify no plaintext_key in list.
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item[%d] is not map[string]any", i)
		}
		if _, has := m["plaintext_key"]; has {
			t.Errorf("item[%d] should not have plaintext_key", i)
		}
	}
}

func TestAPIKey_DeleteProjectKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create a project key.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "to-delete"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	apiKeyID := mustString(t, created, "id")

	// Delete it.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/keys/%s", projID, apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// List should now be empty.
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 after delete", len(items))
	}
}

func TestAPIKey_DeleteProjectKey_WrongProject(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)

	proj1 := createProject(t, a, "proj1", "git@github.com:org/repo1.git", keyID, aliceToken)
	proj1ID := mustString(t, proj1, "id")
	proj2 := createProject(t, a, "proj2", "git@github.com:org/repo2.git", keyID, aliceToken)
	proj2ID := mustString(t, proj2, "id")

	// Create a key in proj1.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", proj1ID),
		map[string]any{"name": "proj1-key"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	apiKeyID := mustString(t, created, "id")

	// Try to delete it via proj2 → 404.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/keys/%s", proj2ID, apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// ---------- Personal key tests ----------

func TestAPIKey_CreatePersonalKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "my-key", "role": "write"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	resp := decodeJSON(t, w)
	if resp["key_type"] != "personal" {
		t.Errorf("key_type = %v, want personal", resp["key_type"])
	}
	if resp["name"] != "my-key" {
		t.Errorf("name = %v, want my-key", resp["name"])
	}
	if resp["role"] != "write" {
		t.Errorf("role = %v, want write", resp["role"])
	}
	if _, ok := resp["plaintext_key"].(string); !ok {
		t.Error("plaintext_key should be a string")
	}
	if _, ok := resp["id"].(string); !ok {
		t.Error("id should be a string")
	}
}

func TestAPIKey_ListPersonalKeys(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	// Create two personal keys.
	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "key-1"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create key-1: status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	w = doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "key-2"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create key-2: status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	w = doRequest(t, a, http.MethodGet, "/v1/users/me/keys", nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
}

func TestAPIKey_DeletePersonalKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "to-delete"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	apiKeyID := mustString(t, created, "id")

	// Delete it.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/users/me/keys/%s", apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// List should now be empty.
	w = doRequest(t, a, http.MethodGet, "/v1/users/me/keys", nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 after delete", len(items))
	}
}

func TestAPIKey_DeletePersonalKey_OtherUser(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")

	// Alice creates a personal key.
	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "alice-key"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	apiKeyID := mustString(t, created, "id")

	// Bob tries to delete alice's key → 404.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/users/me/keys/%s", apiKeyID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// ---------- Validation tests ----------

func TestAPIKey_InvalidRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "bad", "role": "superadmin"},
		authHeader(aliceToken))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestAPIKey_ExpiredExpiresAt(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "bad", "expires_at": "2020-01-01T00:00:00Z"},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestAPIKey_NameTooLong(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	longName := strings.Repeat("a", 101)

	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": longName},
		authHeader(aliceToken))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestAPIKey_DefaultRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	// Omit role → defaults to "read".
	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "default-role"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "read" {
		t.Errorf("role = %v, want read", resp["role"])
	}
}

func TestAPIKey_PersonalKeysIsolated(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")

	// Alice creates a personal key.
	if resp := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "alice-key"},
		authHeader(aliceToken)); resp.Code != http.StatusCreated {
		t.Fatalf("setup: create alice-key got %d, want %d", resp.Code, http.StatusCreated)
	}

	// Bob lists personal keys — should see none.
	w := doRequest(t, a, http.MethodGet, "/v1/users/me/keys", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 (bob should not see alice's keys)", len(items))
	}
}

// ---------- Edge case tests ----------

func TestAPIKey_ListProjectKeys_NonMember(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create a key so there's something to list.
	if resp := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "ci-key"},
		authHeader(aliceToken)); resp.Code != http.StatusCreated {
		t.Fatalf("setup: create ci-key got %d, want %d", resp.Code, http.StatusCreated)
	}

	// Bob is not a member → 404 (middleware hides project existence).
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestAPIKey_CreateWithValidExpiresAt(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	// Create with a future expires_at.
	futureExpiry := time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)
	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "temp-key", "expires_at": futureExpiry},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	resp := decodeJSON(t, w)
	if resp["expires_at"] == nil {
		t.Error("expires_at should not be nil")
	}
}

func TestAPIKey_DeleteProjectKey_DoubleDelete(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create a project key.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "once"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	apiKeyID := mustString(t, created, "id")

	// First delete → 204.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/keys/%s", projID, apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("first delete: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Second delete → 404.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/keys/%s", projID, apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete: status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestAPIKey_DeletePersonalKey_DoubleDelete(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "once"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	apiKeyID := mustString(t, created, "id")

	// First delete → 204.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/users/me/keys/%s", apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("first delete: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Second delete → 404.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/users/me/keys/%s", apiKeyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete: status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestAPIKey_DeleteNonexistentKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	fakeID := "00000000-0000-0000-0000-000000000001"

	// Nonexistent project key → 404.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/keys/%s", projID, fakeID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("project key: status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}

	// Nonexistent personal key → 404.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/users/me/keys/%s", fakeID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("personal key: status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// ---------- Expired key excluded from list ----------

func TestAPIKey_ExpiredKey_ExcludedFromList(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create a project key.
	_, apiKey := createProjectAPIKey(t, a, projID, "soon-expired", "read", aliceToken)

	// Verify it appears in the list.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("list before: status = %d (body=%s)", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("list before: items is not an array: %T", body["items"])
	}
	if len(items) != 1 {
		t.Fatalf("list before: len = %d, want 1", len(items))
	}

	// Expire the key directly in DB.
	keyHash := auth.HashToken(apiKey)
	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE api_keys SET expires_at = NOW() - INTERVAL '1 hour' WHERE key_hash = $1", keyHash)
	if err != nil {
		t.Fatalf("expire key: %v", err)
	}

	// List should now be empty (expired keys are filtered out).
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("list after: status = %d (body=%s)", w.Code, w.Body.String())
	}
	body = decodeJSON(t, w)
	items, ok = body["items"].([]any)
	if !ok {
		t.Fatalf("list after: items is not an array: %T", body["items"])
	}
	if len(items) != 0 {
		t.Fatalf("list after expiry: len = %d, want 0", len(items))
	}
}

// ---------- Admin role rejected (regression test for 3→2 simplification) ----------

func TestAPIKey_AdminRole_Rejected(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Project key with role "admin" → 422.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "bad-key", "role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("project key: status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	// Personal key with role "admin" → 422.
	w = doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "bad-key", "role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("personal key: status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

// ---------- Malformed expires_at rejected ----------

func TestAPIKey_MalformedExpiresAt(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	// Non-date string → 400.
	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "bad", "expires_at": "not-a-date"},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Errorf("not-a-date: status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}

	// Numeric string → 400.
	w = doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "bad", "expires_at": "12345"},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Errorf("numeric: status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}

	// Empty string → should succeed (treated as no expiry).
	w = doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		map[string]any{"name": "no-expiry", "expires_at": ""},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Errorf("empty string: status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
}
