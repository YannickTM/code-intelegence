package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// helper to build a pgtype.UUID from raw bytes.
func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Valid = true
	u.Bytes[15] = b
	return u
}

func testTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// ---------------------------------------------------------------------------
// toCommitSummary
// ---------------------------------------------------------------------------

func TestToCommitSummary_ShortHashAndSubject(t *testing.T) {
	c := db.Commit{
		ID:            testUUID(1),
		CommitHash:    "a1b2c3d4e5f6789",
		AuthorName:    "Alice",
		AuthorEmail:   "alice@example.com",
		AuthorDate:    testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		CommitterName: "Bob",
		CommitterEmail: "bob@example.com",
		CommitterDate: testTimestamptz(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
		Message:       "feat: add commit browser\n\nLonger description here.",
	}
	s := toCommitSummary(c, false)

	if s.ShortHash != "a1b2c3d" {
		t.Errorf("ShortHash = %q, want %q", s.ShortHash, "a1b2c3d")
	}
	if s.MessageSubject != "feat: add commit browser" {
		t.Errorf("MessageSubject = %q, want %q", s.MessageSubject, "feat: add commit browser")
	}
	if s.Message != c.Message {
		t.Errorf("Message should be the full message")
	}
}

func TestToCommitSummary_SingleLineMessage(t *testing.T) {
	c := db.Commit{
		ID:            testUUID(2),
		CommitHash:    "abcdef1234567890",
		AuthorName:    "Alice",
		AuthorDate:    testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		CommitterName: "Alice",
		CommitterDate: testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		Message:       "fix: typo in readme",
	}
	s := toCommitSummary(c, false)

	if s.MessageSubject != "fix: typo in readme" {
		t.Errorf("MessageSubject = %q, want %q", s.MessageSubject, "fix: typo in readme")
	}
}

func TestToCommitSummary_ShortHashUnderSeven(t *testing.T) {
	c := db.Commit{
		ID:            testUUID(3),
		CommitHash:    "abc",
		AuthorName:    "Alice",
		AuthorDate:    testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		CommitterName: "Alice",
		CommitterDate: testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		Message:       "short",
	}
	s := toCommitSummary(c, false)

	if s.ShortHash != "abc" {
		t.Errorf("ShortHash = %q, want %q (should not truncate short hashes)", s.ShortHash, "abc")
	}
}

func TestToCommitSummary_ShowEmails(t *testing.T) {
	c := db.Commit{
		ID:             testUUID(4),
		CommitHash:     "a1b2c3d4e5f6789",
		AuthorName:     "Alice",
		AuthorEmail:    "alice@example.com",
		AuthorDate:     testTimestamptz(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		CommitterName:  "Bob",
		CommitterEmail: "bob@example.com",
		CommitterDate:  testTimestamptz(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
		Message:        "test",
	}

	// showEmails=false → emails should be empty
	s := toCommitSummary(c, false)
	if s.AuthorEmail != "" {
		t.Errorf("AuthorEmail = %q, want empty when showEmails=false", s.AuthorEmail)
	}
	if s.CommitterEmail != "" {
		t.Errorf("CommitterEmail = %q, want empty when showEmails=false", s.CommitterEmail)
	}

	// showEmails=true → emails should be populated
	s = toCommitSummary(c, true)
	if s.AuthorEmail != "alice@example.com" {
		t.Errorf("AuthorEmail = %q, want %q", s.AuthorEmail, "alice@example.com")
	}
	if s.CommitterEmail != "bob@example.com" {
		t.Errorf("CommitterEmail = %q, want %q", s.CommitterEmail, "bob@example.com")
	}
}

func TestToCommitSummary_DateFormat(t *testing.T) {
	ts := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	c := db.Commit{
		ID:            testUUID(5),
		CommitHash:    "a1b2c3d4e5f6789",
		AuthorName:    "Alice",
		AuthorDate:    testTimestamptz(ts),
		CommitterName: "Alice",
		CommitterDate: testTimestamptz(ts),
		Message:       "test",
	}
	s := toCommitSummary(c, false)

	want := "2025-03-15T14:30:00Z"
	if s.AuthorDate != want {
		t.Errorf("AuthorDate = %q, want %q", s.AuthorDate, want)
	}
	if s.CommitterDate != want {
		t.Errorf("CommitterDate = %q, want %q", s.CommitterDate, want)
	}
}

// ---------------------------------------------------------------------------
// toCommitParents
// ---------------------------------------------------------------------------

func TestToCommitParents(t *testing.T) {
	parents := []db.GetCommitParentsRow{
		{
			ParentCommitID: testUUID(10),
			ParentHash:     "1234567890abcdef",
			Ordinal:        0,
		},
		{
			ParentCommitID: testUUID(11),
			ParentHash:     "fedcba0987654321",
			Ordinal:        1,
		},
	}

	out := toCommitParents(parents)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}

	if out[0].ParentShortHash != "1234567" {
		t.Errorf("[0].ParentShortHash = %q, want %q", out[0].ParentShortHash, "1234567")
	}
	if out[0].ParentCommitHash != "1234567890abcdef" {
		t.Errorf("[0].ParentCommitHash = %q, want full hash", out[0].ParentCommitHash)
	}
	if out[0].Ordinal != 0 {
		t.Errorf("[0].Ordinal = %d, want 0", out[0].Ordinal)
	}
	if out[1].ParentShortHash != "fedcba0" {
		t.Errorf("[1].ParentShortHash = %q, want %q", out[1].ParentShortHash, "fedcba0")
	}
	if out[1].Ordinal != 1 {
		t.Errorf("[1].Ordinal = %d, want 1", out[1].Ordinal)
	}
}

func TestToCommitParents_Empty(t *testing.T) {
	out := toCommitParents(nil)
	if len(out) != 0 {
		t.Fatalf("len = %d, want 0", len(out))
	}
}

// ---------------------------------------------------------------------------
// toFileDiff
// ---------------------------------------------------------------------------

func TestToFileDiff_NoPatch(t *testing.T) {
	d := db.CommitFileDiff{
		ID:             testUUID(20),
		OldFilePath:    pgtype.Text{String: "old.go", Valid: true},
		NewFilePath:    pgtype.Text{String: "new.go", Valid: true},
		ChangeType:     "renamed",
		Patch:          pgtype.Text{String: "diff content", Valid: true},
		Additions:      10,
		Deletions:      5,
		ParentCommitID: testUUID(30),
	}

	fd := toFileDiff(d, false)

	if fd.Patch != nil {
		t.Errorf("Patch should be nil when includePatch=false")
	}
	if fd.OldFilePath == nil || *fd.OldFilePath != "old.go" {
		t.Errorf("OldFilePath = %v, want 'old.go'", fd.OldFilePath)
	}
	if fd.NewFilePath == nil || *fd.NewFilePath != "new.go" {
		t.Errorf("NewFilePath = %v, want 'new.go'", fd.NewFilePath)
	}
	if fd.ChangeType != "renamed" {
		t.Errorf("ChangeType = %q, want %q", fd.ChangeType, "renamed")
	}
	if fd.Additions != 10 {
		t.Errorf("Additions = %d, want 10", fd.Additions)
	}
	if fd.Deletions != 5 {
		t.Errorf("Deletions = %d, want 5", fd.Deletions)
	}
	if fd.ParentCommitID == nil {
		t.Errorf("ParentCommitID should not be nil")
	}
}

func TestToFileDiff_WithPatch(t *testing.T) {
	d := db.CommitFileDiff{
		ID:          testUUID(21),
		NewFilePath: pgtype.Text{String: "added.go", Valid: true},
		ChangeType:  "added",
		Patch:       pgtype.Text{String: "@@ -0,0 +1,5 @@\n+package main", Valid: true},
		Additions:   5,
		Deletions:   0,
	}

	fd := toFileDiff(d, true)

	if fd.Patch == nil {
		t.Fatal("Patch should not be nil when includePatch=true")
	}
	if *fd.Patch != "@@ -0,0 +1,5 @@\n+package main" {
		t.Errorf("Patch = %q, want the diff content", *fd.Patch)
	}
	if fd.OldFilePath != nil {
		t.Errorf("OldFilePath should be nil for added files")
	}
	if fd.ParentCommitID != nil {
		t.Errorf("ParentCommitID should be nil when not valid")
	}
}

func TestToFileDiff_NullPatchWithInclude(t *testing.T) {
	d := db.CommitFileDiff{
		ID:          testUUID(22),
		NewFilePath: pgtype.Text{String: "binary.png", Valid: true},
		ChangeType:  "modified",
		Patch:       pgtype.Text{Valid: false}, // NULL patch (binary file)
		Additions:   0,
		Deletions:   0,
	}

	fd := toFileDiff(d, true)

	if fd.Patch != nil {
		t.Errorf("Patch should be nil for NULL patch even when includePatch=true")
	}
}

func TestToFileDiff_NullablePaths(t *testing.T) {
	// Deleted file: only old_file_path set
	d := db.CommitFileDiff{
		ID:          testUUID(23),
		OldFilePath: pgtype.Text{String: "removed.go", Valid: true},
		NewFilePath: pgtype.Text{Valid: false},
		ChangeType:  "deleted",
		Additions:   0,
		Deletions:   42,
	}

	fd := toFileDiff(d, false)

	if fd.OldFilePath == nil || *fd.OldFilePath != "removed.go" {
		t.Errorf("OldFilePath = %v, want 'removed.go'", fd.OldFilePath)
	}
	if fd.NewFilePath != nil {
		t.Errorf("NewFilePath should be nil for deleted files")
	}
}

// ---------------------------------------------------------------------------
// toFileDiffFromMeta
// ---------------------------------------------------------------------------

func TestToFileDiffFromMeta(t *testing.T) {
	d := db.ListCommitFileDiffsMetaRow{
		ID:             testUUID(24),
		OldFilePath:    pgtype.Text{String: "old.go", Valid: true},
		NewFilePath:    pgtype.Text{String: "new.go", Valid: true},
		ChangeType:     "renamed",
		Additions:      10,
		Deletions:      5,
		ParentCommitID: testUUID(30),
	}

	fd := toFileDiffFromMeta(d)

	if fd.Patch != nil {
		t.Errorf("Patch should always be nil from meta row")
	}
	if fd.OldFilePath == nil || *fd.OldFilePath != "old.go" {
		t.Errorf("OldFilePath = %v, want 'old.go'", fd.OldFilePath)
	}
	if fd.NewFilePath == nil || *fd.NewFilePath != "new.go" {
		t.Errorf("NewFilePath = %v, want 'new.go'", fd.NewFilePath)
	}
	if fd.ChangeType != "renamed" {
		t.Errorf("ChangeType = %q, want %q", fd.ChangeType, "renamed")
	}
	if fd.Additions != 10 {
		t.Errorf("Additions = %d, want 10", fd.Additions)
	}
	if fd.Deletions != 5 {
		t.Errorf("Deletions = %d, want 5", fd.Deletions)
	}
	if fd.ParentCommitID == nil {
		t.Errorf("ParentCommitID should not be nil")
	}
}

// ---------------------------------------------------------------------------
// parsePagination
// ---------------------------------------------------------------------------

func TestParsePagination_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	limit, offset, err := parsePagination(r, 20, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 20 {
		t.Errorf("limit = %d, want 20", limit)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
}

func TestParsePagination_CustomValues(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?limit=50&offset=10", nil)
	limit, offset, err := parsePagination(r, 20, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 50 {
		t.Errorf("limit = %d, want 50", limit)
	}
	if offset != 10 {
		t.Errorf("offset = %d, want 10", offset)
	}
}

func TestParsePagination_ClampsToMax(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?limit=999", nil)
	limit, _, err := parsePagination(r, 20, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 100 {
		t.Errorf("limit = %d, want 100 (clamped)", limit)
	}
}

func TestParsePagination_IgnoresInvalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?limit=-5&offset=abc", nil)
	limit, offset, err := parsePagination(r, 20, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 20 {
		t.Errorf("limit = %d, want 20 (default on invalid)", limit)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0 (default on invalid)", offset)
	}
}
