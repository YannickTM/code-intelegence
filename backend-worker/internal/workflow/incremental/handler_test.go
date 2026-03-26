package incremental

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/artifact"
	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/gitclient"
	"myjungle/backend-worker/internal/indexing"
	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/vectorstore"
	"myjungle/backend-worker/internal/workflow"
	"myjungle/backend-worker/internal/workflow/commits"
	"myjungle/backend-worker/internal/workspace"
	db "myjungle/datastore/postgres/sqlc"
)

func noopActivate(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
	return nil
}

// --- test helpers ---

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

func testUUIDString(b byte) string {
	return formatUUID(testUUID(b))
}

// --- mock jobRepo ---

type mockJobRepo struct {
	loadCtxFn      func(ctx context.Context, jobID pgtype.UUID) (*execution.Context, error)
	claimFn        func(ctx context.Context, jobID pgtype.UUID, workerID string) error
	createSnapFn   func(ctx context.Context, params db.CreateIndexSnapshotParams) (db.IndexSnapshot, error)
	setCompletedFn func(ctx context.Context, jobID pgtype.UUID) (bool, error)
	setFailedFn    func(ctx context.Context, jobID pgtype.UUID, errorDetails []byte) (bool, error)
	tryLockFn      func(ctx context.Context, projectID pgtype.UUID) (func(context.Context), error)
}

func (m *mockJobRepo) LoadExecutionContext(ctx context.Context, jobID pgtype.UUID) (*execution.Context, error) {
	return m.loadCtxFn(ctx, jobID)
}
func (m *mockJobRepo) ClaimJob(ctx context.Context, jobID pgtype.UUID, workerID string) error {
	return m.claimFn(ctx, jobID, workerID)
}
func (m *mockJobRepo) CreateSnapshot(ctx context.Context, params db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
	return m.createSnapFn(ctx, params)
}
func (m *mockJobRepo) SetJobCompleted(ctx context.Context, jobID pgtype.UUID) (bool, error) {
	return m.setCompletedFn(ctx, jobID)
}
func (m *mockJobRepo) SetJobFailed(ctx context.Context, jobID pgtype.UUID, errorDetails []byte) (bool, error) {
	return m.setFailedFn(ctx, jobID, errorDetails)
}
func (m *mockJobRepo) TryProjectLock(ctx context.Context, projectID pgtype.UUID) (func(context.Context), error) {
	return m.tryLockFn(ctx, projectID)
}

// --- mock workspace ---

type mockWorkspace struct {
	prepareFn func(ctx context.Context, execCtx *execution.Context) (*workspace.Result, func(), error)
}

func (m *mockWorkspace) Prepare(ctx context.Context, execCtx *execution.Context) (*workspace.Result, func(), error) {
	return m.prepareFn(ctx, execCtx)
}

// --- mock parser ---

type mockParser struct {
	parseFn func(ctx context.Context, projectID, branch, commitSHA string, files []parser.FileInput) ([]parser.ParsedFileResult, error)
}

func (m *mockParser) ParseFilesBatched(ctx context.Context, projectID, branch, commitSHA string, files []parser.FileInput) ([]parser.ParsedFileResult, error) {
	return m.parseFn(ctx, projectID, branch, commitSHA, files)
}

// --- mock differ ---

type mockDiffer struct {
	diffFn func(ctx context.Context, repoDir, baseCommit, targetCommit string) ([]gitclient.DiffEntry, error)
}

func (m *mockDiffer) DiffNameStatus(ctx context.Context, repoDir, baseCommit, targetCommit string) ([]gitclient.DiffEntry, error) {
	return m.diffFn(ctx, repoDir, baseCommit, targetCommit)
}

// --- mock commit indexer ---

type mockCommitIndexer struct {
	indexAllFn   func(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error)
	indexSinceFn func(ctx context.Context, projectID pgtype.UUID, repoDir, sinceCommit string) (*commits.Result, error)
}

func (m *mockCommitIndexer) IndexAll(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error) {
	if m.indexAllFn != nil {
		return m.indexAllFn(ctx, projectID, repoDir, maxCommits)
	}
	return &commits.Result{}, nil
}

func (m *mockCommitIndexer) IndexSince(ctx context.Context, projectID pgtype.UUID, repoDir, sinceCommit string) (*commits.Result, error) {
	if m.indexSinceFn != nil {
		return m.indexSinceFn(ctx, projectID, repoDir, sinceCommit)
	}
	return &commits.Result{}, nil
}

// --- mock querier ---

