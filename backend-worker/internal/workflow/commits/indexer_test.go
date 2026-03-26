package commits

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/gitclient"
	db "myjungle/datastore/postgres/sqlc"
)

// --- test helpers ---

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

// --- mock git provider ---

type mockGitProvider struct {
	logCommitsFn  func(ctx context.Context, repoDir, sinceCommit string, maxCount int) ([]gitclient.CommitLog, error)
	diffStatLogFn func(ctx context.Context, repoDir, sinceCommit string, maxCount int) (map[string][]gitclient.FileDiffEntry, error)
}

func (m *mockGitProvider) LogCommits(ctx context.Context, repoDir, sinceCommit string, maxCount int) ([]gitclient.CommitLog, error) {
	if m.logCommitsFn != nil {
		return m.logCommitsFn(ctx, repoDir, sinceCommit, maxCount)
	}
	return nil, nil
}

func (m *mockGitProvider) DiffStatLog(ctx context.Context, repoDir, sinceCommit string, maxCount int) (map[string][]gitclient.FileDiffEntry, error) {
	if m.diffStatLogFn != nil {
		return m.diffStatLogFn(ctx, repoDir, sinceCommit, maxCount)
	}
	return nil, nil
}

// --- mock querier ---

type mockQuerier struct {
	db.Querier // embed to satisfy interface for unused methods

	insertCommitFn      func(ctx context.Context, arg db.InsertCommitParams) (db.Commit, error)
	insertCommitCalls   []db.InsertCommitParams
	insertParentFn      func(ctx context.Context, arg db.InsertCommitParentParams) error
	insertParentCalls   []db.InsertCommitParentParams
	insertFileDiffFn    func(ctx context.Context, arg db.InsertCommitFileDiffParams) (db.CommitFileDiff, error)
	insertFileDiffCalls []db.InsertCommitFileDiffParams
	getCommitByHashFn   func(ctx context.Context, arg db.GetCommitByHashParams) (db.Commit, error)
}

func (m *mockQuerier) InsertCommit(ctx context.Context, arg db.InsertCommitParams) (db.Commit, error) {
	m.insertCommitCalls = append(m.insertCommitCalls, arg)
	if m.insertCommitFn != nil {
		return m.insertCommitFn(ctx, arg)
	}
	return db.Commit{ID: testUUID(byte(len(m.insertCommitCalls)))}, nil
}

func (m *mockQuerier) InsertCommitParent(ctx context.Context, arg db.InsertCommitParentParams) error {
	m.insertParentCalls = append(m.insertParentCalls, arg)
	if m.insertParentFn != nil {
		return m.insertParentFn(ctx, arg)
	}
	return nil
}

func (m *mockQuerier) InsertCommitFileDiff(ctx context.Context, arg db.InsertCommitFileDiffParams) (db.CommitFileDiff, error) {
	m.insertFileDiffCalls = append(m.insertFileDiffCalls, arg)
	if m.insertFileDiffFn != nil {
		return m.insertFileDiffFn(ctx, arg)
	}
	return db.CommitFileDiff{}, nil
}

func (m *mockQuerier) GetCommitByHash(ctx context.Context, arg db.GetCommitByHashParams) (db.Commit, error) {
	if m.getCommitByHashFn != nil {
		return m.getCommitByHashFn(ctx, arg)
	}
	return db.Commit{}, pgx.ErrNoRows
}

// --- sample data ---

var (
	sampleProjectID = testUUID(0xAA)
	sampleDate      = time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
)

func sampleCommits() []gitclient.CommitLog {
	return []gitclient.CommitLog{
		{
			Hash:           "aaa111",
			ParentHashes:   []string{"bbb222"},
			AuthorName:     "Alice",
			AuthorEmail:    "alice@example.com",
			AuthorDate:     sampleDate,
			CommitterName:  "Alice",
			CommitterEmail: "alice@example.com",
			CommitterDate:  sampleDate,
			Message:        "newer commit",
		},
		{
			Hash:           "bbb222",
			ParentHashes:   []string{"ccc333"},
			AuthorName:     "Bob",
			AuthorEmail:    "bob@example.com",
			AuthorDate:     sampleDate.Add(-time.Hour),
			CommitterName:  "Bob",
			CommitterEmail: "bob@example.com",
			CommitterDate:  sampleDate.Add(-time.Hour),
			Message:        "older commit",
		},
	}
}

