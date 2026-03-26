//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"myjungle/backend-api/internal/app"
)

// seedSymbolData creates a project with an active snapshot, files, and symbols
// for symbol-search integration tests. Returns the project ID and auth token.
func seedSymbolData(t *testing.T, a *app.App) (projectID, token string) {
	t.Helper()

	registerUser(t, a, "symuser")
	token = loginUser(t, a, "symuser")
	keyID := createSSHKey(t, a, "sym-key", token)
	proj := createProject(t, a, "sym-proj", "git@github.com:org/sym.git", keyID, token)
	projectID = mustString(t, proj, "id")

	ctx := context.Background()

	// embedding_provider_configs → embedding_versions → index_snapshots
	var embConfigID, embVersionID, snapID string
	err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO embedding_provider_configs (name, provider, endpoint_url, model, dimensions, is_active, project_id)
		 VALUES ('Test Embed', 'ollama', 'http://localhost:11434', 'test-model', 768, true, $1)
		 RETURNING id`, projectID).Scan(&embConfigID)
	if err != nil {
		t.Fatalf("insert embedding_provider_configs: %v", err)
	}

	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO embedding_versions (embedding_provider_config_id, provider, model, dimensions, version_label)
		 VALUES ($1, 'ollama', 'test-model', 768, 'v1')
		 RETURNING id`, embConfigID).Scan(&embVersionID)
	if err != nil {
		t.Fatalf("insert embedding_versions: %v", err)
	}

	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO index_snapshots (project_id, branch, embedding_version_id, git_commit, is_active, status, activated_at)
		 VALUES ($1, 'main', $2, 'aaa1111', true, 'active', NOW())
		 RETURNING id`, projectID, embVersionID).Scan(&snapID)
	if err != nil {
		t.Fatalf("insert index_snapshots: %v", err)
	}

	// Files in different directories
	files := []struct {
		path     string
		language string
	}{
		{"src/api/handler.go", "go"},
		{"src/api/router.go", "go"},
		{"src/model/user.go", "go"},
		{"src/model/project.go", "go"},
		{"tests/api/handler_test.go", "go"},
		{"lib/utils.ts", "typescript"},
		{"lib/helpers.ts", "typescript"},
	}

	fileIDs := make(map[string]string)
	for _, f := range files {
		var fID string
		err = a.DB.Pool.QueryRow(ctx,
			`INSERT INTO files (project_id, index_snapshot_id, file_path, language, file_hash)
			 VALUES ($1, $2, $3, $4, 'hash-' || $3)
			 RETURNING id`, projectID, snapID, f.path, f.language).Scan(&fID)
		if err != nil {
			t.Fatalf("insert file %s: %v", f.path, err)
		}
		fileIDs[f.path] = fID
	}

	// Symbols across files
	symbols := []struct {
		name      string
		kind      string
		filePath  string
		startLine int
	}{
		{"HandleListUsers", "function", "src/api/handler.go", 10},
		{"HandleGetUser", "function", "src/api/handler.go", 50},
		{"handleListUsers", "function", "src/api/handler.go", 90}, // lowercase variant
		{"NewRouter", "function", "src/api/router.go", 5},
		{"User", "class", "src/model/user.go", 1},
		{"UserName", "field", "src/model/user.go", 3},
		{"Project", "class", "src/model/project.go", 1},
		{"ProjectName", "field", "src/model/project.go", 3},
		{"TestHandleListUsers", "function", "tests/api/handler_test.go", 1},
		{"formatDate", "function", "lib/utils.ts", 10},
		{"parseJSON", "function", "lib/helpers.ts", 5},
	}

	for _, s := range symbols {
		_, err = a.DB.Pool.Exec(ctx,
			`INSERT INTO symbols (project_id, index_snapshot_id, file_id, name, kind, start_line, symbol_hash)
			 VALUES ($1, $2, $3, $4, $5, $6, 'sym-' || $4)`,
			projectID, snapID, fileIDs[s.filePath], s.name, s.kind, s.startLine)
		if err != nil {
			t.Fatalf("insert symbol %s: %v", s.name, err)
		}
	}

	return projectID, token
}

func symbolsURL(projectID string) string {
	return fmt.Sprintf("/v1/projects/%s/symbols", projectID)
}

// symbolItems extracts the items array from a symbol list response.
func symbolItems(t *testing.T, m map[string]any) []any {
	t.Helper()
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", m["items"])
	}
	return items
}

// symbolNames collects symbol names from the items array.
func symbolNames(t *testing.T, items []any) []string {
	t.Helper()
	var names []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected item as map, got %T", item)
		}
		names = append(names, m["name"].(string))
	}
	return names
}

// ---------- Basic endpoint tests ----------

func TestSymbol_ListAll(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	total := m["total"].(float64)

	if len(items) != 11 {
		t.Errorf("expected 11 items, got %d", len(items))
	}
	if total != 11 {
		t.Errorf("expected total=11, got %v", total)
	}
	if m["snapshot_id"] == nil || m["snapshot_id"] == "" {
		t.Error("snapshot_id should be set")
	}
}

func TestSymbol_NoActiveSnapshot(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "nosnap")
	token := loginUser(t, a, "nosnap")
	keyID := createSSHKey(t, a, "nosnap-key", token)
	proj := createProject(t, a, "nosnap-proj", "git@github.com:org/nosnap.git", keyID, token)
	projID := mustString(t, proj, "id")

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	if len(items) != 0 {
		t.Errorf("expected 0 items for project with no snapshot, got %d", len(items))
	}
}

// ---------- Name filter tests ----------

func TestSymbol_NameFilter_Insensitive(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// "HandleListUsers" should match both cases (HandleListUsers + handleListUsers)
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=HandleListUsers", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	names := symbolNames(t, items)

	// Should find HandleListUsers, handleListUsers, and TestHandleListUsers (contains substring)
	if len(items) < 2 {
		t.Errorf("expected at least 2 insensitive matches, got %d: %v", len(items), names)
	}

	// Exact match should be ranked first (case-insensitive)
	if len(names) > 0 && names[0] != "HandleListUsers" {
		t.Errorf("expected exact match first, got %q", names[0])
	}
}

func TestSymbol_NameFilter_Sensitive(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Case-sensitive: "handleListUsers" should NOT match "HandleListUsers"
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=handleListUsers&search_mode=sensitive", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	names := symbolNames(t, items)

	for _, n := range names {
		if n == "HandleListUsers" {
			t.Error("case-sensitive search should NOT match HandleListUsers for query handleListUsers")
		}
	}

	// Should find the lowercase variant
	found := false
	for _, n := range names {
		if n == "handleListUsers" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected handleListUsers in results, got %v", names)
	}
}

func TestSymbol_NameFilter_Regex(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Regex: names starting with "Handle"
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=^Handle&search_mode=regex", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	names := symbolNames(t, items)

	if len(items) != 2 {
		t.Errorf("expected 2 regex matches (HandleListUsers, HandleGetUser), got %d: %v", len(items), names)
	}
	for _, n := range names {
		if n == "handleListUsers" || n == "TestHandleListUsers" {
			t.Errorf("regex ^Handle should not match %q", n)
		}
	}
}

func TestSymbol_NameFilter_InvalidRegex(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=[invalid&search_mode=regex", nil, authHeader(token))
	if w.Code != 422 {
		t.Errorf("expected 422 for invalid regex, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestSymbol_InvalidSearchMode(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?search_mode=fuzzy", nil, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid search_mode, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// ---------- Kind filter tests ----------

func TestSymbol_KindFilter(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?kind=class", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	if len(items) != 2 {
		t.Errorf("expected 2 classes (User, Project), got %d", len(items))
	}
	for _, item := range items {
		im := item.(map[string]any)
		if im["kind"] != "class" {
			t.Errorf("expected kind=class, got %v", im["kind"])
		}
	}
}

// ---------- Directory filter tests ----------

func TestSymbol_IncludeDir(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Only symbols under src/api/
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?include_dir=src/api", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	// src/api/ has handler.go (3 symbols) and router.go (1 symbol) = 4
	if len(items) != 4 {
		names := symbolNames(t, items)
		t.Errorf("expected 4 symbols in src/api/, got %d: %v", len(items), names)
	}
}

func TestSymbol_IncludeDir_Multiple(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// OR logic: src/api OR lib
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?include_dir=src/api,lib", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	// src/api/ = 4 symbols, lib/ = 2 symbols = 6
	if len(items) != 6 {
		names := symbolNames(t, items)
		t.Errorf("expected 6 symbols in src/api + lib, got %d: %v", len(items), names)
	}
}

func TestSymbol_ExcludeDir(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Exclude tests/ directory
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?exclude_dir=tests", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	// 11 total - 1 in tests/ = 10
	if len(items) != 10 {
		names := symbolNames(t, items)
		t.Errorf("expected 10 symbols (excluding tests/), got %d: %v", len(items), names)
	}

	// Verify no test file symbols are present
	for _, item := range items {
		im := item.(map[string]any)
		fp := im["file_path"].(string)
		if fp == "tests/api/handler_test.go" {
			t.Error("excluded file should not appear in results")
		}
	}
}

func TestSymbol_IncludeAndExclude(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Include src/ but exclude src/model/
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?include_dir=src&exclude_dir=src/model", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	// src/api/ = 4 symbols (src/model excluded)
	if len(items) != 4 {
		names := symbolNames(t, items)
		t.Errorf("expected 4 symbols (src/ minus src/model/), got %d: %v", len(items), names)
	}
}

func TestSymbol_IncludeDir_GlobPattern(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Glob: any .ts files
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?include_dir=*.ts", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	// lib/utils.ts and lib/helpers.ts = 2 symbols
	if len(items) != 2 {
		names := symbolNames(t, items)
		t.Errorf("expected 2 .ts symbols, got %d: %v", len(items), names)
	}
}

func TestSymbol_IncludeDir_DoubleStarGlob(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// **/*_test.go — test files at any depth
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?include_dir=**/*_test.go", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	if len(items) != 1 {
		names := symbolNames(t, items)
		t.Errorf("expected 1 test file symbol, got %d: %v", len(items), names)
	}
}

// ---------- Combined filter tests ----------

func TestSymbol_NameAndKindCombined(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Name containing "User" + kind=class
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=User&kind=class", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)

	// Only "User" class matches (not UserName which is a field)
	if len(items) != 1 {
		names := symbolNames(t, items)
		t.Errorf("expected 1 class named User, got %d: %v", len(items), names)
	}
}

func TestSymbol_NameAndIncludeDir(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Name containing "Handle" but only in src/api/
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=Handle&include_dir=src/api", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	names := symbolNames(t, items)

	// HandleListUsers, HandleGetUser, handleListUsers — all in src/api/
	// TestHandleListUsers is in tests/ so excluded
	if len(items) != 3 {
		t.Errorf("expected 3 Handle* symbols in src/api/, got %d: %v", len(items), names)
	}
}

// ---------- Pagination tests ----------

func TestSymbol_Pagination(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Page 1: limit=3
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?limit=3&offset=0", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	total := m["total"].(float64)

	if len(items) != 3 {
		t.Errorf("expected 3 items on page 1, got %d", len(items))
	}
	if total != 11 {
		t.Errorf("total should be 11 regardless of limit, got %v", total)
	}

	// Page 2
	w2 := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?limit=3&offset=3", nil, authHeader(token))
	m2 := decodeJSON(t, w2)
	items2 := symbolItems(t, m2)
	if len(items2) != 3 {
		t.Errorf("expected 3 items on page 2, got %d", len(items2))
	}
}

// ---------- Validation error tests ----------

func TestSymbol_IncludeDir_DotDot(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?include_dir=../etc", nil, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for '..' in include_dir, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestSymbol_ExcludeDir_AbsolutePath(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?exclude_dir=/etc/passwd", nil, authHeader(token))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for absolute path in exclude_dir, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// ---------- V2 field tests ----------

// seedSymbolWithV2 seeds a single symbol with v2 fields populated and returns its ID.
func seedSymbolWithV2(t *testing.T, a *app.App, projectID, snapID, fileID string) string {
	t.Helper()
	var symID string
	err := a.DB.Pool.QueryRow(context.Background(),
		`INSERT INTO symbols (project_id, index_snapshot_id, file_id, name, kind, start_line, symbol_hash,
		    flags, modifiers, return_type, parameter_types)
		 VALUES ($1, $2, $3, 'fetchData', 'function', 20, 'sym-fetchData',
		    '{"is_exported":true,"is_async":true,"is_default_export":false,"is_generator":false,"is_static":false,"is_abstract":false,"is_readonly":false,"is_optional":false,"is_arrow_function":false,"is_react_component_like":false,"is_hook_like":false}'::jsonb,
		    ARRAY['async','export'],
		    'Promise<Response>',
		    ARRAY['string','RequestInit'])
		 RETURNING id`,
		projectID, snapID, fileID).Scan(&symID)
	if err != nil {
		t.Fatalf("insert v2 symbol: %v", err)
	}
	return symID
}

// seedSymbolDataV2 creates the standard seed data plus one symbol with v2 fields.
// Returns projectID, token, snapshotID, fileID (for the v2 symbol's file), and the v2 symbol ID.
func seedSymbolDataV2(t *testing.T, a *app.App) (projectID, token, v2SymbolID string) {
	t.Helper()

	registerUser(t, a, "symv2user")
	token = loginUser(t, a, "symv2user")
	keyID := createSSHKey(t, a, "symv2-key", token)
	proj := createProject(t, a, "symv2-proj", "git@github.com:org/symv2.git", keyID, token)
	projectID = mustString(t, proj, "id")

	ctx := context.Background()

	var embConfigID, embVersionID, snapID string
	err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO embedding_provider_configs (name, provider, endpoint_url, model, dimensions, is_active, project_id)
		 VALUES ('Test Embed V2', 'ollama', 'http://localhost:11434', 'test-model', 768, true, $1)
		 RETURNING id`, projectID).Scan(&embConfigID)
	if err != nil {
		t.Fatalf("insert embedding_provider_configs: %v", err)
	}
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO embedding_versions (embedding_provider_config_id, provider, model, dimensions, version_label)
		 VALUES ($1, 'ollama', 'test-model', 768, 'v1')
		 RETURNING id`, embConfigID).Scan(&embVersionID)
	if err != nil {
		t.Fatalf("insert embedding_versions: %v", err)
	}
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO index_snapshots (project_id, branch, embedding_version_id, git_commit, is_active, status, activated_at)
		 VALUES ($1, 'main', $2, 'bbb2222', true, 'active', NOW())
		 RETURNING id`, projectID, embVersionID).Scan(&snapID)
	if err != nil {
		t.Fatalf("insert index_snapshots: %v", err)
	}

	var fileID string
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO files (project_id, index_snapshot_id, file_path, language, file_hash)
		 VALUES ($1, $2, 'src/api/client.ts', 'typescript', 'hash-client')
		 RETURNING id`, projectID, snapID).Scan(&fileID)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}

	// Symbol WITHOUT v2 fields
	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO symbols (project_id, index_snapshot_id, file_id, name, kind, start_line, symbol_hash)
		 VALUES ($1, $2, $3, 'plainFunc', 'function', 1, 'sym-plainFunc')`,
		projectID, snapID, fileID)
	if err != nil {
		t.Fatalf("insert plain symbol: %v", err)
	}

	// Symbol WITH v2 fields
	v2SymbolID = seedSymbolWithV2(t, a, projectID, snapID, fileID)

	return projectID, token, v2SymbolID
}