type mockQuerier struct {
	db.Querier
	getVersionFn          func(ctx context.Context, label string) (db.EmbeddingVersion, error)
	createVersionFn       func(ctx context.Context, params db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error)
	setProgressFn         func(ctx context.Context, arg db.SetIndexingJobProgressParams) error
	upsertContentFn       func(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error)
	insertFileFn          func(ctx context.Context, arg db.InsertFileParams) (db.File, error)
	insertSymbolFn        func(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error)
	insertChunkFn         func(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error)
	insertDepFn           func(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error)
	getActiveSnapBranchFn func(ctx context.Context, arg db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error)
	listSnapshotFilesFn   func(ctx context.Context, id pgtype.UUID) ([]db.File, error)
	listSymbolsByFileFn   func(ctx context.Context, fileID pgtype.UUID) ([]db.Symbol, error)
	listChunksByFileFn    func(ctx context.Context, fileID pgtype.UUID) ([]db.CodeChunk, error)
	listDepsBySnapFileFn  func(ctx context.Context, arg db.ListDependenciesBySnapshotAndFileParams) ([]db.Dependency, error)
	deleteSnapArtifactFn  func(ctx context.Context, id pgtype.UUID) error
	updateSnapCommitIDFn  func(ctx context.Context, arg db.UpdateSnapshotCommitIDParams) (int64, error)
	getCommitByHashFn     func(ctx context.Context, arg db.GetCommitByHashParams) (db.Commit, error)
}

func (m *mockQuerier) DeleteSnapshotArtifacts(ctx context.Context, id pgtype.UUID) error {
	if m.deleteSnapArtifactFn != nil {
		return m.deleteSnapArtifactFn(ctx, id)
	}
	return nil
}
func (m *mockQuerier) GetEmbeddingVersionByLabel(ctx context.Context, label string) (db.EmbeddingVersion, error) {
	return m.getVersionFn(ctx, label)
}
func (m *mockQuerier) CreateEmbeddingVersion(ctx context.Context, params db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
	return m.createVersionFn(ctx, params)
}
func (m *mockQuerier) SetIndexingJobProgress(ctx context.Context, arg db.SetIndexingJobProgressParams) error {
	return m.setProgressFn(ctx, arg)
}
func (m *mockQuerier) UpsertFileContent(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error) {
	return m.upsertContentFn(ctx, arg)
}
func (m *mockQuerier) InsertFile(ctx context.Context, arg db.InsertFileParams) (db.File, error) {
	return m.insertFileFn(ctx, arg)
}
func (m *mockQuerier) InsertSymbol(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error) {
	return m.insertSymbolFn(ctx, arg)
}
func (m *mockQuerier) InsertCodeChunk(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error) {
	return m.insertChunkFn(ctx, arg)
}
func (m *mockQuerier) InsertDependency(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error) {
	return m.insertDepFn(ctx, arg)
}
func (m *mockQuerier) GetActiveSnapshotForBranch(ctx context.Context, arg db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
	return m.getActiveSnapBranchFn(ctx, arg)
}
func (m *mockQuerier) ListSnapshotFiles(ctx context.Context, id pgtype.UUID) ([]db.File, error) {
	return m.listSnapshotFilesFn(ctx, id)
}
func (m *mockQuerier) ListSymbolsByFileID(ctx context.Context, fileID pgtype.UUID) ([]db.Symbol, error) {
	return m.listSymbolsByFileFn(ctx, fileID)
}
func (m *mockQuerier) ListChunksByFileID(ctx context.Context, fileID pgtype.UUID) ([]db.CodeChunk, error) {
	return m.listChunksByFileFn(ctx, fileID)
}
func (m *mockQuerier) ListDependenciesBySnapshotAndFile(ctx context.Context, arg db.ListDependenciesBySnapshotAndFileParams) ([]db.Dependency, error) {
	return m.listDepsBySnapFileFn(ctx, arg)
}
func (m *mockQuerier) InsertExport(ctx context.Context, arg db.InsertExportParams) (db.Export, error) {
	return db.Export{ID: pgtype.UUID{Bytes: [16]byte{0xE0}, Valid: true}}, nil
}
func (m *mockQuerier) InsertSymbolReference(ctx context.Context, arg db.InsertSymbolReferenceParams) (db.SymbolReference, error) {
	return db.SymbolReference{ID: pgtype.UUID{Bytes: [16]byte{0xE1}, Valid: true}}, nil
}
func (m *mockQuerier) InsertJsxUsage(ctx context.Context, arg db.InsertJsxUsageParams) (db.JsxUsage, error) {
	return db.JsxUsage{ID: pgtype.UUID{Bytes: [16]byte{0xE2}, Valid: true}}, nil
}
func (m *mockQuerier) InsertNetworkCall(ctx context.Context, arg db.InsertNetworkCallParams) (db.NetworkCall, error) {
	return db.NetworkCall{ID: pgtype.UUID{Bytes: [16]byte{0xE3}, Valid: true}}, nil
}
func (m *mockQuerier) ListExportsByFileID(ctx context.Context, fileID pgtype.UUID) ([]db.Export, error) {
	return []db.Export{}, nil
}
func (m *mockQuerier) ListSymbolReferencesByFileID(ctx context.Context, fileID pgtype.UUID) ([]db.SymbolReference, error) {
	return []db.SymbolReference{}, nil
}
func (m *mockQuerier) ListJsxUsagesByFileID(ctx context.Context, fileID pgtype.UUID) ([]db.JsxUsage, error) {
	return []db.JsxUsage{}, nil
}
func (m *mockQuerier) ListNetworkCallsByFileID(ctx context.Context, fileID pgtype.UUID) ([]db.NetworkCall, error) {
	return []db.NetworkCall{}, nil
}
func (m *mockQuerier) UpdateSnapshotCommitID(ctx context.Context, arg db.UpdateSnapshotCommitIDParams) (int64, error) {
	if m.updateSnapCommitIDFn != nil {
		return m.updateSnapCommitIDFn(ctx, arg)
	}
	return 1, nil
}
func (m *mockQuerier) GetCommitByHash(ctx context.Context, arg db.GetCommitByHashParams) (db.Commit, error) {
	if m.getCommitByHashFn != nil {
		return m.getCommitByHashFn(ctx, arg)
	}
	return db.Commit{}, pgx.ErrNoRows
}