func sampleDiffMap() map[string][]gitclient.FileDiffEntry {
	return map[string][]gitclient.FileDiffEntry{
		"aaa111": {
			{Status: "A", Path: "src/new.ts", Additions: 10, Deletions: 0, Patch: "--- /dev/null\n+++ b/src/new.ts\n@@ -0,0 +1,10 @@\n+line1"},
			{Status: "M", Path: "src/mod.ts", Additions: 5, Deletions: 3, Patch: "--- a/src/mod.ts\n+++ b/src/mod.ts\n@@ -1,3 +1,5 @@\n-old\n+new"},
		},
		"bbb222": {
			{Status: "D", Path: "src/old.ts", Additions: 0, Deletions: 20, Patch: "--- a/src/old.ts\n+++ /dev/null\n@@ -1,20 +0,0 @@\n-deleted"},
		},
	}
}

// --- tests ---

func TestIndexAll_HappyPath(t *testing.T) {
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return sampleCommits(), nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return sampleDiffMap(), nil
		},
	}

	// Return deterministic UUIDs for inserted commits.
	commitIDMap := map[string]pgtype.UUID{
		"aaa111": testUUID(0x01),
		"bbb222": testUUID(0x02),
	}
	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, arg db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: commitIDMap[arg.CommitHash]}, nil
		},
	}

	ix := New(q, git)
	result, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify commit count.
	if result.CommitsIndexed != 2 {
		t.Errorf("CommitsIndexed = %d, want 2", result.CommitsIndexed)
	}

	// Verify InsertCommit calls.
	if len(q.insertCommitCalls) != 2 {
		t.Fatalf("InsertCommit calls = %d, want 2", len(q.insertCommitCalls))
	}
	if q.insertCommitCalls[0].CommitHash != "aaa111" {
		t.Errorf("first commit hash = %q", q.insertCommitCalls[0].CommitHash)
	}
	if q.insertCommitCalls[0].AuthorName != "Alice" {
		t.Errorf("first commit author = %q", q.insertCommitCalls[0].AuthorName)
	}
	if q.insertCommitCalls[1].CommitHash != "bbb222" {
		t.Errorf("second commit hash = %q", q.insertCommitCalls[1].CommitHash)
	}

	// Verify parent relationships.
	if len(q.insertParentCalls) != 1 {
		// bbb222's parent ccc333 is not in batch and not in DB → skipped.
		// aaa111's parent bbb222 is in batch → inserted.
		t.Fatalf("InsertCommitParent calls = %d, want 1", len(q.insertParentCalls))
	}
	parentCall := q.insertParentCalls[0]
	if parentCall.CommitID != testUUID(0x01) {
		t.Errorf("parent call CommitID = %v, want 0x01", parentCall.CommitID)
	}
	if parentCall.ParentCommitID != testUUID(0x02) {
		t.Errorf("parent call ParentCommitID = %v, want 0x02", parentCall.ParentCommitID)
	}
	if parentCall.Ordinal != 0 {
		t.Errorf("parent call Ordinal = %d, want 0", parentCall.Ordinal)
	}

	// Verify file diffs: 2 for aaa111 + 1 for bbb222 = 3 total.
	if result.DiffsIndexed != 3 {
		t.Errorf("DiffsIndexed = %d, want 3", result.DiffsIndexed)
	}
	if len(q.insertFileDiffCalls) != 3 {
		t.Fatalf("InsertCommitFileDiff calls = %d, want 3", len(q.insertFileDiffCalls))
	}

	// Verify HEAD commit DB ID.
	if result.HeadCommitDBID != testUUID(0x01) {
		t.Errorf("HeadCommitDBID = %v, want 0x01", result.HeadCommitDBID)
	}
}

