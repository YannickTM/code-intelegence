//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"myjungle/backend-api/internal/app"
)

func TestProviders_SupportedList(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	unauthW := doRequest(t, a, http.MethodGet, "/v1/settings/providers", nil, nil)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d (body=%s)", unauthW.Code, http.StatusUnauthorized, unauthW.Body.String())
	}

	w := doRequest(t, a, http.MethodGet, "/v1/settings/providers", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	embeddingProviders := mustStringSlice(t, m["embedding"])
	llmProviders := mustStringSlice(t, m["llm"])

	if !containsString(embeddingProviders, "ollama") {
		t.Fatalf("embedding providers = %v, want to contain ollama", embeddingProviders)
	}
	if !containsString(llmProviders, "ollama") {
		t.Fatalf("llm providers = %v, want to contain ollama", llmProviders)
	}
}

func TestProjects_SelectedGlobalConfigTriggersRejectProjectOwnedConfigs(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "provider-key", token)
	proj := createProject(t, a, "provider-proj", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")

	embeddingBase := fmt.Sprintf("/v1/projects/%s/settings/embedding", projectID)
	w := doRequest(t, a, http.MethodPut, embeddingBase, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Embed",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "jina/jina-embeddings-v2-base-en",
		"dimensions":   768,
		"max_tokens":   8000,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT embedding custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	embeddingConfigID := mustString(t, mustValueMap(t, decodeJSON(t, w)["config"]), "id")

	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE projects SET selected_embedding_global_config_id = $1 WHERE id = $2",
		embeddingConfigID, projectID); err == nil {
		t.Fatal("expected trigger to reject project-owned embedding config selection")
	} else if !contains(err.Error(), "selected_embedding_global_config_id must reference a shareable global embedding provider config") {
		t.Fatalf("embedding trigger error = %v", err)
	}

	llmBase := fmt.Sprintf("/v1/projects/%s/settings/llm", projectID)
	w = doRequest(t, a, http.MethodPut, llmBase, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Chat",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "llama3.1",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT llm custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	llmConfigID := mustString(t, mustValueMap(t, decodeJSON(t, w)["config"]), "id")

	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE projects SET selected_llm_global_config_id = $1 WHERE id = $2",
		llmConfigID, projectID); err == nil {
		t.Fatal("expected trigger to reject project-owned llm config selection")
	} else if !contains(err.Error(), "selected_llm_global_config_id must reference a shareable global llm provider config") {
		t.Fatalf("llm trigger error = %v", err)
	}

	var unavailableEmbeddingConfigID string
	if err := a.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO embedding_provider_configs (name, provider, endpoint_url, model, dimensions, is_active, is_available_to_projects)
		 VALUES ('Private Global Ollama Embed', 'ollama', 'http://localhost:11434', 'jina/jina-embeddings-v2-base-en', 768, true, false)
		 RETURNING id`).Scan(&unavailableEmbeddingConfigID); err != nil {
		t.Fatalf("insert unavailable global embedding config: %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE projects SET selected_embedding_global_config_id = $1 WHERE id = $2",
		unavailableEmbeddingConfigID, projectID); err == nil {
		t.Fatal("expected trigger to reject non-shareable global embedding config selection")
	} else if !contains(err.Error(), "selected_embedding_global_config_id must reference a shareable global embedding provider config") {
		t.Fatalf("embedding availability trigger error = %v", err)
	}

	var unavailableLLMConfigID string
	if err := a.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO llm_provider_configs (name, provider, endpoint_url, model, is_active, is_available_to_projects)
		 VALUES ('Private Global Ollama Chat', 'ollama', 'http://localhost:11434', 'llama3.1', true, false)
		 RETURNING id`).Scan(&unavailableLLMConfigID); err != nil {
		t.Fatalf("insert unavailable global llm config: %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE projects SET selected_llm_global_config_id = $1 WHERE id = $2",
		unavailableLLMConfigID, projectID); err == nil {
		t.Fatal("expected trigger to reject non-shareable global llm config selection")
	} else if !contains(err.Error(), "selected_llm_global_config_id must reference a shareable global llm provider config") {
		t.Fatalf("llm availability trigger error = %v", err)
	}

	var globalEmbeddingConfigID string
	if err := a.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO embedding_provider_configs (name, provider, endpoint_url, model, dimensions, is_active, is_available_to_projects)
		 VALUES ('Global Ollama Embed', 'ollama', 'http://localhost:11434', 'jina/jina-embeddings-v2-base-en', 768, true, true)
		 RETURNING id`).Scan(&globalEmbeddingConfigID); err != nil {
		t.Fatalf("insert global embedding config: %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE projects SET selected_embedding_global_config_id = $1 WHERE id = $2",
		globalEmbeddingConfigID, projectID); err != nil {
		t.Fatalf("set selected global embedding config: %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE embedding_provider_configs SET is_available_to_projects = FALSE WHERE id = $1",
		globalEmbeddingConfigID); err == nil {
		t.Fatal("expected trigger to reject turning a selected global embedding config non-shareable")
	} else if !contains(err.Error(), "selected_embedding_global_config_id must reference a shareable global embedding provider config") {
		t.Fatalf("embedding provider availability toggle trigger error = %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE embedding_provider_configs SET project_id = $1 WHERE id = $2",
		projectID, globalEmbeddingConfigID); err == nil {
		t.Fatal("expected trigger to reject turning a selected global embedding config into a project config")
	} else if !contains(err.Error(), "selected embedding provider config cannot become project-owned while referenced by a project") {
		t.Fatalf("embedding provider update trigger error = %v", err)
	}

	var globalLLMConfigID string
	if err := a.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO llm_provider_configs (name, provider, endpoint_url, model, is_active, is_available_to_projects)
		 VALUES ('Global Ollama Chat', 'ollama', 'http://localhost:11434', 'llama3.1', true, true)
		 RETURNING id`).Scan(&globalLLMConfigID); err != nil {
		t.Fatalf("insert global llm config: %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE projects SET selected_llm_global_config_id = $1 WHERE id = $2",
		globalLLMConfigID, projectID); err != nil {
		t.Fatalf("set selected global llm config: %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE llm_provider_configs SET is_available_to_projects = FALSE WHERE id = $1",
		globalLLMConfigID); err == nil {
		t.Fatal("expected trigger to reject turning a selected global llm config non-shareable")
	} else if !contains(err.Error(), "selected_llm_global_config_id must reference a shareable global llm provider config") {
		t.Fatalf("llm provider availability toggle trigger error = %v", err)
	}
	if _, err := a.DB.Pool.Exec(context.Background(),
		"UPDATE llm_provider_configs SET project_id = $1 WHERE id = $2",
		projectID, globalLLMConfigID); err == nil {
		t.Fatal("expected trigger to reject turning a selected global llm config into a project config")
	} else if !contains(err.Error(), "selected llm provider config cannot become project-owned while referenced by a project") {
		t.Fatalf("llm provider update trigger error = %v", err)
	}
}

func TestEmbedding_ProjectSettingLifecycle(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "embed-key", token)
	proj := createProject(t, a, "embed-proj", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")
	base := fmt.Sprintf("/v1/projects/%s/settings/embedding", projectID)

	w := doRequest(t, a, http.MethodGet, base, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET default status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["mode"] != "default" {
		t.Fatalf("mode = %v, want default", m["mode"])
	}
	defaultCfg := mustValueMap(t, m["config"])
	if defaultCfg["provider"] != "ollama" {
		t.Fatalf("provider = %v, want ollama", defaultCfg["provider"])
	}
	if defaultCfg["model"] != "jina/jina-embeddings-v2-base-en" {
		t.Fatalf("model = %v, want jina/jina-embeddings-v2-base-en", defaultCfg["model"])
	}
	if dims := int(defaultCfg["dimensions"].(float64)); dims != 768 {
		t.Fatalf("dimensions = %d, want 768", dims)
	}
	assertNoSecretFields(t, defaultCfg)

	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "default" {
		t.Fatalf("source = %v, want default", m["source"])
	}

	w = doRequest(t, a, http.MethodGet, base+"/available", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET available status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	items := mustItems(t, decodeJSON(t, w))
	if len(items) == 0 {
		t.Fatal("expected at least one available embedding config")
	}
	available := mustValueMap(t, items[0])
	defaultConfigID := mustString(t, available, "id")
	assertNoSecretFields(t, available)

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":             "global",
		"global_config_id": defaultConfigID,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT global status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["mode"] != "global" {
		t.Fatalf("mode = %v, want global", m["mode"])
	}
	if m["global_config_id"] != defaultConfigID {
		t.Fatalf("global_config_id = %v, want %s", m["global_config_id"], defaultConfigID)
	}

	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after global select status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "global" {
		t.Fatalf("source = %v, want global", m["source"])
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "mxbai-embed-large",
		"dimensions":   1024,
		"max_tokens":   8000,
		"settings": map[string]any{
			"chunk_size": 512,
		},
		"credentials": map[string]any{
			"api_key": "secret-value",
		},
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["mode"] != "custom" {
		t.Fatalf("mode = %v, want custom", m["mode"])
	}
	customCfg := mustValueMap(t, m["config"])
	if customCfg["has_credentials"] != true {
		t.Fatalf("has_credentials = %v, want true", customCfg["has_credentials"])
	}
	assertNoSecretFields(t, customCfg)
	if count := countRows(t, a, "SELECT count(*) FROM embedding_provider_configs WHERE project_id = $1", projectID); count != 1 {
		t.Fatalf("custom embedding row count after first PUT = %d, want 1", count)
	}
	if !hasEncryptedCredentials(t, a, "SELECT COALESCE(octet_length(credentials_encrypted), 0) > 0 FROM embedding_provider_configs WHERE project_id = $1 AND is_active = TRUE", projectID) {
		t.Fatal("expected active custom embedding config to have encrypted credentials")
	}
	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after first custom PUT status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "custom" {
		t.Fatalf("source after first custom PUT = %v, want custom", m["source"])
	}
	assertNoSecretFields(t, mustValueMap(t, m["config"]))

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama changed endpoint",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11435",
		"model":        "mxbai-embed-large",
		"dimensions":   1024,
		"max_tokens":   8000,
	}, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT custom changed endpoint without credentials status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama v2",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "jina/jina-embeddings-v2-base-en",
		"dimensions":   768,
		"max_tokens":   8000,
		"settings": map[string]any{
			"chunk_size": 256,
		},
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom preserve credentials status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	customCfg = mustValueMap(t, m["config"])
	if customCfg["has_credentials"] != true {
		t.Fatalf("has_credentials = %v, want true after credentials-preserving update", customCfg["has_credentials"])
	}
	if count := countRows(t, a, "SELECT count(*) FROM embedding_provider_configs WHERE project_id = $1", projectID); count != 2 {
		t.Fatalf("custom embedding row count after second PUT = %d, want 2", count)
	}
	if !hasEncryptedCredentials(t, a, "SELECT COALESCE(octet_length(credentials_encrypted), 0) > 0 FROM embedding_provider_configs WHERE project_id = $1 AND is_active = TRUE", projectID) {
		t.Fatal("expected active custom embedding config to preserve encrypted credentials")
	}
	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after credentials-preserving update status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "custom" {
		t.Fatalf("source after credentials-preserving update = %v, want custom", m["source"])
	}
	assertNoSecretFields(t, mustValueMap(t, m["config"]))

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama v3",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "jina/jina-embeddings-v2-base-en",
		"dimensions":   768,
		"max_tokens":   8000,
		"credentials":  nil,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom clear credentials status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	customCfg = mustValueMap(t, m["config"])
	if customCfg["has_credentials"] != false {
		t.Fatalf("has_credentials = %v, want false after clear", customCfg["has_credentials"])
	}
	if hasEncryptedCredentials(t, a, "SELECT COALESCE(octet_length(credentials_encrypted), 0) > 0 FROM embedding_provider_configs WHERE project_id = $1 AND is_active = TRUE", projectID) {
		t.Fatal("expected active custom embedding config to clear encrypted credentials")
	}
	if hasEncryptedCredentials(t, a, "SELECT EXISTS (SELECT 1 FROM embedding_provider_configs WHERE project_id = $1 AND COALESCE(octet_length(credentials_encrypted), 0) > 0)", projectID) {
		t.Fatal("expected all embedding config rows to clear encrypted credentials")
	}
	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after clearing credentials status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "custom" {
		t.Fatalf("source after clearing credentials = %v, want custom", m["source"])
	}
	assertNoSecretFields(t, mustValueMap(t, m["config"]))

	w = doRequest(t, a, http.MethodDelete, base, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = doRequest(t, a, http.MethodGet, base, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET after delete status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["mode"] != "default" {
		t.Fatalf("mode after delete = %v, want default", m["mode"])
	}

	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after delete status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "default" {
		t.Fatalf("source after delete = %v, want default", m["source"])
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Bad Provider",
		"provider":     "unsupported-provider",
		"endpoint_url": "http://localhost:11434",
		"model":        "text-embedding-3-small",
		"dimensions":   1536,
		"max_tokens":   8000,
	}, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported provider status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestEmbedding_ProjectTestEndpoints(t *testing.T) {
	t.Setenv("PROVIDER_CONNECTIVITY_ALLOWED_HOSTS", "127.0.0.1")
	a := setupTestApp(t)
	truncateAll(t, a)

	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "jina/jina-embeddings-v2-base-en:latest"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockOllama.Close()

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "embed-key", token)
	proj := createProject(t, a, "embed-proj-test", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")
	base := fmt.Sprintf("/v1/projects/%s/settings/embedding", projectID)

	w := doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama",
		"provider":     "ollama",
		"endpoint_url": mockOllama.URL,
		"model":        "jina/jina-embeddings-v2-base-en",
		"dimensions":   768,
		"max_tokens":   8000,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = doRequest(t, a, http.MethodPost, base+"/test", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /test resolved status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["ok"] != true {
		t.Fatalf("resolved /test ok = %v, want true", m["ok"])
	}

	w = doRequest(t, a, http.MethodPost, base+"/test", map[string]any{
		"provider":     "ollama",
		"endpoint_url": mockOllama.URL,
		"model":        "jina/jina-embeddings-v2-base-en",
		"dimensions":   768,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /test explicit body status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["ok"] != true {
		t.Fatalf("explicit body /test ok = %v, want true", m["ok"])
	}

	w = doRawRequest(t, a, http.MethodPost, base+"/test", `{"provider":`, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /test malformed body status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestEmbedding_ProjectDeleteScrubsCredentials(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "embed-delete-key", token)
	proj := createProject(t, a, "embed-delete-proj", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")
	base := fmt.Sprintf("/v1/projects/%s/settings/embedding", projectID)

	w := doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "mxbai-embed-large",
		"dimensions":   1024,
		"max_tokens":   8000,
		"credentials": map[string]any{
			"api_key": "secret-value",
		},
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = doRequest(t, a, http.MethodDelete, base, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	if hasEncryptedCredentials(t, a, "SELECT EXISTS (SELECT 1 FROM embedding_provider_configs WHERE project_id = $1 AND is_active = TRUE AND COALESCE(octet_length(credentials_encrypted), 0) > 0)", projectID) {
		t.Fatal("expected no active embedding config row to retain encrypted credentials after delete")
	}
	if hasEncryptedCredentials(t, a, "SELECT EXISTS (SELECT 1 FROM embedding_provider_configs WHERE project_id = $1 AND COALESCE(octet_length(credentials_encrypted), 0) > 0)", projectID) {
		t.Fatal("expected no embedding config row to retain encrypted credentials after delete")
	}
}

func TestLLM_ProjectSettingLifecycle(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "llm-key", token)
	proj := createProject(t, a, "llm-proj", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")
	base := fmt.Sprintf("/v1/projects/%s/settings/llm", projectID)

	w := doRequest(t, a, http.MethodGet, base, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET default status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["mode"] != "default" {
		t.Fatalf("mode = %v, want default", m["mode"])
	}
	defaultCfg := mustValueMap(t, m["config"])
	if defaultCfg["provider"] != "ollama" {
		t.Fatalf("provider = %v, want ollama", defaultCfg["provider"])
	}
	assertNoSecretFields(t, defaultCfg)

	w = doRequest(t, a, http.MethodGet, base+"/available", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET available status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	items := mustItems(t, decodeJSON(t, w))
	if len(items) == 0 {
		t.Fatal("expected at least one available llm config")
	}
	defaultConfigID := mustString(t, mustValueMap(t, items[0]), "id")

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":             "global",
		"global_config_id": defaultConfigID,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT global status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["mode"] != "global" {
		t.Fatalf("mode = %v, want global", m["mode"])
	}
	if m["global_config_id"] != defaultConfigID {
		t.Fatalf("global_config_id = %v, want %s", m["global_config_id"], defaultConfigID)
	}

	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after global select status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "global" {
		t.Fatalf("source = %v, want global", m["source"])
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Chat",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"settings": map[string]any{
			"temperature": 0.2,
		},
		"credentials": map[string]any{
			"api_key": "secret-value",
		},
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["mode"] != "custom" {
		t.Fatalf("mode = %v, want custom", m["mode"])
	}
	customCfg := mustValueMap(t, m["config"])
	if customCfg["has_credentials"] != true {
		t.Fatalf("has_credentials = %v, want true", customCfg["has_credentials"])
	}
	assertNoSecretFields(t, customCfg)
	if !hasEncryptedCredentials(t, a, "SELECT COALESCE(octet_length(credentials_encrypted), 0) > 0 FROM llm_provider_configs WHERE project_id = $1 AND is_active = TRUE", projectID) {
		t.Fatal("expected active custom llm config to have encrypted credentials")
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Chat changed endpoint",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11435",
		"model":        "llama3.1",
	}, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT llm custom changed endpoint without credentials status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Chat v2",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "llama3.1",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom preserve credentials status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !hasEncryptedCredentials(t, a, "SELECT COALESCE(octet_length(credentials_encrypted), 0) > 0 FROM llm_provider_configs WHERE project_id = $1 AND is_active = TRUE", projectID) {
		t.Fatal("expected active custom llm config to preserve encrypted credentials")
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Chat v3",
		"provider":     "ollama",
		"endpoint_url": "http://localhost:11434",
		"model":        "llama3.1",
		"credentials":  nil,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom clear credentials status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	customCfg = mustValueMap(t, m["config"])
	if customCfg["has_credentials"] != false {
		t.Fatalf("has_credentials = %v, want false after clear", customCfg["has_credentials"])
	}
	if hasEncryptedCredentials(t, a, "SELECT COALESCE(octet_length(credentials_encrypted), 0) > 0 FROM llm_provider_configs WHERE project_id = $1 AND is_active = TRUE", projectID) {
		t.Fatal("expected active custom llm config to clear encrypted credentials")
	}
	if hasEncryptedCredentials(t, a, "SELECT EXISTS (SELECT 1 FROM llm_provider_configs WHERE project_id = $1 AND COALESCE(octet_length(credentials_encrypted), 0) > 0)", projectID) {
		t.Fatal("expected all llm config rows to clear encrypted credentials")
	}

	w = doRequest(t, a, http.MethodDelete, base, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = doRequest(t, a, http.MethodGet, base+"/resolved", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolved after delete status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["source"] != "default" {
		t.Fatalf("source after delete = %v, want default", m["source"])
	}

	w = doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Bad Provider",
		"provider":     "unsupported-provider",
		"endpoint_url": "http://localhost:11434",
	}, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported provider status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestLLM_ProjectTestEndpoints(t *testing.T) {
	t.Setenv("PROVIDER_CONNECTIVITY_ALLOWED_HOSTS", "127.0.0.1")
	a := setupTestApp(t)
	truncateAll(t, a)

	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "llama3.1:latest"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockOllama.Close()

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "llm-key", token)
	proj := createProject(t, a, "llm-proj-test", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")
	base := fmt.Sprintf("/v1/projects/%s/settings/llm", projectID)

	w := doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "Project Ollama Chat",
		"provider":     "ollama",
		"endpoint_url": mockOllama.URL,
		"model":        "llama3.1",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	w = doRequest(t, a, http.MethodPost, base+"/test", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /test resolved status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["ok"] != true {
		t.Fatalf("resolved /test ok = %v, want true", m["ok"])
	}

	w = doRequest(t, a, http.MethodPost, base+"/test", map[string]any{
		"provider":     "ollama",
		"endpoint_url": mockOllama.URL,
		"model":        "llama3.1",
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /test explicit body status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["ok"] != true {
		t.Fatalf("explicit body /test ok = %v, want true", m["ok"])
	}

	w = doRawRequest(t, a, http.MethodPost, base+"/test", `{"provider":`, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /test malformed body status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestLLM_ProjectTestNoModelConnectivity(t *testing.T) {
	t.Setenv("PROVIDER_CONNECTIVITY_ALLOWED_HOSTS", "127.0.0.1")
	a := setupTestApp(t)
	truncateAll(t, a)

	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "llama3.1:latest"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockOllama.Close()

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	sshKeyID := createSSHKey(t, a, "llm-nomodel-key", token)
	proj := createProject(t, a, "llm-nomodel-proj", "https://github.com/example/repo.git", sshKeyID, token)
	projectID := mustString(t, proj, "id")
	base := fmt.Sprintf("/v1/projects/%s/settings/llm", projectID)

	// Create a custom config with NO model — should succeed since model is optional for LLM.
	w := doRequest(t, a, http.MethodPut, base, map[string]any{
		"mode":         "custom",
		"name":         "LLM No Model",
		"provider":     "ollama",
		"endpoint_url": mockOllama.URL,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT custom no-model status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	// Test resolved config (no model → endpoint-only check).
	w = doRequest(t, a, http.MethodPost, base+"/test", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /test resolved no-model status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["ok"] != true {
		t.Fatalf("resolved no-model /test ok = %v, want true (message=%v)", m["ok"], m["message"])
	}
	msg, _ := m["message"].(string)
	if !contains(msg, "Connected to Ollama") {
		t.Fatalf("resolved no-model /test message = %q, want to contain 'Connected to Ollama'", msg)
	}

	// Test with explicit body, no model field.
	w = doRequest(t, a, http.MethodPost, base+"/test", map[string]any{
		"provider":     "ollama",
		"endpoint_url": mockOllama.URL,
	}, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /test explicit no-model body status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	m = decodeJSON(t, w)
	if m["ok"] != true {
		t.Fatalf("explicit no-model body /test ok = %v, want true (message=%v)", m["ok"], m["message"])
	}
}
func mustItems(t *testing.T, m map[string]any) []any {
	t.Helper()
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %v", m["items"])
	}
	return items
}

func mustValueMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is not a map: %T %#v", value, value)
	}
	return m
}

func mustStringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value is not a slice: %T %#v", value, value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("slice item is not a string: %T %#v", item, item)
		}
		out = append(out, s)
	}
	return out
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func assertNoSecretFields(t *testing.T, m map[string]any) {
	t.Helper()
	if _, ok := m["credentials"]; ok {
		t.Fatalf("unexpected credentials field in response: %#v", m["credentials"])
	}
	if _, ok := m["credentials_encrypted"]; ok {
		t.Fatalf("unexpected credentials_encrypted field in response: %#v", m["credentials_encrypted"])
	}
}

func countRows(t *testing.T, a *app.App, query string, args ...any) int {
	t.Helper()
	var count int
	if err := a.DB.Pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("countRows query failed: %v", err)
	}
	return count
}

func hasEncryptedCredentials(t *testing.T, a *app.App, query string, args ...any) bool {
	t.Helper()
	var ok bool
	if err := a.DB.Pool.QueryRow(context.Background(), query, args...).Scan(&ok); err != nil {
		t.Fatalf("hasEncryptedCredentials query failed: %v", err)
	}
	return ok
}