// --- mock vectorstore ---

type mockVectorStore struct {
	ensureFn    func(ctx context.Context, collection string, dimensions int32) error
	upsertFn    func(ctx context.Context, collection string, points []vectorstore.Point) error
	getPointsFn func(ctx context.Context, collection string, ids []string, withVector bool) ([]vectorstore.Point, error)
}

func (m *mockVectorStore) EnsureCollection(ctx context.Context, collection string, dimensions int32) error {
	return m.ensureFn(ctx, collection, dimensions)
}
func (m *mockVectorStore) UpsertPoints(ctx context.Context, collection string, points []vectorstore.Point) error {
	return m.upsertFn(ctx, collection, points)
}
func (m *mockVectorStore) GetPoints(ctx context.Context, collection string, ids []string, withVector bool) ([]vectorstore.Point, error) {
	return m.getPointsFn(ctx, collection, ids, withVector)
}

// --- fixtures ---

func defaultExecCtx() *execution.Context {
	return &execution.Context{
		JobID:         testUUID(0x01),
		JobType:       "incremental-index",
		ProjectID:     testUUID(0x02),
		RepoURL:       "git@github.com:test/repo.git",
		Branch:        "main",
		SSHKeyID:      testUUID(0x03),
		SSHPrivateKey: []byte("fake-key"),
		Embedding: execution.EmbeddingConfig{
			ID:          testUUID(0x04),
			Provider:    "ollama",
			EndpointURL: "http://localhost:11434",
			Model:       "jina/jina-embeddings-v2-base-en",
			Dimensions:  768,
		},
	}
}

func defaultTask() workflow.WorkflowTask {
	return workflow.WorkflowTask{
		JobID:    testUUIDString(0x01),
		Workflow: "incremental-index",
	}
}

var embVersion = db.EmbeddingVersion{ID: testUUID(0x10)}

func defaultMocks() (*mockJobRepo, *mockWorkspace, *mockParser, *mockDiffer, *mockQuerier, *mockVectorStore, *mockCommitIndexer) {
	chunkCounter := byte(0x30)

	repo := &mockJobRepo{
		loadCtxFn: func(_ context.Context, _ pgtype.UUID) (*execution.Context, error) {
			return defaultExecCtx(), nil
		},
		claimFn: func(_ context.Context, _ pgtype.UUID, _ string) error {
			return nil
		},
		createSnapFn: func(_ context.Context, _ db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
			return db.IndexSnapshot{ID: testUUID(0x20)}, nil
		},
		setCompletedFn: func(_ context.Context, _ pgtype.UUID) (bool, error) {
			return true, nil
		},
		setFailedFn: func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
			return true, nil
		},
		tryLockFn: func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
			return func(context.Context) {}, nil
		},
	}

	ws := &mockWorkspace{
		prepareFn: func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
			return &workspace.Result{
				RepoDir:     "/tmp/test-worktree",
				CommitSHA:   "newcommitsha",
				SourceFiles: []string{},
			}, func() {}, nil
		},
	}

	p := &mockParser{
		parseFn: func(_ context.Context, _, _, _ string, _ []parser.FileInput) ([]parser.ParsedFileResult, error) {
			return []parser.ParsedFileResult{}, nil
		},
	}

	d := &mockDiffer{
		diffFn: func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
			return nil, nil
		},
	}

	q := &mockQuerier{
		getVersionFn: func(_ context.Context, _ string) (db.EmbeddingVersion, error) {
			return embVersion, nil
		},
		createVersionFn: func(_ context.Context, _ db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
			return embVersion, nil
		},
		setProgressFn: func(_ context.Context, _ db.SetIndexingJobProgressParams) error {
			return nil
		},
		upsertContentFn: func(_ context.Context, _ db.UpsertFileContentParams) (db.FileContent, error) {
			return db.FileContent{ID: testUUID(0x40)}, nil
		},
		insertFileFn: func(_ context.Context, _ db.InsertFileParams) (db.File, error) {
			return db.File{ID: testUUID(0x41)}, nil
		},
		insertSymbolFn: func(_ context.Context, _ db.InsertSymbolParams) (db.Symbol, error) {
			return db.Symbol{ID: testUUID(0x42)}, nil
		},
		insertChunkFn: func(_ context.Context, _ db.InsertCodeChunkParams) (db.CodeChunk, error) {
			id := testUUID(chunkCounter)
			chunkCounter++
			return db.CodeChunk{ID: id}, nil
		},
		insertDepFn: func(_ context.Context, _ db.InsertDependencyParams) (db.Dependency, error) {
			return db.Dependency{ID: testUUID(0x43)}, nil
		},
		getActiveSnapBranchFn: func(_ context.Context, _ db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
			return db.IndexSnapshot{
				ID:                 testUUID(0xA0),
				GitCommit:          "oldcommitsha",
				EmbeddingVersionID: embVersion.ID,
				Branch:             "main",
			}, nil
		},
		listSnapshotFilesFn: func(_ context.Context, _ pgtype.UUID) ([]db.File, error) {
			return []db.File{}, nil
		},
		listSymbolsByFileFn: func(_ context.Context, _ pgtype.UUID) ([]db.Symbol, error) {
			return []db.Symbol{}, nil
		},
		listChunksByFileFn: func(_ context.Context, _ pgtype.UUID) ([]db.CodeChunk, error) {
			return []db.CodeChunk{}, nil
		},
		listDepsBySnapFileFn: func(_ context.Context, _ db.ListDependenciesBySnapshotAndFileParams) ([]db.Dependency, error) {
			return []db.Dependency{}, nil
		},
	}

	mv := &mockVectorStore{
		ensureFn:    func(_ context.Context, _ string, _ int32) error { return nil },
		upsertFn:    func(_ context.Context, _ string, _ []vectorstore.Point) error { return nil },
		getPointsFn: func(_ context.Context, _ string, _ []string, _ bool) ([]vectorstore.Point, error) { return nil, nil },
	}

	ci := &mockCommitIndexer{}

	return repo, ws, p, d, q, mv, ci
}