func TestIndexSince_PassesSinceCommit(t *testing.T) {
	var capturedSince string
	var capturedMax int
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, since string, max int) ([]gitclient.CommitLog, error) {
			capturedSince = since
			capturedMax = max
			return nil, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return nil, nil
		},
	}
	q := &mockQuerier{}
	ix := New(q, git)

	_, err := ix.IndexSince(context.Background(), sampleProjectID, "/repo", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSince != "abc123" {
		t.Errorf("sinceCommit = %q, want abc123", capturedSince)
	}
	if capturedMax != 0 {
		t.Errorf("maxCount = %d, want 0 (no limit)", capturedMax)
	}
}

func TestIndexAll_EmptyLog(t *testing.T) {
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return nil, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return nil, nil
		},
	}
	q := &mockQuerier{}
	ix := New(q, git)

	result, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CommitsIndexed != 0 {
		t.Errorf("CommitsIndexed = %d, want 0", result.CommitsIndexed)
	}
	if len(q.insertCommitCalls) != 0 {
		t.Errorf("InsertCommit should not be called, got %d calls", len(q.insertCommitCalls))
	}
}

func TestIndexAll_ParentInBatch(t *testing.T) {
	// Both commits are in the batch → GetCommitByHash should NOT be called.
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return sampleCommits(), nil // aaa111 has parent bbb222, both in batch
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return nil, nil
		},
	}

	var getByHashCalled bool
	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, arg db.InsertCommitParams) (db.Commit, error) {
			if arg.CommitHash == "aaa111" {
				return db.Commit{ID: testUUID(0x01)}, nil
			}
			return db.Commit{ID: testUUID(0x02)}, nil
		},
		getCommitByHashFn: func(_ context.Context, _ db.GetCommitByHashParams) (db.Commit, error) {
			getByHashCalled = true
			return db.Commit{}, pgx.ErrNoRows
		},
	}

	ix := New(q, git)
	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parent bbb222 was found in batch for aaa111.
	// But bbb222's parent ccc333 isn't in batch, so GetCommitByHash IS called for ccc333.
	if !getByHashCalled {
		t.Error("GetCommitByHash should be called for ccc333 (out-of-batch parent)")
	}

	// aaa111→bbb222 parent should still be inserted (from batch).
	found := false
	for _, p := range q.insertParentCalls {
		if p.CommitID == testUUID(0x01) && p.ParentCommitID == testUUID(0x02) {
			found = true
		}
	}
	if !found {
		t.Error("expected parent insert for aaa111→bbb222")
	}
}

func TestIndexAll_ParentFallbackToGetCommitByHash(t *testing.T) {
	// Single commit whose parent is NOT in the batch but IS in the DB.
	commits := []gitclient.CommitLog{{
		Hash:          "aaa111",
		ParentHashes:  []string{"already_in_db"},
		AuthorName:    "A",
		AuthorEmail:   "a@x",
		AuthorDate:    sampleDate,
		CommitterName: "A", CommitterEmail: "a@x", CommitterDate: sampleDate,
		Message: "commit",
	}}

	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return commits, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return nil, nil
		},
	}

	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, _ db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: testUUID(0x01)}, nil
		},
		getCommitByHashFn: func(_ context.Context, arg db.GetCommitByHashParams) (db.Commit, error) {
			if arg.CommitHash == "already_in_db" {
				return db.Commit{ID: testUUID(0xDD)}, nil
			}
			return db.Commit{}, pgx.ErrNoRows
		},
	}

	ix := New(q, git)
	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(q.insertParentCalls) != 1 {
		t.Fatalf("InsertCommitParent calls = %d, want 1", len(q.insertParentCalls))
	}
	if q.insertParentCalls[0].ParentCommitID != testUUID(0xDD) {
		t.Errorf("ParentCommitID = %v, want 0xDD (from GetCommitByHash)", q.insertParentCalls[0].ParentCommitID)
	}
}

