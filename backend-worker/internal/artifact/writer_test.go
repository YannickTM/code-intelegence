package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/parser"
	db "myjungle/datastore/postgres/sqlc"
)

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

var (
	projectID  = testUUID(0x01)
	snapshotID = testUUID(0x02)
)

// --- mock querier ---

type writerMockQuerier struct {
	db.Querier
	upsertFileContentFn    func(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error)
	insertFileFn           func(ctx context.Context, arg db.InsertFileParams) (db.File, error)
	insertSymbolFn         func(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error)
	insertCodeChunkFn      func(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error)
	insertDependencyFn     func(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error)
	insertExportFn         func(ctx context.Context, arg db.InsertExportParams) (db.Export, error)
	insertSymbolRefFn      func(ctx context.Context, arg db.InsertSymbolReferenceParams) (db.SymbolReference, error)
	insertJsxUsageFn       func(ctx context.Context, arg db.InsertJsxUsageParams) (db.JsxUsage, error)
	insertNetworkCallFn    func(ctx context.Context, arg db.InsertNetworkCallParams) (db.NetworkCall, error)
}

func (m *writerMockQuerier) UpsertFileContent(ctx context.Context, arg db.UpsertFileContentParams) (db.FileContent, error) {
	return m.upsertFileContentFn(ctx, arg)
}

func (m *writerMockQuerier) InsertFile(ctx context.Context, arg db.InsertFileParams) (db.File, error) {
	return m.insertFileFn(ctx, arg)
}

func (m *writerMockQuerier) InsertSymbol(ctx context.Context, arg db.InsertSymbolParams) (db.Symbol, error) {
	return m.insertSymbolFn(ctx, arg)
}

func (m *writerMockQuerier) InsertCodeChunk(ctx context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error) {
	return m.insertCodeChunkFn(ctx, arg)
}

func (m *writerMockQuerier) InsertDependency(ctx context.Context, arg db.InsertDependencyParams) (db.Dependency, error) {
	return m.insertDependencyFn(ctx, arg)
}

func (m *writerMockQuerier) InsertExport(ctx context.Context, arg db.InsertExportParams) (db.Export, error) {
	if m.insertExportFn != nil {
		return m.insertExportFn(ctx, arg)
	}
	return db.Export{ID: testUUID(0xE0)}, nil
}

func (m *writerMockQuerier) InsertSymbolReference(ctx context.Context, arg db.InsertSymbolReferenceParams) (db.SymbolReference, error) {
	if m.insertSymbolRefFn != nil {
		return m.insertSymbolRefFn(ctx, arg)
	}
	return db.SymbolReference{ID: testUUID(0xE1)}, nil
}

func (m *writerMockQuerier) InsertJsxUsage(ctx context.Context, arg db.InsertJsxUsageParams) (db.JsxUsage, error) {
	if m.insertJsxUsageFn != nil {
		return m.insertJsxUsageFn(ctx, arg)
	}
	return db.JsxUsage{ID: testUUID(0xE2)}, nil
}

func (m *writerMockQuerier) InsertNetworkCall(ctx context.Context, arg db.InsertNetworkCallParams) (db.NetworkCall, error) {
	if m.insertNetworkCallFn != nil {
		return m.insertNetworkCallFn(ctx, arg)
	}
	return db.NetworkCall{ID: testUUID(0xE3)}, nil
}

// --- fixtures ---

