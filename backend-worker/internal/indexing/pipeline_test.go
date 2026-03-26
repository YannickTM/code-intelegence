package indexing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/artifact"
	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/vectorstore"
	db "myjungle/datastore/postgres/sqlc"
)

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

// --- mock embedder ---

type mockEmbedder struct {
	embedFn func(ctx context.Context, texts []string) ([][]float32, error)
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return m.embedFn(ctx, texts)
}

// --- mock vector store ---

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

// --- mock querier ---

type pipelineMockQuerier struct {
	db.Querier
	setProgressFn   func(ctx context.Context, arg db.SetIndexingJobProgressParams) error
	upsertContentFn func(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error)
	insertFileFn    func(ctx context.Context, arg db.InsertFileParams) (db.File, error)
	insertSymbolFn  func(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error)
	insertChunkFn   func(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error)
	insertDepFn     func(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error)
}

func (m *pipelineMockQuerier) SetIndexingJobProgress(ctx context.Context, arg db.SetIndexingJobProgressParams) error {
	return m.setProgressFn(ctx, arg)
}

func (m *pipelineMockQuerier) UpsertFileContent(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error) {
	return m.upsertContentFn(ctx, arg)
}

func (m *pipelineMockQuerier) InsertFile(ctx context.Context, arg db.InsertFileParams) (db.File, error) {
	return m.insertFileFn(ctx, arg)
}

func (m *pipelineMockQuerier) InsertSymbol(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error) {
	return m.insertSymbolFn(ctx, arg)
}

func (m *pipelineMockQuerier) InsertCodeChunk(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error) {
	return m.insertChunkFn(ctx, arg)
}

func (m *pipelineMockQuerier) InsertDependency(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error) {
	return m.insertDepFn(ctx, arg)
}

// --- fixtures ---

func noopActivate(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
	return nil
}

func newPipelineMocks() (*pipelineMockQuerier, *mockEmbedder, *mockVectorStore) {
	chunkID := testUUID(0x30)
	mq := &pipelineMockQuerier{
		setProgressFn: func(_ context.Context, _ db.SetIndexingJobProgressParams) error {
			return nil
		},
		upsertContentFn: func(_ context.Context, arg db.UpsertFileContentParams) (db.FileContent, error) {
			return db.FileContent{ID: testUUID(0x10)}, nil
		},
		insertFileFn: func(_ context.Context, arg db.InsertFileParams) (db.File, error) {
			return db.File{ID: testUUID(0x11)}, nil
		},
		insertSymbolFn: func(_ context.Context, arg db.InsertSymbolParams) (db.Symbol, error) {
			return db.Symbol{ID: testUUID(0x20)}, nil
		},
		insertChunkFn: func(_ context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error) {
			id := chunkID
			chunkID.Bytes[0]++ // increment for uniqueness
			return db.CodeChunk{ID: id}, nil
		},
		insertDepFn: func(_ context.Context, arg db.InsertDependencyParams) (db.Dependency, error) {
			return db.Dependency{ID: testUUID(0x40)}, nil
		},
	}

	me := &mockEmbedder{
		embedFn: func(_ context.Context, texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i := range texts {
				vecs[i] = []float32{0.1, 0.2, 0.3}
			}
			return vecs, nil
		},
	}

	mv := &mockVectorStore{
		ensureFn: func(_ context.Context, _ string, _ int32) error { return nil },
		upsertFn: func(_ context.Context, _ string, _ []vectorstore.Point) error { return nil },
	}

	return mq, me, mv
}

