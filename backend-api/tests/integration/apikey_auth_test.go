//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"

	db "myjungle/datastore/postgres/sqlc"
)

// ---------- Helpers ----------

// createProjectAPIKey creates a project API key via the API and returns
// (key_id, plaintext_key).
func createProjectAPIKey(t *testing.T, a *app.App, projID, name, role, token string) (string, string) {
	t.Helper()
	body := map[string]any{"name": name}
	if role != "" {
		body["role"] = role
	}
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		body, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("createProjectAPIKey(%s): status=%d, want %d (body=%s)", name, w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeJSON(t, w)
	return mustString(t, resp, "id"), mustString(t, resp, "plaintext_key")
}

// createPersonalAPIKey creates a personal API key via the API and returns
// (key_id, plaintext_key).
func createPersonalAPIKey(t *testing.T, a *app.App, name, role, token string) (string, string) {
	t.Helper()
	body := map[string]any{"name": name}
	if role != "" {
		body["role"] = role
	}
	w := doRequest(t, a, http.MethodPost, "/v1/users/me/keys",
		body, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("createPersonalAPIKey(%s): status=%d, want %d (body=%s)", name, w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeJSON(t, w)
	return mustString(t, resp, "id"), mustString(t, resp, "plaintext_key")
}

// deactivateAPIKey soft-deletes an API key directly in the database.
func deactivateAPIKey(t *testing.T, a *app.App, plaintextKey string) {
	t.Helper()
	keyHash := auth.HashToken(plaintextKey)
	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE api_keys SET is_active = false WHERE key_hash = $1", keyHash)
	if err != nil {
		t.Fatalf("deactivateAPIKey: %v", err)
	}
}

// ---------- Project key: access own project ----------

func TestAPIKeyAuth_ProjectKey_AccessOwnProject(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Setup: alice creates a project and a project API key.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// The project key should be able to GET its own project.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["id"] != projID {
		t.Errorf("project id = %v, want %s", body["id"], projID)
	}
}

// ---------- Project key: cannot access other project ----------

func TestAPIKeyAuth_ProjectKey_CannotAccessOtherProject(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)

	proj1 := createProject(t, a, "proj1", "git@github.com:org/repo1.git", sshKeyID, aliceToken)
	proj1ID := mustString(t, proj1, "id")
	proj2 := createProject(t, a, "proj2", "git@github.com:org/repo2.git", sshKeyID, aliceToken)
	proj2ID := mustString(t, proj2, "id")

	// Key is scoped to proj1.
	_, apiKey := createProjectAPIKey(t, a, proj1ID, "ci-key", "read", aliceToken)

	// Accessing proj2 with proj1's key → 404 (project not found / no access).
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", proj2ID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// ---------- Personal key: access member projects ----------

func TestAPIKeyAuth_PersonalKey_AccessMemberProjects(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates two projects and a personal API key.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)

	proj1 := createProject(t, a, "proj1", "git@github.com:org/repo1.git", sshKeyID, aliceToken)
	proj1ID := mustString(t, proj1, "id")
	proj2 := createProject(t, a, "proj2", "git@github.com:org/repo2.git", sshKeyID, aliceToken)
	proj2ID := mustString(t, proj2, "id")

	_, apiKey := createPersonalAPIKey(t, a, "my-key", "write", aliceToken)

	// Personal key can access both projects.
	for _, pid := range []string{proj1ID, proj2ID} {
		w := doRequest(t, a, http.MethodGet,
			fmt.Sprintf("/v1/projects/%s", pid),
			nil, authHeader(apiKey))
		if w.Code != http.StatusOK {
			t.Errorf("GET /projects/%s: status = %d, want %d (body=%s)", pid, w.Code, http.StatusOK, w.Body.String())
		}
	}
}

// ---------- Personal key: cannot access non-member project ----------

func TestAPIKeyAuth_PersonalKey_CannotAccessNonMemberProject(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Bob creates a personal key but is NOT a member of alice's project.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	_, bobKey := createPersonalAPIKey(t, a, "bob-key", "write", bobToken)

	// Bob's personal key cannot access alice's project → 404.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(bobKey))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// ---------- API key: cannot access settings endpoint ----------

func TestAPIKeyAuth_Key_CannotAccessSettingsEndpoint(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Project key.
	_, projKey := createProjectAPIKey(t, a, projID, "ci-key", "write", aliceToken)

	// Personal key.
	_, persKey := createPersonalAPIKey(t, a, "my-key", "write", aliceToken)

	for _, tc := range []struct {
		name string
		key  string
	}{
		{"project_key", projKey},
		{"personal_key", persKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Platform settings routes require a real user session before role checks,
			// so API keys are rejected with 401.
			w := doRequest(t, a, http.MethodGet, "/v1/platform-management/settings/embedding", nil, authHeader(tc.key))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("GET /settings/embedding: status = %d, want %d (body=%s)",
					w.Code, http.StatusUnauthorized, w.Body.String())
			}
		})
	}
}

