package fullindex

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
	"myjungle/backend-worker/internal/embedding"
	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/vectorstore"
	"myjungle/backend-worker/internal/workflow"
	"myjungle/backend-worker/internal/workflow/commits"
	"myjungle/backend-worker/internal/workspace"
	db "myjungle/datastore/postgres/sqlc"
)

// --- test helpers ---

func noopActivate(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
	return nil
}

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

// --- mock querier ---

type mockQuerier struct {
	db.Querier
	getVersionFn         func(ctx context.Context, label string) (db.EmbeddingVersion, error)
	createVersionFn      func(ctx context.Context, params db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error)
	setProgressFn        func(ctx context.Context, arg db.SetIndexingJobProgressParams) error
	upsertContentFn      func(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error)
	insertFileFn         func(ctx context.Context, arg db.InsertFileParams) (db.File, error)
	insertSymbolFn       func(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error)
	insertChunkFn        func(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error)
	insertDepFn          func(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error)
	deleteSnapArtifactFn func(ctx context.Context, id pgtype.UUID) error
	updateSnapCommitIDFn func(ctx context.Context, arg db.UpdateSnapshotCommitIDParams) (int64, error)
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

// --- mock commit indexer ---

type mockCommitIndexer struct {
	indexAllFn func(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error)
}

func (m *mockCommitIndexer) IndexAll(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error) {
	if m.indexAllFn != nil {
		return m.indexAllFn(ctx, projectID, repoDir, maxCommits)
	}
	return &commits.Result{}, nil
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
	if m.getPointsFn != nil {
		return m.getPointsFn(ctx, collection, ids, withVector)
	}
	return nil, nil
}

// --- fixtures ---

func defaultExecCtx() *execution.Context {
	return &execution.Context{
		JobID:         testUUID(0x01),
		JobType:       "full-index",
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
		Workflow: "full-index",
	}
}

func defaultMocks() (*mockJobRepo, *mockWorkspace, *mockParser, *mockQuerier, *mockVectorStore, *mockCommitIndexer) {
	embVersion := db.EmbeddingVersion{ID: testUUID(0x10)}
	chunkID := testUUID(0x30)

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
				CommitSHA:   "abc123def456",
				SourceFiles: []string{},
			}, func() {}, nil
		},
	}

	p := &mockParser{
		parseFn: func(_ context.Context, _, _, _ string, _ []parser.FileInput) ([]parser.ParsedFileResult, error) {
			return []parser.ParsedFileResult{}, nil
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
			id := chunkID
			chunkID.Bytes[0]++
			return db.CodeChunk{ID: id}, nil
		},
		insertDepFn: func(_ context.Context, _ db.InsertDependencyParams) (db.Dependency, error) {
			return db.Dependency{ID: testUUID(0x43)}, nil
		},
	}

	mv := &mockVectorStore{
		ensureFn: func(_ context.Context, _ string, _ int32) error { return nil },
		upsertFn: func(_ context.Context, _ string, _ []vectorstore.Point) error { return nil },
	}

	ci := &mockCommitIndexer{}

	return repo, ws, p, q, mv, ci
}

func newTestHandler(repo *mockJobRepo, ws *mockWorkspace, p *mockParser, q *mockQuerier, mv *mockVectorStore, ci *mockCommitIndexer) *Handler {
	var commitIdx commitIndexer
	if ci != nil {
		commitIdx = ci
	}
	return NewHandler(repo, ws, p, q, noopActivate, artifact.NewWriter(q), mv, commitIdx)
}

// --- tests ---

func TestHandle_HappyPath_EmptyFiles(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	var snapshotParams db.CreateIndexSnapshotParams
	repo.createSnapFn = func(_ context.Context, params db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
		snapshotParams = params
		return db.IndexSnapshot{ID: testUUID(0x20)}, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("SetJobCompleted should have been called")
	}
	if snapshotParams.Branch != "main" {
		t.Errorf("snapshot branch = %q, want %q", snapshotParams.Branch, "main")
	}
	if snapshotParams.GitCommit != "abc123def456" {
		t.Errorf("snapshot commit = %q, want %q", snapshotParams.GitCommit, "abc123def456")
	}
}

