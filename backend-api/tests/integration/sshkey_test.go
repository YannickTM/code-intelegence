//go:build integration

package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHKey_Create(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name": "my-deploy-key",
	}, authHeader(token))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)

	// Verify required fields.
	if id, ok := m["id"].(string); !ok || id == "" {
		t.Error("id should be a non-empty string (UUID)")
	}
	if m["name"] != "my-deploy-key" {
		t.Errorf("name = %v, want %q", m["name"], "my-deploy-key")
	}
	if pub, ok := m["public_key"].(string); !ok || !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Errorf("public_key = %v, want ssh-ed25519 prefix", m["public_key"])
	}
	if fp, ok := m["fingerprint"].(string); !ok || !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("fingerprint = %v, want SHA256: prefix", m["fingerprint"])
	}
	if m["key_type"] != "ed25519" {
		t.Errorf("key_type = %v, want %q", m["key_type"], "ed25519")
	}
	if m["is_active"] != true {
		t.Errorf("is_active = %v, want true", m["is_active"])
	}

	// Private key must NEVER appear in response.
	if _, ok := m["private_key"]; ok {
		t.Error("response must not contain private_key")
	}
	if _, ok := m["private_key_encrypted"]; ok {
		t.Error("response must not contain private_key_encrypted")
	}
}