func newTestHandler(repo *mockJobRepo, ws *mockWorkspace, p *mockParser, d *mockDiffer, q *mockQuerier, mv *mockVectorStore, ci *mockCommitIndexer) *Handler {
	return newTestHandlerWithActivate(repo, ws, p, d, q, noopActivate, mv, ci)
}

func newTestHandlerWithActivate(repo *mockJobRepo, ws *mockWorkspace, p *mockParser, d *mockDiffer, q *mockQuerier, activate indexing.ActivateFunc, mv *mockVectorStore, ci *mockCommitIndexer) *Handler {
	h := &Handler{
		repo:      repo,
		workspace: ws,
		parser:    p,
		queries:   q,
		activate:  activate,
		writer:    artifact.NewWriter(q),
		vectorDB:  mv,
		differ:    d,
	}
	if ci != nil {
		h.commitIndexer = ci
	}
	return h
}

// --- tests ---

func TestHandle_HappyPath_Incremental(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	// Create temp dir with a real modified file.
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "src/modified.ts"), []byte("const x = 1;"), 0644)

	// Workspace returns 3 source files (1 modified + 2 unchanged; 1 deleted is not on disk).
	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     tmpDir,
			CommitSHA:   "newcommitsha",
			SourceFiles: []string{"src/modified.ts", "src/unchanged1.ts", "src/unchanged2.ts"},
		}, func() {}, nil
	}

	// Diff: 1 modified, 1 deleted.
	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		return []gitclient.DiffEntry{
			{Status: "M", Path: "src/modified.ts"},
			{Status: "D", Path: "src/deleted.ts"},
		}, nil
	}

	// Old snapshot has 3 files: modified, unchanged1, unchanged2, deleted.
	oldFileIDs := map[string]pgtype.UUID{
		"src/modified.ts":   testUUID(0xB0),
		"src/unchanged1.ts": testUUID(0xB1),
		"src/unchanged2.ts": testUUID(0xB2),
		"src/deleted.ts":    testUUID(0xB3),
	}
	q.listSnapshotFilesFn = func(_ context.Context, _ pgtype.UUID) ([]db.File, error) {
		return []db.File{
			{ID: oldFileIDs["src/modified.ts"], FilePath: "src/modified.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
			{ID: oldFileIDs["src/unchanged1.ts"], FilePath: "src/unchanged1.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
			{ID: oldFileIDs["src/unchanged2.ts"], FilePath: "src/unchanged2.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
			{ID: oldFileIDs["src/deleted.ts"], FilePath: "src/deleted.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
		}, nil
	}

	// Track which files are parsed; return 1 chunk per file so we can verify progress totals.
	var parsedFilePaths []string
	p.parseFn = func(_ context.Context, _, _, _ string, files []parser.FileInput) ([]parser.ParsedFileResult, error) {
		var results []parser.ParsedFileResult
		for _, f := range files {
			parsedFilePaths = append(parsedFilePaths, f.FilePath)
			results = append(results, parser.ParsedFileResult{
				FilePath: f.FilePath,
				Language: "typescript",
				Chunks: []parser.Chunk{
					{Content: "chunk from parser", ChunkType: "function", StartLine: 1, EndLine: 1},
				},
			})
		}
		return results, nil
	}

	// Track copy-forward: unchanged files have 1 chunk each.
	q.listChunksByFileFn = func(_ context.Context, fileID pgtype.UUID) ([]db.CodeChunk, error) {
		// Return 1 chunk for unchanged files.
		if fileID == oldFileIDs["src/unchanged1.ts"] || fileID == oldFileIDs["src/unchanged2.ts"] {
			return []db.CodeChunk{
				{ID: testUUID(fileID.Bytes[0] + 0x10), Content: "chunk content", ChunkType: "function"},
			}, nil
		}
		return []db.CodeChunk{}, nil
	}

	// GetPoints returns vectors for copy-forward.
	mv.getPointsFn = func(_ context.Context, _ string, ids []string, _ bool) ([]vectorstore.Point, error) {
		points := make([]vectorstore.Point, len(ids))
		for i, id := range ids {
			points[i] = vectorstore.Point{
				ID:     id,
				Vector: []float32{0.1, 0.2},
				Payload: map[string]interface{}{
					"index_snapshot_id": "old-snap",
					"git_commit":        "oldcommitsha",
				},
			}
		}
		return points, nil
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	var activated bool
	activate := func(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
		activated = true
		return nil
	}

	// Track all SetIndexingJobProgress calls so we can verify the final totals.
	var progressCalls []db.SetIndexingJobProgressParams
	q.setProgressFn = func(_ context.Context, arg db.SetIndexingJobProgressParams) error {
		progressCalls = append(progressCalls, arg)
		return nil
	}

	h := newTestHandlerWithActivate(repo, ws, p, d, q, activate, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called")
	}
	if !activated {
		t.Error("ActivateSnapshot should have been called")
	}
	// Only modified file should be parsed (not unchanged ones).
	if len(parsedFilePaths) != 1 {
		t.Fatalf("expected parser to be called with 1 file, got %d: %v", len(parsedFilePaths), parsedFilePaths)
	}
	if parsedFilePaths[0] != "src/modified.ts" {
		t.Errorf("expected parsed file to be %q, got %q", "src/modified.ts", parsedFilePaths[0])
	}
	// Verify final progress call includes both pipeline chunks (1 from parsed file)
	// and copy-forward chunks (2 from unchanged files).
	if len(progressCalls) == 0 {
		t.Fatal("expected at least one SetIndexingJobProgress call")
	}
	lastProgress := progressCalls[len(progressCalls)-1]
	// 1 parsed + 2 unchanged = 3 total files.
	if lastProgress.FilesProcessed != 3 {
		t.Errorf("expected FilesProcessed=3, got %d", lastProgress.FilesProcessed)
	}
	// 1 chunk from pipeline + 2 chunks from copy-forward = 3 total chunks.
	if lastProgress.ChunksUpserted != 3 {
		t.Errorf("expected ChunksUpserted=3 (1 pipeline + 2 copied), got %d", lastProgress.ChunksUpserted)
	}
}

func TestHandle_NoActiveSnapshot_FallsBackToFull(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	q.getActiveSnapBranchFn = func(_ context.Context, _ db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, pgx.ErrNoRows
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	// Diff should NOT be called since we fall back before it.
	var diffCalled bool
	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		diffCalled = true
		return nil, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called (full fallback)")
	}
	if diffCalled {
		t.Error("DiffNameStatus should not be called when no active snapshot exists")
	}
}

func TestHandle_EmbeddingVersionMismatch_FallsBackToFull(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	// Base snapshot has a different embedding version.
	q.getActiveSnapBranchFn = func(_ context.Context, _ db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{
			ID:                 testUUID(0xA0),
			GitCommit:          "oldcommit",
			EmbeddingVersionID: testUUID(0xFF), // different from embVersion.ID
			Branch:             "main",
		}, nil
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called (full fallback)")
	}
}

func TestHandle_DiffFailure_FallsBackToFull(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		return nil, errors.New("diff failed: ambiguous commit")
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called (full fallback)")
	}
}

func TestHandle_ParserFailure_FailsJob(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "src/changed.ts"), []byte("const x = 1;"), 0644)

	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     tmpDir,
			CommitSHA:   "newcommitsha",
			SourceFiles: []string{"src/changed.ts"},
		}, func() {}, nil
	}

	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		return []gitclient.DiffEntry{
			{Status: "M", Path: "src/changed.ts"},
		}, nil
	}

	q.listSnapshotFilesFn = func(_ context.Context, _ pgtype.UUID) ([]db.File, error) {
		return []db.File{
			{ID: testUUID(0xB0), FilePath: "src/changed.ts"},
		}, nil
	}

	p.parseFn = func(_ context.Context, _, _, _ string, _ []parser.FileInput) ([]parser.ParsedFileResult, error) {
		return nil, errors.New("parser unavailable")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	assertErrorCategory(t, failDetails, "parser")
}