func TestIndexAll_ParentNotFound(t *testing.T) {
	// Single commit whose parent is NOT in the batch and NOT in the DB.
	commits := []gitclient.CommitLog{{
		Hash:          "aaa111",
		ParentHashes:  []string{"unknown_parent"},
		AuthorName:    "A",
		AuthorEmail:   "a@x",
		AuthorDate:    sampleDate,
		CommitterName: "A", CommitterEmail: "a@x", CommitterDate: sampleDate,
		Message: "commit",
	}}

	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return commits, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return nil, nil
		},
	}

	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, _ db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: testUUID(0x01)}, nil
		},
		// GetCommitByHash returns ErrNoRows by default.
	}

	ix := New(q, git)
	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parent should be silently skipped.
	if len(q.insertParentCalls) != 0 {
		t.Errorf("InsertCommitParent should not be called when parent not found, got %d calls", len(q.insertParentCalls))
	}
}

func TestIndexAll_ChangeTypeMapping(t *testing.T) {
	commits := []gitclient.CommitLog{{
		Hash:          "aaa111",
		ParentHashes:  nil,
		AuthorName:    "A",
		AuthorEmail:   "a@x",
		AuthorDate:    sampleDate,
		CommitterName: "A", CommitterEmail: "a@x", CommitterDate: sampleDate,
		Message: "commit",
	}}
	diffMap := map[string][]gitclient.FileDiffEntry{
		"aaa111": {
			{Status: "A", Path: "added.ts", Additions: 10, Deletions: 0},
			{Status: "M", Path: "modified.ts", Additions: 5, Deletions: 3},
			{Status: "D", Path: "deleted.ts", Additions: 0, Deletions: 20},
		},
	}

	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return commits, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return diffMap, nil
		},
	}

	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, _ db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: testUUID(0x01)}, nil
		},
	}

	ix := New(q, git)
	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(q.insertFileDiffCalls) != 3 {
		t.Fatalf("InsertCommitFileDiff calls = %d, want 3", len(q.insertFileDiffCalls))
	}

	// Find each by change_type.
	for _, call := range q.insertFileDiffCalls {
		switch call.ChangeType {
		case "added":
			if call.OldFilePath.Valid {
				t.Error("added file should have NULL old_file_path")
			}
			if !call.NewFilePath.Valid || call.NewFilePath.String != "added.ts" {
				t.Errorf("added file new_file_path = %v", call.NewFilePath)
			}
		case "modified":
			if !call.OldFilePath.Valid || call.OldFilePath.String != "modified.ts" {
				t.Errorf("modified file old_file_path = %v", call.OldFilePath)
			}
			if !call.NewFilePath.Valid || call.NewFilePath.String != "modified.ts" {
				t.Errorf("modified file new_file_path = %v", call.NewFilePath)
			}
		case "deleted":
			if !call.OldFilePath.Valid || call.OldFilePath.String != "deleted.ts" {
				t.Errorf("deleted file old_file_path = %v", call.OldFilePath)
			}
			if call.NewFilePath.Valid {
				t.Error("deleted file should have NULL new_file_path")
			}
		default:
			t.Errorf("unexpected change_type: %q", call.ChangeType)
		}
	}
}

func TestIndexAll_GitLogError(t *testing.T) {
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return nil, errors.New("git log failed")
		},
	}
	q := &mockQuerier{}
	ix := New(q, git)

	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err == nil {
		t.Fatal("expected error from git log failure")
	}

	// Verify the error wraps the original message with context.
	if got := err.Error(); !strings.Contains(got, "git log failed") {
		t.Errorf("error = %q, want it to contain %q", got, "git log failed")
	}
	if got := err.Error(); !strings.Contains(got, "commits: log commits") {
		t.Errorf("error = %q, want it to contain wrapping prefix %q", got, "commits: log commits")
	}
}

