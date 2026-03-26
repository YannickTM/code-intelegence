//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"myjungle/backend-api/internal/app"
)

// seedCommitWithDiff inserts a commit and file diff directly into the database.
func seedCommitWithDiff(t *testing.T, a *app.App, projectID, commitHash, authorName, message, changeType, newFilePath, oldFilePath string, committerDate time.Time, additions, deletions int) {
	t.Helper()
	ctx := context.Background()

	var commitID string
	err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO commits (project_id, commit_hash, author_name, author_email, author_date, committer_name, committer_email, committer_date, message)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		projectID, commitHash, authorName, authorName+"@example.com",
		committerDate, authorName, authorName+"@example.com",
		committerDate, message,
	).Scan(&commitID)
	if err != nil {
		t.Fatalf("seedCommitWithDiff: insert commit: %v", err)
	}

	// Use NULL for empty paths to match the DB schema's CHECK constraints.
	var newPath, oldPath any
	if newFilePath != "" {
		newPath = newFilePath
	}
	if oldFilePath != "" {
		oldPath = oldFilePath
	}

	_, err = a.DB.Pool.Exec(ctx,
		`INSERT INTO commit_file_diffs (project_id, commit_id, new_file_path, old_file_path, change_type, additions, deletions)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		projectID, commitID, newPath, oldPath, changeType, additions, deletions,
	)
	if err != nil {
		t.Fatalf("seedCommitWithDiff: insert diff: %v", err)
	}
}

func TestFileHistory_Basic(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)
	proj := createProject(t, a, "test-project", "https://github.com/example/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	now := time.Now().UTC().Truncate(time.Second)

	// Seed 3 commits touching src/main.go with different dates.
	seedCommitWithDiff(t, a, projID, "aaa1111111111111", "Alice", "feat: initial commit", "added",
		"src/main.go", "", now.Add(-2*time.Hour), 100, 0)
	seedCommitWithDiff(t, a, projID, "bbb2222222222222", "Bob", "fix: bug fix\n\nDetailed description.", "modified",
		"src/main.go", "src/main.go", now.Add(-1*time.Hour), 10, 5)
	seedCommitWithDiff(t, a, projID, "ccc3333333333333", "Alice", "refactor: cleanup", "modified",
		"src/main.go", "src/main.go", now, 20, 15)

	// Also seed a commit touching a different file.
	seedCommitWithDiff(t, a, projID, "ddd4444444444444", "Bob", "docs: update readme", "modified",
		"README.md", "README.md", now, 5, 2)

	path := fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/main.go", projID)
	w := doRequest(t, a, http.MethodGet, path, nil, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)

	// Total should be 3 (only src/main.go commits).
	total, ok := m["total"].(float64)
	if !ok || int(total) != 3 {
		t.Errorf("total = %v, want 3", m["total"])
	}

	items, ok := m["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("items length = %d, want 3", len(items))
	}

	// Verify ordering: most recent first (ccc, bbb, aaa).
	first := items[0].(map[string]any)
	if first["short_hash"] != "ccc3333" {
		t.Errorf("first item short_hash = %v, want %q", first["short_hash"], "ccc3333")
	}
	if first["message_subject"] != "refactor: cleanup" {
		t.Errorf("first item message_subject = %v, want %q", first["message_subject"], "refactor: cleanup")
	}
	if first["change_type"] != "modified" {
		t.Errorf("first item change_type = %v, want %q", first["change_type"], "modified")
	}

	second := items[1].(map[string]any)
	if second["short_hash"] != "bbb2222" {
		t.Errorf("second item short_hash = %v, want %q", second["short_hash"], "bbb2222")
	}
	// Multi-line message: subject should be first line only.
	if second["message_subject"] != "fix: bug fix" {
		t.Errorf("second item message_subject = %v, want %q", second["message_subject"], "fix: bug fix")
	}

	third := items[2].(map[string]any)
	if third["short_hash"] != "aaa1111" {
		t.Errorf("third item short_hash = %v, want %q", third["short_hash"], "aaa1111")
	}

	// Verify response shape has required fields.
	for _, key := range []string{"diff_id", "commit_hash", "short_hash", "author_name", "committer_date", "message_subject", "change_type", "additions", "deletions"} {
		if _, exists := first[key]; !exists {
			t.Errorf("missing field %q in response item", key)
		}
	}
}

func TestFileHistory_Pagination(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)
	proj := createProject(t, a, "test-project", "https://github.com/example/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	now := time.Now().UTC().Truncate(time.Second)

	// Seed 3 commits.
	for i := 0; i < 3; i++ {
		hash := fmt.Sprintf("aaa%013d", i)
		seedCommitWithDiff(t, a, projID, hash, "Alice", fmt.Sprintf("commit %d", i), "modified",
			"src/app.go", "src/app.go", now.Add(time.Duration(-i)*time.Hour), i+1, i)
	}

	// Request with limit=1, offset=0.
	path := fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/app.go&limit=1&offset=0", projID)
	w := doRequest(t, a, http.MethodGet, path, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := decodeJSON(t, w)
	items := m["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items length = %d, want 1", len(items))
	}
	if total := int(m["total"].(float64)); total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if lim := int(m["limit"].(float64)); lim != 1 {
		t.Errorf("limit = %d, want 1", lim)
	}

	// Request with offset=2 → 1 item remaining.
	path = fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/app.go&limit=10&offset=2", projID)
	w = doRequest(t, a, http.MethodGet, path, nil, authHeader(token))
	m = decodeJSON(t, w)
	items = m["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items length = %d, want 1 (offset=2 of 3)", len(items))
	}
}

func TestFileHistory_MissingFilePath(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)
	proj := createProject(t, a, "test-project", "https://github.com/example/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	path := fmt.Sprintf("/v1/projects/%s/files/history", projID)
	w := doRequest(t, a, http.MethodGet, path, nil, authHeader(token))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestFileHistory_NoMatchingFile(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)
	proj := createProject(t, a, "test-project", "https://github.com/example/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	path := fmt.Sprintf("/v1/projects/%s/files/history?file_path=nonexistent.go", projID)
	w := doRequest(t, a, http.MethodGet, path, nil, authHeader(token))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := decodeJSON(t, w)
	items := m["items"].([]any)
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0 for nonexistent file", len(items))
	}
	if total := int(m["total"].(float64)); total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

func TestFileHistory_MatchesOldFilePath(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)
	proj := createProject(t, a, "test-project", "https://github.com/example/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	now := time.Now().UTC().Truncate(time.Second)

	// Simulate a rename: old_file_path=src/old.go, new_file_path=src/new.go.
	seedCommitWithDiff(t, a, projID, "ren1111111111111", "Alice", "refactor: rename file", "renamed",
		"src/new.go", "src/old.go", now, 0, 0)

	// Searching for old path should still find the rename entry.
	path := fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/old.go", projID)
	w := doRequest(t, a, http.MethodGet, path, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := decodeJSON(t, w)
	items := m["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items length = %d, want 1 (old_file_path match)", len(items))
	}

	// Searching for new path should also find it.
	path = fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/new.go", projID)
	w = doRequest(t, a, http.MethodGet, path, nil, authHeader(token))
	m = decodeJSON(t, w)
	items = m["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items length = %d, want 1 (new_file_path match)", len(items))
	}
}

func TestFileHistory_ProjectIsolation(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)

	projA := createProject(t, a, "project-a", "https://github.com/example/a.git", keyID, token)
	projAID := mustString(t, projA, "id")

	projB := createProject(t, a, "project-b", "https://github.com/example/b.git", keyID, token)
	projBID := mustString(t, projB, "id")

	now := time.Now().UTC().Truncate(time.Second)

	// Seed commit in project A only.
	seedCommitWithDiff(t, a, projAID, "aaa1111111111111", "Alice", "feat: add file", "added",
		"src/main.go", "", now, 50, 0)

	// Query project B — should be empty.
	path := fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/main.go", projBID)
	w := doRequest(t, a, http.MethodGet, path, nil, authHeader(token))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := decodeJSON(t, w)
	items := m["items"].([]any)
	if len(items) != 0 {
		t.Errorf("project B items = %d, want 0 (project isolation)", len(items))
	}

	// Query project A — should have 1 item.
	path = fmt.Sprintf("/v1/projects/%s/files/history?file_path=src/main.go", projAID)
	w = doRequest(t, a, http.MethodGet, path, nil, authHeader(token))
	m = decodeJSON(t, w)
	items = m["items"].([]any)
	if len(items) != 1 {
		t.Errorf("project A items = %d, want 1", len(items))
	}
}