func TestHandle_CopyForwardFailure_FailsJob(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     "/tmp/test-worktree",
			CommitSHA:   "newcommitsha",
			SourceFiles: []string{"src/unchanged.ts"},
		}, func() {}, nil
	}

	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		return nil, nil // empty diff = all unchanged
	}

	q.listSnapshotFilesFn = func(_ context.Context, _ pgtype.UUID) ([]db.File, error) {
		return []db.File{
			{ID: testUUID(0xB0), FilePath: "src/unchanged.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
		}, nil
	}

	// InsertFile fails during copy-forward.
	q.insertFileFn = func(_ context.Context, _ db.InsertFileParams) (db.File, error) {
		return db.File{}, errors.New("constraint violation")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	assertErrorCategory(t, failDetails, "artifact_write")
}

func TestHandle_AllFilesChanged_NoUnchanged(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "src/a.ts"), []byte("const a = 1;"), 0644)

	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     tmpDir,
			CommitSHA:   "newcommitsha",
			SourceFiles: []string{"src/a.ts"},
		}, func() {}, nil
	}

	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		return []gitclient.DiffEntry{
			{Status: "M", Path: "src/a.ts"},
		}, nil
	}

	q.listSnapshotFilesFn = func(_ context.Context, _ pgtype.UUID) ([]db.File, error) {
		return []db.File{
			{ID: testUUID(0xB0), FilePath: "src/a.ts"},
		}, nil
	}

	var parsedPaths []string
	p.parseFn = func(_ context.Context, _, _, _ string, files []parser.FileInput) ([]parser.ParsedFileResult, error) {
		for _, f := range files {
			parsedPaths = append(parsedPaths, f.FilePath)
		}
		return []parser.ParsedFileResult{
			{FilePath: "src/a.ts", Language: "typescript", Chunks: []parser.Chunk{
				{Content: "const a = 1;", ChunkType: "module_context", StartLine: 1, EndLine: 1},
			}},
		}, nil
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called")
	}
	// The single file was changed — it must be parsed, not copy-forwarded.
	if len(parsedPaths) != 1 || parsedPaths[0] != "src/a.ts" {
		t.Errorf("expected parser called with [src/a.ts], got %v", parsedPaths)
	}
}