func TestSSHKey_CreateValidation(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Missing name.
	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{}, authHeader(token))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	// Name too long (> 100 chars).
	w = doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name": strings.Repeat("a", 101),
	}, authHeader(token))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("long name: status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

func TestSSHKey_CreateAnonymous(t *testing.T) {
	a := setupTestApp(t)

	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name": "anon-key",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestSSHKey_List(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Initially empty.
	w := doRequest(t, a, http.MethodGet, "/v1/ssh-keys", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list empty: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0", len(items))
	}

	// Create two keys.
	createSSHKey(t, a, "key-1", token)
	createSSHKey(t, a, "key-2", token)

	w = doRequest(t, a, http.MethodGet, "/v1/ssh-keys", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m = decodeJSON(t, w)
	items, ok = m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 2 {
		t.Errorf("items length = %d, want 2", len(items))
	}
}

func TestSSHKey_ListUserScoped(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a key.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	createSSHKey(t, a, "alice-key", aliceToken)

	// Bob creates a key.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	createSSHKey(t, a, "bob-key", bobToken)

	// Alice should only see her own key.
	w := doRequest(t, a, http.MethodGet, "/v1/ssh-keys", nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("alice list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items is not []any, got %T: %v", m["items"], m["items"])
	}
	if len(items) != 1 {
		t.Fatalf("alice items = %d, want 1", len(items))
	}
	key, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("item is not a map")
	}
	if key["name"] != "alice-key" {
		t.Errorf("alice key name = %v, want %q", key["name"], "alice-key")
	}

	// Bob should only see his own key.
	w = doRequest(t, a, http.MethodGet, "/v1/ssh-keys", nil, authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("bob list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	m = decodeJSON(t, w)
	items, ok = m["items"].([]any)
	if !ok {
		t.Fatalf("items is not []any, got %T: %v", m["items"], m["items"])
	}
	if len(items) != 1 {
		t.Fatalf("bob items = %d, want 1", len(items))
	}
	key, ok = items[0].(map[string]any)
	if !ok {
		t.Fatal("item is not a map")
	}
	if key["name"] != "bob-key" {
		t.Errorf("bob key name = %v, want %q", key["name"], "bob-key")
	}
}

func TestSSHKey_Get(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Create a key.
	keyID := createSSHKey(t, a, "get-test", token)

	// Get the key.
	w := doRequest(t, a, http.MethodGet, "/v1/ssh-keys/"+keyID, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("get: status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["id"] != keyID {
		t.Errorf("id = %v, want %q", m["id"], keyID)
	}
	if m["name"] != "get-test" {
		t.Errorf("name = %v, want %q", m["name"], "get-test")
	}
}

func TestSSHKey_GetOtherUser404(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Alice creates a key.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "alice-key", aliceToken)

	// Bob tries to get Alice's key.
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	w := doRequest(t, a, http.MethodGet, "/v1/ssh-keys/"+keyID, nil, authHeader(bobToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestSSHKey_ListProjectsEmpty(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	keyID := createSSHKey(t, a, "proj-test", token)

	w := doRequest(t, a, http.MethodGet, "/v1/ssh-keys/"+keyID+"/projects", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list-projects: status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatal("missing items field")
	}
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0", len(items))
	}
	if m["total"] != float64(0) {
		t.Errorf("total = %v, want 0", m["total"])
	}
}

func TestSSHKey_Retire(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	keyID := createSSHKey(t, a, "retire-me", token)

	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys/"+keyID+"/retire", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("retire: status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	if m["is_active"] != false {
		t.Errorf("is_active = %v, want false", m["is_active"])
	}
	if m["rotated_at"] == nil {
		t.Error("rotated_at should be set after retirement")
	}
}

func TestSSHKey_RetireOtherUser404(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "alice-key", aliceToken)

	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys/"+keyID+"/retire", nil, authHeader(bobToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestSSHKey_RetireWithActiveAssignment409(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Create an SSH key.
	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{"name": "assigned-key"}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	keyID, _ := created["id"].(string)

	// Insert a project and an active assignment directly via SQL.
	ctx := context.Background()
	_, err := a.DB.Pool.Exec(ctx,
		`INSERT INTO projects (name, repo_url) VALUES ('test-proj', 'https://example.com/repo.git')`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO project_ssh_key_assignments (project_id, ssh_key_id, is_active)
		 SELECT p.id, $1::uuid, TRUE FROM projects p WHERE p.name = 'test-proj'`, keyID)
	if err != nil {
		t.Fatalf("insert assignment: %v", err)
	}

	// Retire should fail with 409.
	w = doRequest(t, a, http.MethodPost, "/v1/ssh-keys/"+keyID+"/retire", nil, authHeader(token))
	if w.Code != http.StatusConflict {
		t.Fatalf("retire: status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestSSHKey_NoPrivateKeyInResponses(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Create.
	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{"name": "check-priv"}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d (body=%s)", w.Code, w.Body.String())
	}
	created := decodeJSON(t, w)
	keyID, _ := created["id"].(string)

	assertNoPrivateKey(t, "create", created)

	// Get.
	w = doRequest(t, a, http.MethodGet, "/v1/ssh-keys/"+keyID, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("get: status = %d (body=%s)", w.Code, w.Body.String())
	}
	assertNoPrivateKey(t, "get", decodeJSON(t, w))

	// List.
	w = doRequest(t, a, http.MethodGet, "/v1/ssh-keys", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	listBody := decodeJSON(t, w)
	items, ok := listBody["items"].([]any)
	if !ok {
		t.Fatalf("items is not []any, got %T: %v", listBody["items"], listBody["items"])
	}
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("list[%d]: expected map[string]any, got %T", i, item)
		}
		assertNoPrivateKey(t, fmt.Sprintf("list[%d]", i), m)
	}

	// Retire.
	w = doRequest(t, a, http.MethodPost, "/v1/ssh-keys/"+keyID+"/retire", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("retire: status = %d (body=%s)", w.Code, w.Body.String())
	}
	assertNoPrivateKey(t, "retire", decodeJSON(t, w))
}

// generateTestEd25519PEM creates an Ed25519 private key PEM for integration tests.
func generateTestEd25519PEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("ssh.MarshalPrivateKey: %v", err)
	}
	return string(pem.EncodeToMemory(pemBlock))
}

func TestSSHKey_CreateWithUploadedKey(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	privPEM := generateTestEd25519PEM(t)

	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name":        "uploaded-key",
		"private_key": privPEM,
	}, authHeader(token))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	m := decodeJSON(t, w)

	if pub, ok := m["public_key"].(string); !ok || !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Errorf("public_key = %v, want ssh-ed25519 prefix", m["public_key"])
	}
	if fp, ok := m["fingerprint"].(string); !ok || !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("fingerprint = %v, want SHA256: prefix", m["fingerprint"])
	}
	if m["key_type"] != "ed25519" {
		t.Errorf("key_type = %v, want %q", m["key_type"], "ed25519")
	}
	if m["name"] != "uploaded-key" {
		t.Errorf("name = %v, want %q", m["name"], "uploaded-key")
	}

	assertNoPrivateKey(t, "create-uploaded", m)
}

func TestSSHKey_CreateWithInvalidPEM(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name":        "bad-key",
		"private_key": "this is not a valid PEM",
	}, authHeader(token))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestSSHKey_CreateWithPassphraseProtectedPEM(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	// Generate a passphrase-protected Ed25519 PEM.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("secret123"))
	if err != nil {
		t.Fatalf("ssh.MarshalPrivateKeyWithPassphrase: %v", err)
	}
	encryptedPEM := string(pem.EncodeToMemory(pemBlock))

	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name":        "locked-key",
		"private_key": encryptedPEM,
	}, authHeader(token))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "passphrase") {
		t.Errorf("response body = %q, want message containing 'passphrase'", body)
	}
}

func TestSSHKey_CreateDuplicateFingerprint(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	privPEM := generateTestEd25519PEM(t)

	// First upload should succeed.
	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name":        "key-first",
		"private_key": privPEM,
	}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("first: status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	// Second upload of the same key should conflict.
	w = doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{
		"name":        "key-second",
		"private_key": privPEM,
	}, authHeader(token))
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate: status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
}

func assertNoPrivateKey(t *testing.T, context string, m map[string]any) {
	t.Helper()
	for _, key := range []string{"private_key", "private_key_encrypted", "privateKey"} {
		if _, ok := m[key]; ok {
			t.Errorf("%s: response contains %q — private key must never be exposed", context, key)
		}
	}
}