func TestHandle_InvalidJobID(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var loadCalled bool
	repo.loadCtxFn = func(_ context.Context, _ pgtype.UUID) (*execution.Context, error) {
		loadCalled = true
		return nil, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	task := defaultTask()
	task.JobID = "not-a-uuid"

	err := h.Handle(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid job_id")
	}
	if !strings.Contains(err.Error(), "invalid job_id") {
		t.Errorf("error = %q, want to contain 'invalid job_id'", err)
	}
	if loadCalled {
		t.Error("LoadExecutionContext should not be called for invalid job_id")
	}
}

func TestHandle_LoadContextFailure_ReturnsError(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	repo.loadCtxFn = func(_ context.Context, _ pgtype.UUID) (*execution.Context, error) {
		return nil, errors.New("project not found")
	}

	var failedCalled bool
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
		failedCalled = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err == nil {
		t.Fatal("expected error")
	}
	if failedCalled {
		t.Error("SetJobFailed should NOT be called before ClaimJob")
	}
}

func TestHandle_ClaimFailure_ReturnsError(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	repo.claimFn = func(_ context.Context, _ pgtype.UUID, _ string) error {
		return errors.New("already claimed")
	}

	var failedCalled bool
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
		failedCalled = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err == nil {
		t.Fatal("expected error")
	}
	if failedCalled {
		t.Error("SetJobFailed should NOT be called before ClaimJob succeeds")
	}
}

func TestHandle_ResolveVersionFailure_FailsJob(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	q.getVersionFn = func(_ context.Context, _ string) (db.EmbeddingVersion, error) {
		return db.EmbeddingVersion{}, pgx.ErrNoRows
	}
	q.createVersionFn = func(_ context.Context, _ db.CreateEmbeddingVersionParams) (db.EmbeddingVersion, error) {
		return db.EmbeddingVersion{}, errors.New("create failed")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	assertErrorCategory(t, failDetails, "context_load")
}

func TestHandle_WorkspacePrepareFailure_FailsJob(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return nil, nil, errors.New("clone failed")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	assertErrorCategory(t, failDetails, "repo_access")
}

func TestHandle_ParseFailure_FailsJob_CleanupRuns(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var cleanedUp bool
	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     "/tmp/test",
			CommitSHA:   "abc123",
			SourceFiles: []string{},
		}, func() { cleanedUp = true }, nil
	}

	p.parseFn = func(_ context.Context, _, _, _ string, _ []parser.FileInput) ([]parser.ParsedFileResult, error) {
		return nil, errors.New("parser unavailable")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	assertErrorCategory(t, failDetails, "parser")
	if !cleanedUp {
		t.Error("workspace cleanup should always run")
	}
}

func TestHandle_CreateSnapshotFailure_FailsJob(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	repo.createSnapFn = func(_ context.Context, _ db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, errors.New("constraint violation")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	assertErrorCategory(t, failDetails, "artifact_write")
}

func TestHandle_PipelineEnsureCollectionFailure_FailsJob(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	mv.ensureFn = func(_ context.Context, _ string, _ int32) error {
		return errors.New("qdrant down")
	}

	var failDetails []byte
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, details []byte) (bool, error) {
		failDetails = details
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil after failJob, got: %v", err)
	}

	// Pipeline wraps as "indexing: ensure collection: qdrant down"
	assertErrorCategory(t, failDetails, "vector_write")
}

func TestHandle_CategorizePipelineError(t *testing.T) {
	tests := []struct {
		errMsg  string
		wantCat string
	}{
		{"indexing: ensure collection: qdrant down", "vector_write"},
		{"indexing: embed file src/a.ts: model not found", "embedding"},
		{"indexing: upsert vectors for src/a.ts: qdrant unreachable", "vector_write"},
		{"indexing: activate snapshot: constraint violation", "activation"},
		{"indexing: file 1/5 src/a.ts: insert failed", "artifact_write"},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			got := categorizePipelineError(errors.New(tt.errMsg))
			if got != tt.wantCat {
				t.Errorf("categorizePipelineError(%q) = %q, want %q", tt.errMsg, got, tt.wantCat)
			}
		})
	}
}

func TestLanguageForExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".ts", "typescript"},
		{".tsx", "tsx"},
		{".js", "javascript"},
		{".jsx", "jsx"},
		{".mjs", "javascript"},
		{".cjs", "javascript"},
		{".go", "go"},
		{".py", "python"},
		{".rs", "rust"},
		{".java", "java"},
		{".c", "c"},
		{"", ""},
		{".xyz", ""},
	}
	for _, tt := range tests {
		got := languageForExt(tt.ext)
		if got != tt.want {
			t.Errorf("languageForExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestSplitByParserSupport(t *testing.T) {
	files := []string{"src/index.ts", "README.md", "lib/util.js", "main.go", "config.json", "notes.txt"}
	parseable, raw := splitByParserSupport(files)

	wantParseable := []string{"src/index.ts", "README.md", "lib/util.js", "main.go", "config.json"}
	wantRaw := []string{"notes.txt"}

	if len(parseable) != len(wantParseable) {
		t.Fatalf("parseable = %v, want %v", parseable, wantParseable)
	}
	for i := range wantParseable {
		if parseable[i] != wantParseable[i] {
			t.Errorf("parseable[%d] = %q, want %q", i, parseable[i], wantParseable[i])
		}
	}
	if len(raw) != len(wantRaw) {
		t.Fatalf("raw = %v, want %v", raw, wantRaw)
	}
	for i := range wantRaw {
		if raw[i] != wantRaw[i] {
			t.Errorf("raw[%d] = %q, want %q", i, raw[i], wantRaw[i])
		}
	}
}

func TestBuildRawFileResults(t *testing.T) {
	contents := map[string]string{
		"README.md":   "# Hello\nWorld\n",
		"main.go":     "package main\n",
		"config.json": `{"key": "val"}`,
	}
	files := []string{"README.md", "main.go", "config.json"}

	results := buildRawFileResults(files, contents)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// Check README.md result.
	r := results[0]
	if r.FilePath != "README.md" {
		t.Errorf("FilePath = %q, want %q", r.FilePath, "README.md")
	}
	if r.Language != "markdown" {
		t.Errorf("Language = %q, want %q", r.Language, "markdown")
	}
	if r.SizeBytes != int64(len(contents["README.md"])) {
		t.Errorf("SizeBytes = %d, want %d", r.SizeBytes, len(contents["README.md"]))
	}
	if r.LineCount != 2 {
		t.Errorf("LineCount = %d, want 2", r.LineCount)
	}
	if len(r.Chunks) != 1 {
		t.Fatalf("Chunks = %d, want 1", len(r.Chunks))
	}
	if r.Chunks[0].ChunkType != "raw" {
		t.Errorf("ChunkType = %q, want %q", r.Chunks[0].ChunkType, "raw")
	}
	if r.Chunks[0].Content != contents["README.md"] {
		t.Errorf("Chunk content mismatch")
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}

	// config.json has no trailing newline.
	r3 := results[2]
	if r3.LineCount != 1 {
		t.Errorf("config.json LineCount = %d, want 1", r3.LineCount)
	}
	if r3.Language != "json" {
		t.Errorf("config.json Language = %q, want %q", r3.Language, "json")
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

func TestSplitRawChunks_LargeFile(t *testing.T) {
	// Build a file that is ~3x the max chunk size by repeating short lines.
	// Each line is 50 chars + newline = 51 bytes.
	maxChars := embedding.DefaultMaxInputChars // 7500
	linesPerChunk := maxChars / 51             // ~147 lines fit in one chunk
	totalLines := linesPerChunk*3 + 10         // ensure 4 chunks
	var sb strings.Builder
	for i := 0; i < totalLines; i++ {
		sb.WriteString(strings.Repeat("x", 50))
		sb.WriteByte('\n')
	}
	content := sb.String()

	chunks := splitRawChunks(content, int32(totalLines))

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks for a large file, got %d", len(chunks))
	}

	// Verify every chunk fits within the max size.
	for i, c := range chunks {
		if len(c.Content) > maxChars {
			t.Errorf("chunk %d: len=%d exceeds maxChars=%d", i, len(c.Content), maxChars)
		}
		if c.ChunkType != "raw" {
			t.Errorf("chunk %d: ChunkType=%q, want %q", i, c.ChunkType, "raw")
		}
		if c.ChunkHash == "" {
			t.Errorf("chunk %d: ChunkHash is empty", i)
		}
	}

	// Verify all content is preserved (concatenation of chunks = original).
	var reassembled strings.Builder
	for _, c := range chunks {
		reassembled.WriteString(c.Content)
	}
	if reassembled.String() != content {
		t.Error("reassembled chunks do not match original content")
	}

	// Verify line ranges are contiguous and cover all lines.
	if chunks[0].StartLine != 1 {
		t.Errorf("first chunk StartLine=%d, want 1", chunks[0].StartLine)
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartLine != chunks[i-1].EndLine+1 {
			t.Errorf("chunk %d StartLine=%d, expected %d (prev EndLine=%d)",
				i, chunks[i].StartLine, chunks[i-1].EndLine+1, chunks[i-1].EndLine)
		}
	}
}

func TestSplitRawChunks_SmallFile(t *testing.T) {
	content := "hello\nworld\n"
	chunks := splitRawChunks(content, 2)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small file, got %d", len(chunks))
	}
	if chunks[0].Content != content {
		t.Error("small file chunk content mismatch")
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 2 {
		t.Errorf("small file lines: start=%d end=%d, want 1..2", chunks[0].StartLine, chunks[0].EndLine)
	}
}

func TestSplitOversizedChunks(t *testing.T) {
	maxChars := embedding.DefaultMaxInputChars

	// Build a chunk that's 2.5x the limit.
	var sb strings.Builder
	for sb.Len() < maxChars*2+maxChars/2 {
		sb.WriteString(strings.Repeat("x", 50))
		sb.WriteByte('\n')
	}
	bigContent := sb.String()

	results := []parser.ParsedFileResult{{
		FilePath: "big.ts",
		Chunks: []parser.Chunk{
			{ChunkType: "function", SymbolID: "sym1", Content: "small\n", StartLine: 1, EndLine: 1},
			{ChunkType: "function", SymbolID: "sym2", Content: bigContent, StartLine: 2, EndLine: 500, SemanticRole: "implementation"},
			{ChunkType: "module_context", Content: "also small\n", StartLine: 501, EndLine: 501},
		},
	}}

	splitOversizedChunks(results, maxChars)

	// The small chunks should be untouched.
	if results[0].Chunks[0].Content != "small\n" {
		t.Error("first small chunk was modified")
	}
	lastChunk := results[0].Chunks[len(results[0].Chunks)-1]
	if lastChunk.Content != "also small\n" {
		t.Error("last small chunk was modified")
	}

	// The big chunk should have been split into 3+ sub-chunks.
	// Total chunks = 1 (small) + N (split) + 1 (small) >= 5.
	if len(results[0].Chunks) < 5 {
		t.Fatalf("expected at least 5 chunks after split, got %d", len(results[0].Chunks))
	}

	// Verify metadata is preserved on split chunks.
	for _, c := range results[0].Chunks[1 : len(results[0].Chunks)-1] {
		if c.ChunkType != "function" {
			t.Errorf("split chunk lost ChunkType: got %q", c.ChunkType)
		}
		if c.SymbolID != "sym2" {
			t.Errorf("split chunk lost SymbolID: got %q", c.SymbolID)
		}
		if c.SemanticRole != "implementation" {
			t.Errorf("split chunk lost SemanticRole: got %q", c.SemanticRole)
		}
		if len(c.Content) > maxChars {
			t.Errorf("split chunk exceeds maxChars: %d > %d", len(c.Content), maxChars)
		}
	}

	// Verify content is fully preserved.
	var reassembled strings.Builder
	for _, c := range results[0].Chunks[1 : len(results[0].Chunks)-1] {
		reassembled.WriteString(c.Content)
	}
	if reassembled.String() != bigContent {
		t.Error("split chunk content doesn't reassemble to original")
	}
}

func TestMarshalErrorDetails(t *testing.T) {
	details := []ErrorDetail{{
		Category: "parser",
		Message:  "parser unavailable",
		Step:     "parse_files",
	}}
	b := marshalErrorDetails(details)

	var parsed []ErrorDetail
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("len = %d, want 1", len(parsed))
	}
	if parsed[0].Category != "parser" {
		t.Errorf("category = %q, want %q", parsed[0].Category, "parser")
	}
	if parsed[0].Step != "parse_files" {
		t.Errorf("step = %q, want %q", parsed[0].Step, "parse_files")
	}
}

func TestHandle_WorkspaceCleanupAlwaysRuns(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var cleanedUp bool
	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return &workspace.Result{
			RepoDir:     "/tmp/test",
			CommitSHA:   "abc123",
			SourceFiles: []string{},
		}, func() { cleanedUp = true }, nil
	}

	// Fail at CreateSnapshot (after workspace prep succeeds).
	repo.createSnapFn = func(_ context.Context, _ db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, errors.New("db error")
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	_ = h.Handle(context.Background(), defaultTask())
	if !cleanedUp {
		t.Error("workspace cleanup should run even when CreateSnapshot fails")
	}
}

// --- project lock and stale transition tests ---

func TestHandle_ProjectLockUnavailable_ReturnsError(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	repo.tryLockFn = func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
		return nil, errors.New("repository: project is locked by another worker")
	}

	var failedCalled bool
	repo.setFailedFn = func(_ context.Context, _ pgtype.UUID, _ []byte) (bool, error) {
		failedCalled = true
		return true, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
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
	repo, ws, p, q, mv, ci := defaultMocks()

	// SetJobCompleted returns false (0 rows affected — stale).
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		return false, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("handler should return nil even when completion is stale, got: %v", err)
	}
}

func TestHandle_ProjectLockReleasedOnSuccess(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var unlocked bool
	repo.tryLockFn = func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
		return func(context.Context) { unlocked = true }, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	_ = h.Handle(context.Background(), defaultTask())
	if !unlocked {
		t.Error("project lock should be released on handler return")
	}
}

func TestHandle_ProjectLockReleasedOnFailure(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var unlocked bool
	repo.tryLockFn = func(_ context.Context, _ pgtype.UUID) (func(context.Context), error) {
		return func(context.Context) { unlocked = true }, nil
	}

	// Force a post-claim failure.
	ws.prepareFn = func(_ context.Context, _ *execution.Context) (*workspace.Result, func(), error) {
		return nil, nil, errors.New("clone failed")
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	_ = h.Handle(context.Background(), defaultTask())
	if !unlocked {
		t.Error("project lock should be released even on failure")
	}
}

func TestReadSourceFiles_SkipsBinaryContent(t *testing.T) {
	dir := t.TempDir()

	// Valid UTF-8 text file.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Binary file (PNG magic number 0x89 followed by "PNG").
	if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}, 0o644); err != nil {
		t.Fatal(err)
	}

	contents, err := readSourceFiles(dir, []string{"hello.txt", "image.png"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, ok := contents["hello.txt"]; !ok || got != "hello world\n" {
		t.Errorf("hello.txt content = %q, ok = %v", got, ok)
	}
	if _, ok := contents["image.png"]; ok {
		t.Error("binary file image.png should be skipped (not present in map)")
	}
}

// --- commit indexing tests ---

func TestHandle_CommitIndexingCalled(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	var capturedProjectID pgtype.UUID
	var capturedRepoDir string
	var capturedMaxCommits int
	ci.indexAllFn = func(_ context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error) {
		capturedProjectID = projectID
		capturedRepoDir = repoDir
		capturedMaxCommits = maxCommits
		return &commits.Result{CommitsIndexed: 42, DiffsIndexed: 100, HeadCommitDBID: testUUID(0xCC)}, nil
	}

	h := newTestHandler(repo, ws, p, q, mv, ci)
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedProjectID != testUUID(0x02) {
		t.Errorf("projectID = %v, want 0x02", capturedProjectID)
	}
	if capturedRepoDir != "/tmp/test-worktree" {
		t.Errorf("repoDir = %q, want /tmp/test-worktree", capturedRepoDir)
	}
	if capturedMaxCommits != 5000 {
		t.Errorf("maxCommits = %d, want 5000", capturedMaxCommits)
	}
}

func TestHandle_CommitIndexingFailure_NonFatal(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

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

	h := newTestHandler(repo, ws, p, q, mv, ci)
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

func TestHandle_UpdateSnapshotCommitID_CalledOnSuccess(t *testing.T) {
	repo, ws, p, q, mv, ci := defaultMocks()

	ci.indexAllFn = func(_ context.Context, _ pgtype.UUID, _ string, _ int) (*commits.Result, error) {
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

	h := newTestHandler(repo, ws, p, q, mv, ci)
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
	if snapCommitParams.ID != testUUID(0x20) {
		t.Errorf("snapshot ID = %v, want 0x20", snapCommitParams.ID)
	}
	if snapCommitParams.CommitID != testUUID(0xCC) {
		t.Errorf("CommitID = %v, want 0xCC", snapCommitParams.CommitID)
	}
}

func TestHandle_NilCommitIndexer_Skips(t *testing.T) {
	repo, ws, p, q, mv, _ := defaultMocks()

	var completed bool
	repo.setCompletedFn = func(_ context.Context, _ pgtype.UUID) (bool, error) {
		completed = true
		return true, nil
	}

	// Explicitly set commitIndexer to nil.
	h := newTestHandler(repo, ws, p, q, mv, nil)
	h.commitIndexer = nil
	err := h.Handle(context.Background(), defaultTask())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Error("job should complete even without commit indexer")
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