func TestHandle_WorkspaceCleanupAlwaysRuns(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	var cleanedUp bool
	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     "/tmp/test",
			CommitSHA:   "abc123",
			SourceFiles: []string{},
		}, func() { cleanedUp = true }, nil
	}

	// Fail at create snapshot.
	repo.createSnapFn = func(_ context.Context, _ db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, errors.New("db error")
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	_ = h.Handle(context.Background(), defaultTask())
	if !cleanedUp {
		t.Error("workspace cleanup should run even on failure")
	}
}

func TestHandle_InvalidJobID(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()
	h := newTestHandler(repo, ws, p, d, q, mv, ci)

	task := defaultTask()
	task.JobID = "not-a-uuid"

	err := h.Handle(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid job_id")
	}
	if !strings.Contains(err.Error(), "invalid job_id") {
		t.Errorf("error = %q, want to contain 'invalid job_id'", err)
	}
}

func TestHandle_LoadContextFailure_ReturnsError(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	repo.loadCtxFn = func(_ context.Context, _ pgtype.UUID) (*execution.Context, error) {
		return nil, errors.New("project not found")
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHandle_ClaimFailure_ReturnsError(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	repo.claimFn = func(_ context.Context, _ pgtype.UUID, _ string) error {
		return errors.New("already claimed")
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHandle_DeletedFilesAbsentFromNewSnapshot(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     "/tmp/test-worktree",
			CommitSHA:   "newcommitsha",
			SourceFiles: []string{"src/unchanged.ts"},
		}, func() {}, nil
	}

	d.diffFn = func(_ context.Context, _, _, _ string) ([]gitclient.DiffEntry, error) {
		return []gitclient.DiffEntry{
			{Status: "D", Path: "src/deleted.ts"},
		}, nil
	}

	q.listSnapshotFilesFn = func(_ context.Context, _ pgtype.UUID) ([]db.File, error) {
		return []db.File{
			{ID: testUUID(0xB0), FilePath: "src/unchanged.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
			{ID: testUUID(0xB1), FilePath: "src/deleted.ts", Language: pgtype.Text{String: "typescript", Valid: true}},
		}, nil
	}

	// Track which file paths get copied.
	var copiedFilePaths []string
	origInsertFn := q.insertFileFn
	q.insertFileFn = func(ctx context.Context, arg db.InsertFileParams) (db.File, error) {
		copiedFilePaths = append(copiedFilePaths, arg.FilePath)
		return origInsertFn(ctx, arg)
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called")
	}

	// Only unchanged file should be copied, not deleted.
	for _, path := range copiedFilePaths {
		if path == "src/deleted.ts" {
			t.Error("deleted file should NOT be copied forward")
		}
	}
	found := false
	for _, path := range copiedFilePaths {
		if path == "src/unchanged.ts" {
			found = true
		}
	}
	if !found {
		t.Error("unchanged file should be copied forward")
	}
}

// --- project lock and stale transition tests ---

func TestHandle_ProjectLockUnavailable_ReturnsError(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	repo.tryLockFn = func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
		return nil, errors.New("repository: project is locked by another worker")
	}

	var failedCalled bool
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
		failedCalled = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err == nil {
		t.Fatal("expected error for project lock failure")
	}
	if !strings.Contains(err.Error(), "project lock") {
		t.Errorf("error = %q, want to contain 'project lock'", err)
	}
	if failedCalled {
		t.Error("SetJobFailed should NOT be called before ClaimJob (project lock is pre-claim)")
	}
}

func TestHandle_SetJobCompletedStale_NoError(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	// SetJobCompleted returns false (stale).
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		return false, nil
	}

	// Must fall back to full to exercise the simpler completed path.
	q.getActiveSnapBranchFn = func(_ context.Context, _ db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, pgx.ErrNoRows
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil even when completion is stale, got: %v", err)
	}
}

func TestHandle_ProjectLockReleasedOnSuccess(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	var unlocked bool
	repo.tryLockFn = func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
		return func(context.Context) { unlocked = true }, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	_ = h.Handle(context.Background(), defaultTask())
	if !unlocked {
		t.Error("project lock should be released on handler return")
	}
}

func TestHandle_ProjectLockReleasedOnFailure(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	var unlocked bool
	repo.tryLockFn = func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
		return func(context.Context) { unlocked = true }, nil
	}

	// Force a post-claim failure.
	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return nil, nil, errors.New("clone failed")
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	_ = h.Handle(context.Background(), defaultTask())
	if !unlocked {
		t.Error("project lock should be released even on failure")
	}
}