func TestIndexAll_DiffStatLogError(t *testing.T) {
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return sampleCommits(), nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return nil, errors.New("diff stat failed")
		},
	}
	q := &mockQuerier{}
	ix := New(q, git)

	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIndexAll_RootCommit(t *testing.T) {
	// Root commit: no parents, should still index diffs.
	commits := []gitclient.CommitLog{{
		Hash:          "root1",
		ParentHashes:  nil,
		AuthorName:    "A",
		AuthorEmail:   "a@x",
		AuthorDate:    sampleDate,
		CommitterName: "A", CommitterEmail: "a@x", CommitterDate: sampleDate,
		Message: "initial commit",
	}}
	diffMap := map[string][]gitclient.FileDiffEntry{
		"root1": {
			{Status: "A", Path: "README.md", Additions: 5, Deletions: 0},
		},
	}

	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return commits, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return diffMap, nil
		},
	}

	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, _ db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: testUUID(0x01)}, nil
		},
	}

	ix := New(q, git)
	result, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No parents to insert.
	if len(q.insertParentCalls) != 0 {
		t.Errorf("InsertCommitParent should not be called for root commit, got %d", len(q.insertParentCalls))
	}

	// Diff should still be inserted with zero-value ParentCommitID.
	if len(q.insertFileDiffCalls) != 1 {
		t.Fatalf("InsertCommitFileDiff calls = %d, want 1", len(q.insertFileDiffCalls))
	}
	if q.insertFileDiffCalls[0].ParentCommitID.Valid {
		t.Error("root commit diff should have zero-value (invalid) ParentCommitID")
	}

	if result.DiffsIndexed != 1 {
		t.Errorf("DiffsIndexed = %d, want 1", result.DiffsIndexed)
	}
}

func TestIndexAll_PatchPopulated(t *testing.T) {
	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return sampleCommits(), nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return sampleDiffMap(), nil
		},
	}

	commitIDMap := map[string]pgtype.UUID{
		"aaa111": testUUID(0x01),
		"bbb222": testUUID(0x02),
	}
	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, arg db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: commitIDMap[arg.CommitHash]}, nil
		},
	}

	ix := New(q, git)
	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 file diffs should have non-empty patches.
	for _, call := range q.insertFileDiffCalls {
		if !call.Patch.Valid {
			path := call.NewFilePath.String
			if !call.NewFilePath.Valid {
				path = call.OldFilePath.String
			}
			t.Errorf("file diff for %s should have a valid patch", path)
		}
		if call.Patch.String == "" {
			t.Error("patch string should not be empty when Valid is true")
		}
	}
}

func TestIndexAll_PatchEmpty(t *testing.T) {
	// Verify that entries with empty Patch produce NULL (invalid pgtype.Text).
	commits := []gitclient.CommitLog{{
		Hash:          "aaa111",
		ParentHashes:  nil,
		AuthorName:    "A",
		AuthorEmail:   "a@x",
		AuthorDate:    sampleDate,
		CommitterName: "A", CommitterEmail: "a@x", CommitterDate: sampleDate,
		Message: "commit with binary",
	}}
	diffMap := map[string][]gitclient.FileDiffEntry{
		"aaa111": {
			{Status: "A", Path: "image.png", Additions: 0, Deletions: 0, Patch: ""}, // binary file, no patch
		},
	}

	git := &mockGitProvider{
		logCommitsFn: func(_ context.Context, _, _ string, _ int) ([]gitclient.CommitLog, error) {
			return commits, nil
		},
		diffStatLogFn: func(_ context.Context, _, _ string, _ int) (map[string][]gitclient.FileDiffEntry, error) {
			return diffMap, nil
		},
	}

	q := &mockQuerier{
		insertCommitFn: func(_ context.Context, _ db.InsertCommitParams) (db.Commit, error) {
			return db.Commit{ID: testUUID(0x01)}, nil
		},
	}

	ix := New(q, git)
	_, err := ix.IndexAll(context.Background(), sampleProjectID, "/repo", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(q.insertFileDiffCalls) != 1 {
		t.Fatalf("InsertCommitFileDiff calls = %d, want 1", len(q.insertFileDiffCalls))
	}
	if q.insertFileDiffCalls[0].Patch.Valid {
		t.Error("empty patch should produce NULL (invalid pgtype.Text)")
	}
}

func TestToPgText(t *testing.T) {
	// Empty string → NULL.
	result := toPgText("")
	if result.Valid {
		t.Error("empty string should produce invalid pgtype.Text")
	}

	// Non-empty string → valid.
	result = toPgText("some patch content")
	if !result.Valid {
		t.Error("non-empty string should produce valid pgtype.Text")
	}
	if result.String != "some patch content" {
		t.Errorf("String = %q, want %q", result.String, "some patch content")
	}
}