// ---------- API key: cannot access user profile ----------

func TestAPIKeyAuth_Key_CannotAccessUserProfile(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	_, persKey := createPersonalAPIKey(t, a, "my-key", "write", aliceToken)

	// User profile endpoints are user-only → 401 for API keys.
	for _, path := range []string{"/v1/users/me", "/v1/users/me/projects"} {
		w := doRequest(t, a, http.MethodGet, path, nil, authHeader(persKey))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: status = %d, want %d (body=%s)",
				path, w.Code, http.StatusUnauthorized, w.Body.String())
		}
	}
}

// ---------- API key: cannot manage project keys (user-only) ----------

func TestAPIKeyAuth_Key_CannotManageKeys(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, projKey := createProjectAPIKey(t, a, projID, "ci-key", "write", aliceToken)

	// Listing project keys via API key → 401 (user-only route).
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		nil, authHeader(projKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /keys: status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}

	// Creating a project key via API key → 401.
	w = doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/keys", projID),
		map[string]any{"name": "from-api-key"},
		authHeader(projKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /keys: status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- Project key: role enforcement ----------

func TestAPIKeyAuth_ProjectKey_RoleEnforcement(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create a read-only project key.
	_, readKey := createProjectAPIKey(t, a, projID, "read-key", "read", aliceToken)

	// read-key can GET project (member-level → read).
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(readKey))
	if w.Code != http.StatusOK {
		t.Fatalf("GET project: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	// read-key cannot POST /index (admin-level → write required) → 403.
	w = doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/index", projID),
		map[string]any{},
		authHeader(readKey))
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST /index: status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

// ---------- Personal key: role capping by membership ----------

func TestAPIKeyAuth_PersonalKey_RoleCapping(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Bob is added as a member (lowest role).
	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Bob creates a write-level personal key.
	_, bobKey := createPersonalAPIKey(t, a, "bob-write-key", "write", bobToken)

	// Bob's personal key can GET project (member→read, write≥read → OK).
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(bobKey))
	if w.Code != http.StatusOK {
		t.Fatalf("GET project: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	// Bob's personal key cannot POST /index because effective role is
	// min(write_key, member_membership) = read, but admin route requires write → 403.
	w = doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/index", projID),
		map[string]any{},
		authHeader(bobKey))
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST /index: status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

// ---------- Deactivated key: rejected ----------

func TestAPIKeyAuth_DeactivatedKey_Rejected(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// Verify key works before deactivation.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusOK {
		t.Fatalf("pre-deactivation: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Deactivate key directly in DB.
	deactivateAPIKey(t, a, apiKey)

	// Deactivated key → 401.
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("post-deactivation: status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- Invalid key: rejected ----------

func TestAPIKeyAuth_InvalidKey_Rejected(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Random mj_ prefixed key that doesn't exist → 401.
	fakeKey := "mj_proj_00000000000000000000000000000000"
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(fakeKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- API key: dashboard & events are user-only ----------

func TestAPIKeyAuth_Key_CannotAccessDashboard(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")

	_, persKey := createPersonalAPIKey(t, a, "my-key", "write", aliceToken)

	// Dashboard is user-only → 401.
	w := doRequest(t, a, http.MethodGet, "/v1/dashboard/summary", nil, authHeader(persKey))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /dashboard/summary: status = %d, want %d (body=%s)",
			w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- API key: data routes (multiple endpoints) ----------

func TestAPIKeyAuth_ProjectKey_DataRoutes(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// All these data routes should be accessible by a project API key.
	dataRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/ssh-key", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/members", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/jobs", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/symbols", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/dependencies", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/structure", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/files/context", projID)},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/conventions", projID)},
	}

	for _, tc := range dataRoutes {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			w := doRequest(t, a, tc.method, tc.path, nil, authHeader(apiKey))
			// We accept 200, 404 (no data yet), or any non-auth error.
			// The key should NOT get 401/403 or 5xx.
			if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
				t.Errorf("status = %d, should not be auth error (body=%s)", w.Code, w.Body.String())
			}
			if w.Code >= 500 {
				t.Errorf("status = %d, unexpected server error (body=%s)", w.Code, w.Body.String())
			}
		})
	}
}

// ---------- API key: user-only management routes are blocked ----------

func TestAPIKeyAuth_Key_UserOnlyManagementRoutes(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "write", aliceToken)

	// All these management routes should be blocked for API keys.
	userOnlyRoutes := []struct {
		method string
		path   string
		body   map[string]any
	}{
		{http.MethodPatch, fmt.Sprintf("/v1/projects/%s", projID), map[string]any{"name": "new-name"}},
		{http.MethodPut, fmt.Sprintf("/v1/projects/%s/ssh-key", projID), map[string]any{"ssh_key_id": sshKeyID}},
		{http.MethodDelete, fmt.Sprintf("/v1/projects/%s/ssh-key", projID), nil},
		{http.MethodPost, fmt.Sprintf("/v1/projects/%s/members", projID), map[string]any{"username": "bob"}},
		{http.MethodGet, fmt.Sprintf("/v1/projects/%s/keys", projID), nil},
		{http.MethodPost, fmt.Sprintf("/v1/projects/%s/keys", projID), map[string]any{"name": "new-key"}},
	}

	for _, tc := range userOnlyRoutes {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			w := doRequest(t, a, tc.method, tc.path, tc.body, authHeader(apiKey))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
			}
		})
	}
}