func testParsedFile() parser.ParsedFileResult {
	return parser.ParsedFileResult{
		FilePath:  "src/handler.ts",
		Language:  "typescript",
		FileHash:  "abc123",
		SizeBytes: 1024,
		LineCount: 50,
		Symbols: []parser.Symbol{
			{
				SymbolID:      "sym-1",
				Name:          "handleRequest",
				QualifiedName: "handler.handleRequest",
				Kind:          "function",
				Signature:     "function handleRequest(req: Request): Response",
				StartLine:     10,
				EndLine:       30,
				DocText:       "Handles incoming requests",
				SymbolHash:    "symhash1",
				Flags: &parser.SymbolFlags{
					IsExported: true,
					IsAsync:    true,
				},
				ReturnType:     "Response",
				ParameterTypes: []string{"Request"},
			},
		},
		Chunks: []parser.Chunk{
			{
				ChunkID:            "chunk-1",
				SymbolID:           "sym-1",
				ChunkType:          "function",
				ChunkHash:          "chunkhash1",
				Content:            "function handleRequest(req: Request): Response { ... }",
				ContextBefore:      "import { Request } from 'http';",
				ContextAfter:       "",
				StartLine:          10,
				EndLine:            30,
				EstimatedTokens:    50,
				OwnerQualifiedName: "handler.handleRequest",
				OwnerKind:          "function",
				IsExportedContext:  true,
				SemanticRole:       "api_surface",
			},
		},
		Imports: []parser.Import{
			{
				SourceFilePath: "src/handler.ts",
				TargetFilePath: "src/utils.ts",
				ImportName:     "formatDate",
				ImportType:     "INTERNAL",
			},
		},
		Exports: []parser.Export{
			{
				ExportKind:   "NAMED",
				ExportedName: "handleRequest",
				LocalName:    "handleRequest",
				SymbolID:     "sym-1",
				Line:         10,
				Column:       0,
			},
		},
		References: []parser.Reference{
			{
				SourceSymbolID:  "sym-1",
				ReferenceKind:   "CALL",
				TargetName:      "formatDate",
				StartLine:       15,
				StartColumn:     4,
				EndLine:         15,
				EndColumn:       14,
				ResolutionScope: "IMPORTED",
				Confidence:      "HIGH",
			},
		},
		JsxUsages: []parser.JsxUsage{
			{
				SourceSymbolID: "sym-1",
				ComponentName:  "Button",
				Line:           20,
				Column:         8,
				Confidence:     "HIGH",
			},
		},
		NetworkCalls: []parser.NetworkCall{
			{
				SourceSymbolID: "sym-1",
				ClientKind:     "FETCH",
				Method:         "GET",
				URLLiteral:     "/api/users",
				IsRelative:     true,
				StartLine:      25,
				StartColumn:    4,
				Confidence:     "HIGH",
			},
		},
		Facts: &parser.FileFacts{
			HasDefaultExport: false,
			HasNamedExports:  true,
			HasFetchCalls:    true,
		},
		Issues: []parser.Issue{
			{
				Code:     "LONG_FUNCTION",
				Message:  "Function exceeds 200 lines",
				Line:     10,
				Severity: "warning",
			},
		},
	}
}

func newSuccessMock() *writerMockQuerier {
	fileContentID := testUUID(0x10)
	fileID := testUUID(0x11)
	symbolCounter := byte(0x20)
	chunkCounter := byte(0x30)
	depCounter := byte(0x40)
	exportCounter := byte(0x50)
	refCounter := byte(0x60)
	jsxCounter := byte(0x70)
	ncCounter := byte(0x80)

	return &writerMockQuerier{
		upsertFileContentFn: func(_ context.Context, arg db.UpsertFileContentParams) (db.FileContent, error) {
			return db.FileContent{ID: fileContentID, ContentHash: arg.ContentHash}, nil
		},
		insertFileFn: func(_ context.Context, arg db.InsertFileParams) (db.File, error) {
			return db.File{ID: fileID, FilePath: arg.FilePath}, nil
		},
		insertSymbolFn: func(_ context.Context, arg db.InsertSymbolParams) (db.Symbol, error) {
			id := testUUID(symbolCounter)
			symbolCounter++
			return db.Symbol{ID: id, Name: arg.Name}, nil
		},
		insertCodeChunkFn: func(_ context.Context, arg db.InsertCodeChunkParams) (db.CodeChunk, error) {
			id := testUUID(chunkCounter)
			chunkCounter++
			return db.CodeChunk{ID: id, ChunkType: arg.ChunkType}, nil
		},
		insertDependencyFn: func(_ context.Context, arg db.InsertDependencyParams) (db.Dependency, error) {
			id := testUUID(depCounter)
			depCounter++
			return db.Dependency{ID: id}, nil
		},
		insertExportFn: func(_ context.Context, arg db.InsertExportParams) (db.Export, error) {
			id := testUUID(exportCounter)
			exportCounter++
			return db.Export{ID: id}, nil
		},
		insertSymbolRefFn: func(_ context.Context, arg db.InsertSymbolReferenceParams) (db.SymbolReference, error) {
			id := testUUID(refCounter)
			refCounter++
			return db.SymbolReference{ID: id}, nil
		},
		insertJsxUsageFn: func(_ context.Context, arg db.InsertJsxUsageParams) (db.JsxUsage, error) {
			id := testUUID(jsxCounter)
			jsxCounter++
			return db.JsxUsage{ID: id}, nil
		},
		insertNetworkCallFn: func(_ context.Context, arg db.InsertNetworkCallParams) (db.NetworkCall, error) {
			id := testUUID(ncCounter)
			ncCounter++
			return db.NetworkCall{ID: id}, nil
		},
	}
}