func testInput() PipelineInput {
	return PipelineInput{
		ProjectID:    testUUID(0x01),
		SnapshotID:   testUUID(0x02),
		JobID:        testUUID(0x03),
		Branch:       "main",
		GitCommit:    "abc123",
		VersionLabel: "ollama-jina/jina-embeddings-v2-base-en-768",
		Dimensions:   3,
		ParsedFiles: []parser.ParsedFileResult{
			{
				FilePath:  "src/handler.ts",
				Language:  "typescript",
				FileHash:  "hash1",
				SizeBytes: 100,
				LineCount: 10,
				Symbols: []parser.Symbol{
					{SymbolID: "s1", Name: "handler", Kind: "function", SymbolHash: "sh1"},
				},
				Chunks: []parser.Chunk{
					{ChunkID: "c1", SymbolID: "s1", ChunkType: "function", ChunkHash: "ch1", Content: "code here", StartLine: 1, EndLine: 10},
				},
			},
		},
		FileContents: map[string]string{
			"src/handler.ts": "function handler() {}",
		},
	}
}

// --- tests ---

func TestPipeline_FullSuccess(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	var activated bool
	activate := func(_ context.Context, _ pgtype.UUID, branch string, _ pgtype.UUID) error {
		if branch != "main" {
			t.Errorf("branch = %q", branch)
		}
		activated = true
		return nil
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, activate, w, me, mv)
	err := p.Run(context.Background(), testInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !activated {
		t.Error("snapshot should be activated")
	}
}

func TestPipeline_EmbeddingError_NoActivation(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	me.embedFn = func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, errors.New("ollama unreachable")
	}

	var activated bool
	activate := func(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
		activated = true
		return nil
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, activate, w, me, mv)
	err := p.Run(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "embed") {
		t.Errorf("error = %q, want to contain 'embed'", err)
	}
	if activated {
		t.Error("snapshot should NOT be activated on embedding failure")
	}
}

func TestPipeline_VectorStoreError_NoActivation(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	mv.upsertFn = func(_ context.Context, _ string, _ []vectorstore.Point) error {
		return errors.New("qdrant unreachable")
	}

	var activated bool
	activate := func(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
		activated = true
		return nil
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, activate, w, me, mv)
	err := p.Run(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "upsert vectors") {
		t.Errorf("error = %q, want to contain 'upsert vectors'", err)
	}
	if activated {
		t.Error("snapshot should NOT be activated on vector store failure")
	}
}

func TestPipeline_ArtifactWriteError_NoActivation(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	mq.insertFileFn = func(_ context.Context, _ db.InsertFileParams) (db.File, error) {
		return db.File{}, errors.New("constraint violation")
	}

	var activated bool
	activate := func(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
		activated = true
		return nil
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, activate, w, me, mv)
	err := p.Run(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected error")
	}
	if activated {
		t.Error("snapshot should NOT be activated on artifact write failure")
	}
}

func TestPipeline_ProgressUpdated(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	var lastProgress db.SetIndexingJobProgressParams
	mq.setProgressFn = func(_ context.Context, arg db.SetIndexingJobProgressParams) error {
		lastProgress = arg
		return nil
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, noopActivate, w, me, mv)
	err := p.Run(context.Background(), testInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastProgress.FilesProcessed != 1 {
		t.Errorf("FilesProcessed = %d, want 1", lastProgress.FilesProcessed)
	}
	if lastProgress.ChunksUpserted != 1 {
		t.Errorf("ChunksUpserted = %d, want 1", lastProgress.ChunksUpserted)
	}
}

func TestPipeline_EmptyFiles(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	var activated bool
	activate := func(_ context.Context, _ pgtype.UUID, _ string, _ pgtype.UUID) error {
		activated = true
		return nil
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, activate, w, me, mv)
	input := testInput()
	input.ParsedFiles = nil
	input.FileContents = nil

	err := p.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !activated {
		t.Error("snapshot should be activated even with zero files")
	}
}

func TestPipeline_EnsureCollectionError(t *testing.T) {
	mq, me, mv := newPipelineMocks()

	mv.ensureFn = func(_ context.Context, _ string, _ int32) error {
		return errors.New("qdrant down")
	}

	w := artifact.NewWriter(mq)
	p := NewPipeline(mq, noopActivate, w, me, mv)
	err := p.Run(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ensure collection") {
		t.Errorf("error = %q, want to contain 'ensure collection'", err)
	}
}