// ---------- Personal key: access via membership (added as member to another user's project) ----------

func TestAPIKeyAuth_PersonalKey_AccessViaMembership(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Bob is added as an admin member.
	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Bob creates a write-level personal key.
	_, bobKey := createPersonalAPIKey(t, a, "bob-key", "write", bobToken)

	// Bob's personal key can access alice's project (bob is admin member,
	// key role is write, effective = min(write, admin→write) = write).
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(bobKey))
	if w.Code != http.StatusOK {
		t.Fatalf("GET project: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	// Bob's personal key can also POST /index (write ≥ write required).
	w = doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/index", projID),
		map[string]any{},
		authHeader(bobKey))
	// We accept non-auth status (the handler may return 4xx for missing body etc.,
	// but not 401/403 or 5xx).
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("POST /index: status = %d, should not be auth error (body=%s)", w.Code, w.Body.String())
	}
	if w.Code >= 500 {
		t.Fatalf("POST /index: status = %d, unexpected server error (body=%s)", w.Code, w.Body.String())
	}
}

// ---------- Write-role project key can trigger index ----------

func TestAPIKeyAuth_WriteKey_CanIndex(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Write-role key should be able to POST /index (admin route → write key role).
	_, writeKey := createProjectAPIKey(t, a, projID, "write-key", "write", aliceToken)

	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/index", projID),
		map[string]any{},
		authHeader(writeKey))
	// Should not get 401/403 or 5xx.
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("POST /index: status = %d, should not be auth error (body=%s)", w.Code, w.Body.String())
	}
	if w.Code >= 500 {
		t.Fatalf("POST /index: status = %d, unexpected server error (body=%s)", w.Code, w.Body.String())
	}
}

// ---------- Owner-level routes blocked for API keys ----------