// --- split and raw result tests ---

func TestSplitByParserSupport(t *testing.T) {
	files := []string{"src/index.ts", "README.md", "lib/util.js", "main.py", "notes.txt"}
	parseable, raw := splitByParserSupport(files)

	if len(parseable) != 4 {
		t.Fatalf("parseable = %v, want 4 files", parseable)
	}
	wantParseable := []string{"src/index.ts", "README.md", "lib/util.js", "main.py"}
	for i, want := range wantParseable {
		if parseable[i] != want {
			t.Errorf("parseable[%d] = %q, want %q", i, parseable[i], want)
		}
	}
	if len(raw) != 1 {
		t.Fatalf("raw = %v, want 1 file", raw)
	}
	if raw[0] != "notes.txt" {
		t.Errorf("raw = %v, want [notes.txt]", raw)
	}
}

func TestBuildRawFileResults(t *testing.T) {
	contents := map[string]string{
		"main.py": "print('hello')\n",
	}
	results := buildRawFileResults([]string{"main.py"}, contents)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	r := results[0]
	if r.FilePath != "main.py" {
		t.Errorf("FilePath = %q, want %q", r.FilePath, "main.py")
	}
	if r.Language != "python" {
		t.Errorf("Language = %q, want %q", r.Language, "python")
	}
	if len(r.Chunks) != 1 || r.Chunks[0].ChunkType != "raw" {
		t.Errorf("expected 1 raw chunk, got %v", r.Chunks)
	}

	// Empty file produces a ParsedFileResult with 0 chunks.
	emptyContents := map[string]string{
		"empty.txt": "",
	}
	emptyResults := buildRawFileResults([]string{"empty.txt"}, emptyContents)
	if len(emptyResults) != 1 {
		t.Fatalf("empty file: got %d results, want 1", len(emptyResults))
	}
	er := emptyResults[0]
	if er.FilePath != "empty.txt" {
		t.Errorf("empty file: FilePath = %q, want %q", er.FilePath, "empty.txt")
	}
	if er.SizeBytes != 0 {
		t.Errorf("empty file: SizeBytes = %d, want 0", er.SizeBytes)
	}
	if er.LineCount != 0 {
		t.Errorf("empty file: LineCount = %d, want 0", er.LineCount)
	}
	if er.FileHash == "" {
		t.Error("empty file: FileHash should not be empty")
	}
	if len(er.Chunks) != 0 {
		t.Errorf("empty file: got %d chunks, want 0", len(er.Chunks))
	}
}

// --- commit indexing tests ---

func TestHandle_IncrementalPath_IndexSinceCalled(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	var capturedSince string
	ci.indexSinceFn = func(_ context.Context, _ pgtype.UUID, _ string, since string) (*commits.Result, error) {
		capturedSince = since
		return &commits.Result{CommitsIndexed: 3, DiffsIndexed: 5, HeadCommitDBID: testUUID(0xCC)}, nil
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("job should complete")
	}
	if capturedSince != "oldcommitsha" {
		t.Errorf("IndexSince sinceCommit = %q, want %q", capturedSince, "oldcommitsha")
	}
}

