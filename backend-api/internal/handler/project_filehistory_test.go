package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"
)

// ---------------------------------------------------------------------------
// toFileHistoryEntry
// ---------------------------------------------------------------------------

func TestToFileHistoryEntry_ShortHashAndSubject(t *testing.T) {
	row := db.ListFileDiffsByPathWithCommitRow{
		DiffID:        testUUID(1),
		CommitHash:    "a1b2c3d4e5f6789",
		AuthorName:    "Alice",
		CommitterDate: testTimestamptz(time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)),
		Message:       "feat: add login page\n\nLonger description here.",
		ChangeType:    "modified",
		Additions:     42,
		Deletions:     7,
	}

	entry := toFileHistoryEntry(row)

	if entry.ShortHash != "a1b2c3d" {
		t.Errorf("ShortHash = %q, want %q", entry.ShortHash, "a1b2c3d")
	}
	if entry.MessageSubject != "feat: add login page" {
		t.Errorf("MessageSubject = %q, want %q", entry.MessageSubject, "feat: add login page")
	}
	if entry.CommitHash != "a1b2c3d4e5f6789" {
		t.Errorf("CommitHash = %q, want full hash", entry.CommitHash)
	}
	if entry.AuthorName != "Alice" {
		t.Errorf("AuthorName = %q, want %q", entry.AuthorName, "Alice")
	}
	if entry.ChangeType != "modified" {
		t.Errorf("ChangeType = %q, want %q", entry.ChangeType, "modified")
	}
	if entry.Additions != 42 {
		t.Errorf("Additions = %d, want 42", entry.Additions)
	}
	if entry.Deletions != 7 {
		t.Errorf("Deletions = %d, want 7", entry.Deletions)
	}
	if entry.CommitterDate != "2025-01-15T10:30:00Z" {
		t.Errorf("CommitterDate = %q, want RFC3339", entry.CommitterDate)
	}
}

func TestToFileHistoryEntry_ShortHashUnderSeven(t *testing.T) {
	row := db.ListFileDiffsByPathWithCommitRow{
		DiffID:        testUUID(2),
		CommitHash:    "abc",
		AuthorName:    "Bob",
		CommitterDate: testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		Message:       "short",
		ChangeType:    "added",
	}
	entry := toFileHistoryEntry(row)
	if entry.ShortHash != "abc" {
		t.Errorf("ShortHash = %q, want %q (no truncation)", entry.ShortHash, "abc")
	}
}

func TestToFileHistoryEntry_SingleLineMessage(t *testing.T) {
	row := db.ListFileDiffsByPathWithCommitRow{
		DiffID:        testUUID(3),
		CommitHash:    "abcdef1234567890",
		AuthorName:    "Alice",
		CommitterDate: testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		Message:       "fix: typo in readme",
		ChangeType:    "modified",
	}
	entry := toFileHistoryEntry(row)
	if entry.MessageSubject != "fix: typo in readme" {
		t.Errorf("MessageSubject = %q, want %q", entry.MessageSubject, "fix: typo in readme")
	}
}

// ---------------------------------------------------------------------------
// HandleFileHistory validation
// ---------------------------------------------------------------------------

func TestHandleFileHistory_MissingFilePath(t *testing.T) {
	h := &ProjectHandler{db: &postgres.DB{Queries: db.New(nil)}}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/123/files/history", nil)
	req = withChiParams(req, map[string]string{"projectID": "00000000-0000-0000-0000-000000000001"})
	w := httptest.NewRecorder()

	h.HandleFileHistory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	body := mustDecodeJSON(t, w.Body)
	if msg, ok := body["error"].(string); !ok || msg != "file_path query param required" {
		t.Errorf("error = %v, want %q", body["error"], "file_path query param required")
	}
}