func TestSymbol_V2Fields_InListResponse(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token, _ := seedSymbolDataV2(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=fetchData", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)

	// flags must be a JSON object, not a string
	flags, ok := item["flags"].(map[string]any)
	if !ok {
		t.Fatalf("expected flags to be a JSON object, got %T: %v", item["flags"], item["flags"])
	}
	if flags["is_exported"] != true {
		t.Errorf("expected flags.is_exported=true, got %v", flags["is_exported"])
	}
	if flags["is_async"] != true {
		t.Errorf("expected flags.is_async=true, got %v", flags["is_async"])
	}

	// modifiers must be a non-null array
	mods, ok := item["modifiers"].([]any)
	if !ok {
		t.Fatalf("expected modifiers to be an array, got %T", item["modifiers"])
	}
	if len(mods) != 2 {
		t.Errorf("expected 2 modifiers, got %d", len(mods))
	}

	// return_type
	rt, ok := item["return_type"].(string)
	if !ok || rt != "Promise<Response>" {
		t.Errorf("expected return_type='Promise<Response>', got %v", item["return_type"])
	}

	// parameter_types must be a non-null array
	pts, ok := item["parameter_types"].([]any)
	if !ok {
		t.Fatalf("expected parameter_types to be an array, got %T", item["parameter_types"])
	}
	if len(pts) != 2 {
		t.Errorf("expected 2 parameter_types, got %d", len(pts))
	}
}

func TestSymbol_V2Fields_EmptyWhenNull(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token, _ := seedSymbolDataV2(t, a)

	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=plainFunc", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)

	// flags should be absent (omitempty with nil)
	if _, exists := item["flags"]; exists {
		t.Errorf("expected flags to be omitted for null, got %v", item["flags"])
	}

	// return_type should be absent (omitempty with nil)
	if _, exists := item["return_type"]; exists {
		t.Errorf("expected return_type to be omitted for null, got %v", item["return_type"])
	}

	// modifiers must be empty array [], not null or absent
	mods, ok := item["modifiers"].([]any)
	if !ok {
		t.Fatalf("expected modifiers to be an array, got %T (%v)", item["modifiers"], item["modifiers"])
	}
	if len(mods) != 0 {
		t.Errorf("expected empty modifiers, got %v", mods)
	}

	// parameter_types must be empty array [], not null or absent
	pts, ok := item["parameter_types"].([]any)
	if !ok {
		t.Fatalf("expected parameter_types to be an array, got %T (%v)", item["parameter_types"], item["parameter_types"])
	}
	if len(pts) != 0 {
		t.Errorf("expected empty parameter_types, got %v", pts)
	}
}

func TestSymbol_V2Fields_InGetResponse(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token, v2SymID := seedSymbolDataV2(t, a)

	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/symbols/%s", projID, v2SymID), nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	item := decodeJSON(t, w)

	// flags must be a JSON object
	flags, ok := item["flags"].(map[string]any)
	if !ok {
		t.Fatalf("expected flags to be a JSON object, got %T: %v", item["flags"], item["flags"])
	}
	if flags["is_exported"] != true {
		t.Errorf("expected flags.is_exported=true, got %v", flags["is_exported"])
	}

	// modifiers
	mods, ok := item["modifiers"].([]any)
	if !ok {
		t.Fatalf("expected modifiers to be an array, got %T", item["modifiers"])
	}
	if len(mods) != 2 {
		t.Errorf("expected 2 modifiers, got %d", len(mods))
	}

	// return_type
	if item["return_type"] != "Promise<Response>" {
		t.Errorf("expected return_type='Promise<Response>', got %v", item["return_type"])
	}

	// parameter_types
	pts, ok := item["parameter_types"].([]any)
	if !ok {
		t.Fatalf("expected parameter_types to be an array, got %T", item["parameter_types"])
	}
	if len(pts) != 2 {
		t.Errorf("expected 2 parameter_types, got %d", len(pts))
	}
}

// ---------- Count query correctness test ----------

func TestSymbol_CountMatchesItems(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)
	projID, token := seedSymbolData(t, a)

	// Use a filter that returns fewer than default limit so total == len(items)
	w := doRequest(t, a, http.MethodGet, symbolsURL(projID)+"?name=Handle&search_mode=insensitive", nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}

	m := decodeJSON(t, w)
	items := symbolItems(t, m)
	total := int(m["total"].(float64))

	// When all results fit in one page, total must equal len(items)
	if total != len(items) {
		t.Errorf("total (%d) != len(items) (%d) — count query may have arg mismatch", total, len(items))
	}
}