func TestHandle_FullFallback_IndexAllCalled(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	// Force fallback: no active snapshot.
	q.getActiveSnapBranchFn = func(_ context.Context, _ db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, pgx.ErrNoRows
	}

	var indexAllCalled bool
	var capturedMax int
	ci.indexAllFn = func(_ context.Context, _ pgtype.UUID, _ string, maxCommits int) (*commits.Result, error) {
		indexAllCalled = true
		capturedMax = maxCommits
		return &commits.Result{CommitsIndexed: 10}, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !indexAllCalled {
		t.Error("IndexAll should be called on full fallback")
	}
	if capturedMax != 5000 {
		t.Errorf("maxCommits = %d, want 5000", capturedMax)
	}
}

func TestHandle_CommitIndexingFailure_NonFatal_Incremental(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	ci.indexSinceFn = func(_ context.Context, _ pgtype.UUID, _, _ string) (*commits.Result, error) {
		return nil, errors.New("git log failed")
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	var failedCalled bool
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
		failedCalled = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("job should still complete when commit indexing fails")
	}
	if failedCalled {
		t.Error("SetJobFailed should NOT be called — commit indexing is non-fatal")
	}
}

func TestHandle_CommitIndexingFailure_NonFatal_FullFallback(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	// Force fallback.
	q.getActiveSnapBranchFn = func(_ context.Context, _ db.GetActiveSnapshotForBranchParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, pgx.ErrNoRows
	}

	ci.indexAllFn = func(_ context.Context, _ pgtype.UUID, _ string, _ int) (*commits.Result, error) {
		return nil, errors.New("git log failed")
	}

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	var failedCalled bool
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
		failedCalled = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("job should still complete when commit indexing fails")
	}
	if failedCalled {
		t.Error("SetJobFailed should NOT be called — commit indexing is non-fatal")
	}
}

func TestHandle_UpdateSnapshotCommitID_Incremental(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	ci.indexSinceFn = func(_ context.Context, _ pgtype.UUID, _, _ string) (*commits.Result, error) {
		return &commits.Result{
			CommitsIndexed: 1,
			DiffsIndexed:   2,
			HeadCommitDBID: testUUID(0xCC),
		}, nil
	}

	var snapCommitParams db.UpdateSnapshotCommitIDParams
	var updateCalled bool
	q.updateSnapCommitIDFn = func(_ context.Context, arg db.UpdateSnapshotCommitIDParams) (int64, error) {
		updateCalled = true
		snapCommitParams = arg
		return 1, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !updateCalled {
		t.Fatal("UpdateSnapshotCommitID should be called")
	}
	if snapCommitParams.ProjectID != testUUID(0x02) {
		t.Errorf("ProjectID = %v, want 0x02", snapCommitParams.ProjectID)
	}
	if snapCommitParams.CommitID != testUUID(0xCC) {
		t.Errorf("CommitID = %v, want 0xCC", snapCommitParams.CommitID)
	}
}

func TestHandle_UpdateSnapshotCommitID_FallbackLookup(t *testing.T) {
	repo, ws, p, d, q, mv, ci := defaultMocks()

	// IndexSince returns empty result (HEAD unchanged — no new commits).
	ci.indexSinceFn = func(_ context.Context, _ pgtype.UUID, _, _ string) (*commits.Result, error) {
		return &commits.Result{}, nil
	}

	// The commit already exists in the DB from a prior run.
	q.getCommitByHashFn = func(_ context.Context, arg db.GetCommitByHashParams) (db.Commit, error) {
		if arg.CommitHash == "newcommitsha" {
			return db.Commit{ID: testUUID(0xDD)}, nil
		}
		return db.Commit{}, pgx.ErrNoRows
	}

	var snapCommitParams db.UpdateSnapshotCommitIDParams
	var updateCalled bool
	q.updateSnapCommitIDFn = func(_ context.Context, arg db.UpdateSnapshotCommitIDParams) (int64, error) {
		updateCalled = true
		snapCommitParams = arg
		return 1, nil
	}

	h := newTestHandler(repo, ws, p, d, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !updateCalled {
		t.Fatal("UpdateSnapshotCommitID should be called via fallback lookup")
	}
	if snapCommitParams.CommitID != testUUID(0xDD) {
		t.Errorf("CommitID = %v, want 0xDD (from fallback lookup)", snapCommitParams.CommitID)
	}
}

// --- assertion helpers ---

func assertErrorCategory(t *testing.T, detailsJSON []byte, wantCategory string) {
	t.Helper()
	if len(detailsJSON) == 0 {
		t.Fatal("SetJobFailed was not called (no error details)")
	}
	var details []ErrorDetail
	if err := json.Unmarshal(detailsJSON, &details); err != nil {
		t.Fatalf("invalid error details JSON: %v\nraw: %s", err, detailsJSON)
	}
	if len(details) == 0 {
		t.Fatal("error details array is empty")
	}
	if details[0].Category != wantCategory {
		t.Errorf("error category = %q, want %q\nmessage: %s", details[0].Category, wantCategory, details[0].Message)
	}
}
