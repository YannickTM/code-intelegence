//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"

	"myjungle/backend-api/internal/app"
)

func TestUser_Register(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username":     "alice",
		"email":        "alice@example.com",
		"display_name": "Alice Test",
	}, nil)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user field in response")
	}

	if user["username"] != "alice" {
		t.Errorf("username = %v, want %q", user["username"], "alice")
	}
	if user["email"] != "alice@example.com" {
		t.Errorf("email = %v, want %q", user["email"], "alice@example.com")
	}
	if user["display_name"] != "Alice Test" {
		t.Errorf("display_name = %v, want %q", user["display_name"], "Alice Test")
	}
	if id, ok := user["id"].(string); !ok || id == "" {
		t.Error("id should be a non-empty string (UUID)")
	}
	if user["is_active"] != true {
		t.Errorf("is_active = %v, want true", user["is_active"])
	}
}

func TestUser_RegisterDefaultDisplayName(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
	}, nil)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid user field in response")
	}

	// When no display_name is provided, it defaults to the username.
	if user["display_name"] != "bob" {
		t.Errorf("display_name = %v, want %q (should default to username)", user["display_name"], "bob")
	}
	if user["email"] != "bob@example.com" {
		t.Errorf("email = %v, want %q", user["email"], "bob@example.com")
	}
}

func TestUser_RegisterDuplicate(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username": "alice",
		"email":    "alice-2@example.com",
	}, nil)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["code"] != "conflict" {
		t.Errorf("code = %v, want %q", m["code"], "conflict")
	}

	w = doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username": "bob",
		"email":    "alice-2@example.com",
	}, nil)

	if w.Code != http.StatusCreated {
		t.Fatalf("email seed status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	w = doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username": "charlie",
		"email":    " Alice-2@Example.com ",
	}, nil)

	if w.Code != http.StatusConflict {
		t.Fatalf("email conflict status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}

	m = decodeJSON(t, w)
	if m["code"] != "conflict" {
		t.Errorf("email conflict code = %v, want %q", m["code"], "conflict")
	}
}

func TestUser_RegisterNormalization(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username": " Alice ",
		"email":    " Alice@Example.COM ",
	}, nil)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid user field in response")
	}
	if user["username"] != "alice" {
		t.Errorf("username = %v, want %q (should be normalized)", user["username"], "alice")
	}
	if user["email"] != "alice@example.com" {
		t.Errorf("email = %v, want %q (should be normalized)", user["email"], "alice@example.com")
	}
}

func TestUser_RegisterMissingUsername(t *testing.T) {
	a := setupTestApp(t)

	w := doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{}, nil)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["code"] != "validation_error" {
		t.Errorf("code = %v, want %q", m["code"], "validation_error")
	}
}

func TestUser_Login(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/auth/login", map[string]any{
		"username": "alice",
	}, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)

	token, ok := m["token"].(string)
	if !ok || token == "" {
		t.Fatal("missing or empty token in response")
	}
	// GenerateSessionToken produces a 32-byte hex-encoded token = 64 chars.
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}

	if _, ok := m["expires_at"]; !ok {
		t.Error("missing expires_at in response")
	}

	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user in response")
	}
	if user["username"] != "alice" {
		t.Errorf("user.username = %v, want %q", user["username"], "alice")
	}

	// Verify Set-Cookie header.
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}
	if sessionCookie.Value != token {
		t.Errorf("cookie value = %q, want %q", sessionCookie.Value, token)
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if sessionCookie.Path != "/" {
		t.Errorf("cookie path = %q, want %q", sessionCookie.Path, "/")
	}
}

func TestUser_LoginUnknownUser(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodPost, "/v1/auth/login", map[string]any{
		"username": "nonexistent",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["code"] != "unauthorized" {
		t.Errorf("code = %v, want %q", m["code"], "unauthorized")
	}
}

func TestUser_GetMeAuthenticated(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user field")
	}
	if user["username"] != "alice" {
		t.Errorf("username = %v, want %q", user["username"], "alice")
	}
	if user["is_active"] != true {
		t.Errorf("is_active = %v, want true", user["is_active"])
	}
}

func TestUser_GetMeAnonymous(t *testing.T) {
	a := setupTestApp(t)

	w := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	m := decodeJSON(t, w)
	if m["code"] != "unauthorized" {
		t.Errorf("code = %v, want %q", m["code"], "unauthorized")
	}
}

func TestUser_GetMeViaCookie(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Send the token via a cookie instead of the Authorization header.
	w := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, map[string]string{
		"Cookie": "session=" + token,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user field")
	}
	if user["username"] != "alice" {
		t.Errorf("username = %v, want %q", user["username"], "alice")
	}
}

func TestUser_GetMeInvalidToken(t *testing.T) {
	a := setupTestApp(t)

	w := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader("invalid-token-value"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestUser_UpdateProfile(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPatch, "/v1/users/me", map[string]any{
		"display_name": "Alice Updated",
	}, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user field")
	}
	if user["display_name"] != "Alice Updated" {
		t.Errorf("display_name = %v, want %q", user["display_name"], "Alice Updated")
	}
	if user["username"] != "alice" {
		t.Errorf("username = %v, want %q (should not change)", user["username"], "alice")
	}
}

func TestUser_UpdateProfileWithAvatarURL(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Set avatar URL.
	w := doRequest(t, a, http.MethodPatch, "/v1/users/me", map[string]any{
		"avatar_url": "https://example.com/avatar.png",
	}, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("set avatar: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid user field in set avatar response")
	}
	if user["avatar_url"] != "https://example.com/avatar.png" {
		t.Errorf("avatar_url = %v, want %q", user["avatar_url"], "https://example.com/avatar.png")
	}

	// Verify it persists when fetching /me.
	w = doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("get me: status = %d", w.Code)
	}
	m = decodeJSON(t, w)
	user, ok = m["user"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid user field in get me response")
	}
	if user["avatar_url"] != "https://example.com/avatar.png" {
		t.Errorf("avatar_url after reload = %v, want %q", user["avatar_url"], "https://example.com/avatar.png")
	}
}