// --- tests ---

func TestWriteFile_Success(t *testing.T) {
	m := newSuccessMock()
	w := NewWriter(m)

	result, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "file content here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FileID.Valid {
		t.Error("FileID should be valid")
	}
	if len(result.Chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(result.Chunks))
	}
	if result.Chunks[0].FilePath != "src/handler.ts" {
		t.Errorf("chunk FilePath = %q", result.Chunks[0].FilePath)
	}
	if result.Chunks[0].SymbolName != "handleRequest" {
		t.Errorf("chunk SymbolName = %q", result.Chunks[0].SymbolName)
	}
	if result.Chunks[0].ChunkType != "function" {
		t.Errorf("chunk ChunkType = %q", result.Chunks[0].ChunkType)
	}
}

func TestWriteFile_ContentHashIsSHA256(t *testing.T) {
	content := "export const x = 42;"
	expectedHash := sha256Hex(content)

	var gotHash string
	m := newSuccessMock()
	m.upsertFileContentFn = func(_ context.Context, arg db.UpsertFileContentParams) (db.FileContent, error) {
		gotHash = arg.ContentHash
		return db.FileContent{ID: testUUID(0x10), ContentHash: arg.ContentHash}, nil
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotHash != expectedHash {
		t.Errorf("hash = %q, want %q", gotHash, expectedHash)
	}

	// Verify it's a proper SHA-256.
	h := sha256.Sum256([]byte(content))
	if gotHash != hex.EncodeToString(h[:]) {
		t.Error("hash is not a valid SHA-256")
	}
}

func TestWriteFile_SymbolParentLinking(t *testing.T) {
	parsed := parser.ParsedFileResult{
		FilePath:  "src/index.ts",
		Language:  "typescript",
		FileHash:  "abc",
		SizeBytes: 100,
		LineCount: 10,
		Symbols: []parser.Symbol{
			{SymbolID: "parent-1", Name: "MyClass", Kind: "class", SymbolHash: "h1"},
			{SymbolID: "child-1", Name: "myMethod", Kind: "method", SymbolHash: "h2", ParentSymbolID: "parent-1"},
		},
	}

	parentDBID := testUUID(0xA0)
	callCount := 0
	var gotParentID pgtype.UUID

	m := newSuccessMock()
	m.insertSymbolFn = func(_ context.Context, arg db.InsertSymbolParams) (db.Symbol, error) {
		callCount++
		if callCount == 1 {
			// First call: parent — should have no parent.
			if arg.ParentSymbolID.Valid {
				t.Error("parent should have no ParentSymbolID")
			}
			return db.Symbol{ID: parentDBID, Name: arg.Name}, nil
		}
		// Second call: child — should reference parent.
		gotParentID = arg.ParentSymbolID
		return db.Symbol{ID: testUUID(0xA1), Name: arg.Name}, nil
	}

	w := NewWriter(m)
	_, err := w.WriteFile(context.Background(), projectID, snapshotID, parsed, "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotParentID != parentDBID {
		t.Errorf("child ParentSymbolID = %v, want %v", gotParentID, parentDBID)
	}
}

func TestWriteFile_V2Exports(t *testing.T) {
	var gotExport db.InsertExportParams
	m := newSuccessMock()
	m.insertExportFn = func(_ context.Context, arg db.InsertExportParams) (db.Export, error) {
		gotExport = arg
		return db.Export{ID: testUUID(0x50)}, nil
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotExport.ExportedName != "handleRequest" {
		t.Errorf("export name = %q, want %q", gotExport.ExportedName, "handleRequest")
	}
	if gotExport.ExportKind != "NAMED" {
		t.Errorf("export kind = %q, want %q", gotExport.ExportKind, "NAMED")
	}
	// SymbolID should be resolved (sym-1 → DB UUID).
	if !gotExport.SymbolID.Valid {
		t.Error("export SymbolID should be resolved to a valid UUID")
	}
}

func TestWriteFile_V2References(t *testing.T) {
	var gotRef db.InsertSymbolReferenceParams
	m := newSuccessMock()
	m.insertSymbolRefFn = func(_ context.Context, arg db.InsertSymbolReferenceParams) (db.SymbolReference, error) {
		gotRef = arg
		return db.SymbolReference{ID: testUUID(0x60)}, nil
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRef.TargetName != "formatDate" {
		t.Errorf("ref target = %q, want %q", gotRef.TargetName, "formatDate")
	}
	if gotRef.ReferenceKind != "CALL" {
		t.Errorf("ref kind = %q, want %q", gotRef.ReferenceKind, "CALL")
	}
}

func TestWriteFile_V2JsxUsages(t *testing.T) {
	var gotJsx db.InsertJsxUsageParams
	m := newSuccessMock()
	m.insertJsxUsageFn = func(_ context.Context, arg db.InsertJsxUsageParams) (db.JsxUsage, error) {
		gotJsx = arg
		return db.JsxUsage{ID: testUUID(0x70)}, nil
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotJsx.ComponentName != "Button" {
		t.Errorf("jsx component = %q, want %q", gotJsx.ComponentName, "Button")
	}
	if gotJsx.Line != 20 {
		t.Errorf("jsx line = %d, want %d", gotJsx.Line, 20)
	}
	// SourceSymbolID should be resolved (sym-1 → DB UUID).
	if !gotJsx.SourceSymbolID.Valid {
		t.Error("jsx SourceSymbolID should be resolved to a valid UUID")
	}
}

func TestWriteFile_V2NetworkCalls(t *testing.T) {
	var gotNC db.InsertNetworkCallParams
	m := newSuccessMock()
	m.insertNetworkCallFn = func(_ context.Context, arg db.InsertNetworkCallParams) (db.NetworkCall, error) {
		gotNC = arg
		return db.NetworkCall{ID: testUUID(0x80)}, nil
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotNC.ClientKind != "FETCH" {
		t.Errorf("network call client = %q, want %q", gotNC.ClientKind, "FETCH")
	}
	if gotNC.Method != "GET" {
		t.Errorf("network call method = %q, want %q", gotNC.Method, "GET")
	}
	if !gotNC.IsRelative {
		t.Error("network call should be relative")
	}
}

func TestWriteFile_UpsertContentError(t *testing.T) {
	m := newSuccessMock()
	m.upsertFileContentFn = func(_ context.Context, _ db.UpsertFileContentParams) (db.FileContent, error) {
		return db.FileContent{}, errors.New("db error")
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "content")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "upsert file content") {
		t.Errorf("error = %q, want to contain %q", err, "upsert file content")
	}
}

func TestWriteFile_InsertFileError(t *testing.T) {
	m := newSuccessMock()
	m.insertFileFn = func(_ context.Context, _ db.InsertFileParams) (db.File, error) {
		return db.File{}, errors.New("constraint violation")
	}
	w := NewWriter(m)

	_, err := w.WriteFile(context.Background(), projectID, snapshotID, testParsedFile(), "content")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "insert file") {
		t.Errorf("error = %q, want to contain %q", err, "insert file")
	}
}