func TestAPIKeyAuth_Key_CannotDeleteProject(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "write-key", "write", aliceToken)

	// DELETE /projects/{id} is owner-only → 401 for API keys (RequireUser blocks first).
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("DELETE project: status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- Search via API key ----------

func TestAPIKeyAuth_ProjectKey_CanSearch(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// POST /query/search should be accessible (member-level data route).
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/query/search", projID),
		map[string]any{"query": "test"},
		authHeader(apiKey))
	// Should not get 401/403 or 5xx (handler may return 4xx for other reasons).
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("POST /query/search: status = %d, should not be auth error (body=%s)", w.Code, w.Body.String())
	}
	if w.Code >= 500 {
		t.Fatalf("POST /query/search: status = %d, unexpected server error (body=%s)", w.Code, w.Body.String())
	}
}

// ---------- last_used_at is updated ----------

func TestAPIKeyAuth_LastUsedAtUpdated(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	keyID, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// Verify last_used_at is NULL before first use.
	pgKeyID, err := dbconv.StringToPgUUID(keyID)
	if err != nil {
		t.Fatalf("parse key ID: %v", err)
	}
	var lastUsedBefore *string
	err = a.DB.Pool.QueryRow(context.Background(),
		"SELECT last_used_at::text FROM api_keys WHERE id = $1", pgKeyID).Scan(&lastUsedBefore)
	if err != nil {
		t.Fatalf("query last_used_at before: %v", err)
	}
	if lastUsedBefore != nil {
		t.Fatalf("last_used_at should be NULL before use, got %v", *lastUsedBefore)
	}

	// Use the key.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusOK {
		t.Fatalf("GET project: status = %d (body=%s)", w.Code, w.Body.String())
	}

	// The update is fire-and-forget in a goroutine; poll up to 2s.
	var lastUsedAfter *string
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		err = a.DB.Pool.QueryRow(context.Background(),
			"SELECT last_used_at::text FROM api_keys WHERE id = $1", pgKeyID).Scan(&lastUsedAfter)
		if err != nil {
			t.Fatalf("query last_used_at after: %v", err)
		}
		if lastUsedAfter != nil {
			break
		}
	}
	if lastUsedAfter == nil {
		t.Error("last_used_at still NULL after use — fire-and-forget update did not complete")
	}
}

// ---------- Expired key: rejected at auth time ----------

func TestAPIKeyAuth_ExpiredKey_Rejected(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// Verify key works before expiry.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusOK {
		t.Fatalf("pre-expiry: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Expire the key directly in DB (set expires_at to the past).
	keyHash := auth.HashToken(apiKey)
	_, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE api_keys SET expires_at = NOW() - INTERVAL '1 hour' WHERE key_hash = $1", keyHash)
	if err != nil {
		t.Fatalf("expire key: %v", err)
	}

	// Expired key → 401.
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("post-expiry: status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- Deleted key (via API): rejected at auth time ----------

func TestAPIKeyAuth_DeletedKey_Rejected(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	keyID, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	// Verify key works before deletion.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusOK {
		t.Fatalf("pre-delete: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Delete the key via the API.
	w = doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/keys/%s", projID, keyID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Deleted key → 401.
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(apiKey))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("post-delete: status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ---------- Personal key: access revoked when membership removed ----------

func TestAPIKeyAuth_PersonalKey_RevokedOnMemberRemoval(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	sshKeyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", sshKeyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Bob is added as an admin member and creates a personal key.
	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	_, bobKey := createPersonalAPIKey(t, a, "bob-key", "write", bobToken)

	// Bob's personal key can access the project.
	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(bobKey))
	if w.Code != http.StatusOK {
		t.Fatalf("pre-removal: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	// Remove bob's membership directly in DB.
	bobUUID, err := dbconv.StringToPgUUID(bobID)
	if err != nil {
		t.Fatalf("parse bob ID: %v", err)
	}
	projUUID, err := dbconv.StringToPgUUID(projID)
	if err != nil {
		t.Fatalf("parse project ID: %v", err)
	}
	err = a.DB.Queries.DeleteProjectMember(context.Background(), db.DeleteProjectMemberParams{
		ProjectID: projUUID,
		UserID:    bobUUID,
	})
	if err != nil {
		t.Fatalf("remove member: %v", err)
	}

	// Bob's personal key can no longer access the project → 404.
	w = doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s", projID),
		nil, authHeader(bobKey))
	if w.Code != http.StatusNotFound {
		t.Fatalf("post-removal: status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}