func TestUser_UpdateProfileEmptyPayload(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPatch, "/v1/users/me", map[string]any{}, authHeader(token))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["code"] != "validation_error" {
		t.Errorf("code = %v, want %q", m["code"], "validation_error")
	}
}

func TestUser_UpdateProfileBlankDisplayName(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	displayName := ""
	w := doRequest(t, a, http.MethodPatch, "/v1/users/me", map[string]any{
		"display_name": displayName,
	}, authHeader(token))

	// The handler rejects blank display_name (after trimming) with 422.
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestUser_ListMyProjectsEmpty(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0", len(items))
	}
}

// grantPlatformAdmin grants the platform_admin role to a user via direct SQL.
func grantPlatformAdmin(t *testing.T, a *app.App, userID string) {
	t.Helper()
	_, err := a.DB.Pool.Exec(context.Background(),
		"INSERT INTO user_platform_roles (user_id, role) VALUES ($1, 'platform_admin') ON CONFLICT DO NOTHING", userID)
	if err != nil {
		t.Fatalf("grantPlatformAdmin: %v", err)
	}
}

func TestUser_ListMyProjects_PlatformAdminSeesAllProjects(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", aliceToken)
	createProject(t, a, "alice-proj", "git@github.com:alice/repo.git", keyID, aliceToken)

	// Bob is a platform admin but NOT a member of Alice's project.
	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	grantPlatformAdmin(t, a, bobID)
	bobToken := loginUser(t, a, "bob")

	// Bob should see Alice's project via /v1/users/me/projects.
	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}

	proj := items[0].(map[string]any)
	if proj["name"] != "alice-proj" {
		t.Errorf("name = %v, want alice-proj", proj["name"])
	}
	if proj["role"] != "owner" {
		t.Errorf("role = %v, want owner", proj["role"])
	}
}

func TestUser_ListMyProjects_PlatformAdminMemberStillSeesOwnerRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project and adds Bob as a "member".
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", aliceToken)
	projResp := createProject(t, a, "shared-proj", "git@github.com:org/shared.git", keyID, aliceToken)
	projID := projResp["id"].(string)

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	grantPlatformAdmin(t, a, bobID)

	// Add Bob as member (lowest role) to the project.
	w := doRequest(t, a, http.MethodPost, "/v1/projects/"+projID+"/members", map[string]any{
		"user_id": bobID,
		"role":    "member",
	}, authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("add member: status = %d (body=%s)", w.Code, w.Body.String())
	}

	bobToken := loginUser(t, a, "bob")

	// Bob should see the project with role "owner" (platform admin overrides member role).
	w = doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}

	proj := items[0].(map[string]any)
	if proj["role"] != "owner" {
		t.Errorf("role = %v, want owner (platform admin should override member role)", proj["role"])
	}
}

func TestUser_ListMyProjects_RegularUserUnchanged(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a project.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "k1", aliceToken)
	createProject(t, a, "alice-proj", "git@github.com:alice/repo.git", keyID, aliceToken)

	// Bob is a regular user with no projects.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")

	// Bob should see zero projects (not Alice's).
	w := doRequest(t, a, http.MethodGet, "/v1/users/me/projects", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0 (regular user should not see others' projects)", len(items))
	}
}

func TestUser_Logout(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Verify token works before logout.
	w := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("pre-logout /me: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Logout.
	w = doRequest(t, a, http.MethodPost, "/v1/auth/logout", nil, authHeader(token))
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify Set-Cookie clears the session cookie.
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("logout response missing session cookie")
	}
	if sessionCookie.MaxAge >= 0 {
		t.Errorf("session cookie MaxAge = %d, want < 0 (cleared)", sessionCookie.MaxAge)
	}

	// Verify the token no longer works.
	w = doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("post-logout /me: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestUser_MultipleSessionsIndependent(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")

	// Create two separate sessions.
	token1 := loginUser(t, a, "alice")
	token2 := loginUser(t, a, "alice")

	// Both should work.
	w1 := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token1))
	w2 := doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token2))
	if w1.Code != http.StatusOK {
		t.Errorf("token1 /me: status = %d, want %d", w1.Code, http.StatusOK)
	}
	if w2.Code != http.StatusOK {
		t.Errorf("token2 /me: status = %d, want %d", w2.Code, http.StatusOK)
	}

	// Logout with token1 — token2 should still work.
	doRequest(t, a, http.MethodPost, "/v1/auth/logout", nil, authHeader(token1))

	w1 = doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token1))
	w2 = doRequest(t, a, http.MethodGet, "/v1/users/me", nil, authHeader(token2))
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("token1 post-logout: status = %d, want %d", w1.Code, http.StatusUnauthorized)
	}
	if w2.Code != http.StatusOK {
		t.Errorf("token2 post-logout: status = %d, want %d (independent session)", w2.Code, http.StatusOK)
	}
}
