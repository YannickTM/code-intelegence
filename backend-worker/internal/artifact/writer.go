// Package artifact persists parsed file artifacts into PostgreSQL.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/parser"
	db "myjungle/datastore/postgres/sqlc"
)

// Writer persists parsed file artifacts into PostgreSQL.
type Writer struct {
	queries db.Querier
}

// NewWriter creates an artifact Writer.
func NewWriter(queries db.Querier) *Writer {
	return &Writer{queries: queries}
}

// WriteResult holds the output of a single file's artifact persistence.
type WriteResult struct {
	FileID pgtype.UUID
	Chunks []ChunkForEmbed
}

// ChunkForEmbed carries the data needed to embed and store a code chunk vector.
type ChunkForEmbed struct {
	ChunkDBID  pgtype.UUID
	Content    string
	FilePath   string
	Language   string
	SymbolName string
	SymbolKind string
	ChunkType  string
	ChunkHash  string
	StartLine  int32
	EndLine    int32
}

// WriteFile persists one parsed file result into PostgreSQL:
//  1. upserts file content (content-addressable dedup by SHA-256)
//  2. inserts the file row (with file facts, parser meta, extractor statuses, issues)
//  3. inserts symbols (with parent linking and v2 flags)
//  4. inserts code chunks (with v2 semantic context)
//  5. inserts dependencies
//  6. inserts exports
//  7. inserts symbol references
//  8. inserts JSX usages
//  9. inserts network calls
func (w *Writer) WriteFile(
	ctx context.Context,
	projectID, snapshotID pgtype.UUID,
	parsed parser.ParsedFileResult,
	fileContent string,
) (*WriteResult, error) {
	// 1. Content deduplication.
	contentHash := sha256Hex(fileContent)
	fc, err := w.queries.UpsertFileContent(ctx, db.UpsertFileContentParams{
		ProjectID:   projectID,
		ContentHash: contentHash,
		Content:     fileContent,
		SizeBytes:   parsed.SizeBytes,
		LineCount:   int32(parsed.LineCount),
	})
	if err != nil {
		return nil, fmt.Errorf("artifact: upsert file content %s: %w", parsed.FilePath, err)
	}

	// 2. Insert file row.
	file, err := w.queries.InsertFile(ctx, db.InsertFileParams{
		ProjectID:         projectID,
		IndexSnapshotID:   snapshotID,
		FilePath:          parsed.FilePath,
		Language:          textOrNull(parsed.Language),
		FileHash:          parsed.FileHash,
		SizeBytes:         pgtype.Int8{Int64: parsed.SizeBytes, Valid: true},
		LineCount:         pgtype.Int4{Int32: parsed.LineCount, Valid: true},
		FileContentID:     fc.ID,
		FileFacts:         jsonbOrNull(parsed.Facts),
		ParserMeta:        jsonbOrNull(parsed.Metadata),
		ExtractorStatuses: jsonbOrNull(parsed.ExtractorStatuses),
		Issues:            jsonbOrNull(parsed.Issues),
	})
	if err != nil {
		return nil, fmt.Errorf("artifact: insert file %s: %w", parsed.FilePath, err)
	}

	// 3. Insert symbols — single-pass with running parent map.
	// Parser emits symbols in AST order (parents before children).
	type symbolInfo struct {
		Name string
		Kind string
	}
	symbolMap := make(map[string]pgtype.UUID, len(parsed.Symbols))
	symbolInfoMap := make(map[string]symbolInfo, len(parsed.Symbols))
	for _, sym := range parsed.Symbols {
		parentID := pgtype.UUID{} // NULL by default
		if sym.ParentSymbolID != "" {
			if dbID, ok := symbolMap[sym.ParentSymbolID]; ok {
				parentID = dbID
			}
		}
		dbSym, err := w.queries.InsertSymbol(ctx, db.InsertSymbolParams{
			ProjectID:       projectID,
			IndexSnapshotID: snapshotID,
			FileID:          file.ID,
			Name:            sym.Name,
			QualifiedName:   textOrNull(sym.QualifiedName),
			Kind:            sym.Kind,
			Signature:       textOrNull(sym.Signature),
			StartLine:       pgtype.Int4{Int32: sym.StartLine, Valid: true},
			EndLine:         pgtype.Int4{Int32: sym.EndLine, Valid: true},
			DocText:         textOrNull(sym.DocText),
			SymbolHash:      sym.SymbolHash,
			ParentSymbolID:  parentID,
			Flags:           jsonbOrNull(sym.Flags),
			Modifiers:       sym.Modifiers,
			ReturnType:      textOrNull(sym.ReturnType),
			ParameterTypes:  sym.ParameterTypes,
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert symbol %s in %s: %w", sym.Name, parsed.FilePath, err)
		}
		if sym.SymbolID != "" {
			symbolMap[sym.SymbolID] = dbSym.ID
			symbolInfoMap[sym.SymbolID] = symbolInfo{Name: sym.Name, Kind: sym.Kind}
		}
	}

	// 4. Insert code chunks.
	chunks := make([]ChunkForEmbed, 0, len(parsed.Chunks))
	for _, ch := range parsed.Chunks {
		symbolID := pgtype.UUID{} // NULL
		symbolName := ""
		symbolKind := ""
		if ch.SymbolID != "" {
			if dbID, ok := symbolMap[ch.SymbolID]; ok {
				symbolID = dbID
			}
			// Resolve symbol name/kind for vector metadata (O(1) lookup).
			if info, ok := symbolInfoMap[ch.SymbolID]; ok {
				symbolName = info.Name
				symbolKind = info.Kind
			}
		}

		dbChunk, err := w.queries.InsertCodeChunk(ctx, db.InsertCodeChunkParams{
			ProjectID:          projectID,
			IndexSnapshotID:    snapshotID,
			FileID:             file.ID,
			SymbolID:           symbolID,
			ChunkType:          ch.ChunkType,
			ChunkHash:          ch.ChunkHash,
			Content:            ch.Content,
			ContextBefore:      textOrNull(ch.ContextBefore),
			ContextAfter:       textOrNull(ch.ContextAfter),
			StartLine:          ch.StartLine,
			EndLine:            ch.EndLine,
			EstimatedTokens:    pgtype.Int4{Int32: ch.EstimatedTokens, Valid: ch.EstimatedTokens > 0},
			OwnerQualifiedName: textOrNull(ch.OwnerQualifiedName),
			OwnerKind:          textOrNull(ch.OwnerKind),
			IsExportedContext:  ch.IsExportedContext,
			SemanticRole:       textOrNull(ch.SemanticRole),
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert chunk in %s: %w", parsed.FilePath, err)
		}
		chunks = append(chunks, ChunkForEmbed{
			ChunkDBID:  dbChunk.ID,
			Content:    ch.Content,
			FilePath:   parsed.FilePath,
			Language:   parsed.Language,
			SymbolName: symbolName,
			SymbolKind: symbolKind,
			ChunkType:  ch.ChunkType,
			ChunkHash:  ch.ChunkHash,
			StartLine:  ch.StartLine,
			EndLine:    ch.EndLine,
		})
	}

	// 5. Insert dependencies.
	for _, imp := range parsed.Imports {
		sourceSymID := resolveSymbol(imp.SourceSymbolID, symbolMap)
		_, err := w.queries.InsertDependency(ctx, db.InsertDependencyParams{
			ProjectID:       projectID,
			IndexSnapshotID: snapshotID,
			SourceSymbolID:  sourceSymID,
			SourceFilePath:  parsed.FilePath,
			TargetFilePath:  textOrNull(imp.TargetFilePath),
			ImportName:      imp.ImportName,
			ImportType:      imp.ImportType,
			PackageName:     textOrNull(imp.PackageName),
			PackageVersion:  textOrNull(imp.PackageVersion),
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert dependency %s in %s: %w", imp.ImportName, parsed.FilePath, err)
		}
	}

	// 6. Insert exports.
	for _, exp := range parsed.Exports {
		_, err := w.queries.InsertExport(ctx, db.InsertExportParams{
			ProjectID:       projectID,
			IndexSnapshotID: snapshotID,
			FileID:          file.ID,
			ExportKind:      exp.ExportKind,
			ExportedName:    exp.ExportedName,
			LocalName:       textOrNull(exp.LocalName),
			SymbolID:        resolveSymbol(exp.SymbolID, symbolMap),
			SourceModule:    textOrNull(exp.SourceModule),
			Line:            exp.Line,
			Column:          exp.Column,
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert export %s in %s: %w", exp.ExportedName, parsed.FilePath, err)
		}
	}

	// 7. Insert symbol references.
	for _, ref := range parsed.References {
		_, err := w.queries.InsertSymbolReference(ctx, db.InsertSymbolReferenceParams{
			ProjectID:              projectID,
			IndexSnapshotID:        snapshotID,
			FileID:                 file.ID,
			SourceSymbolID:         resolveSymbol(ref.SourceSymbolID, symbolMap),
			ReferenceKind:          ref.ReferenceKind,
			RawText:                textOrNull(ref.RawText),
			TargetName:             ref.TargetName,
			QualifiedTargetHint:    textOrNull(ref.QualifiedTargetHint),
			StartLine:              ref.StartLine,
			StartColumn:            ref.StartColumn,
			EndLine:                ref.EndLine,
			EndColumn:              ref.EndColumn,
			ResolutionScope:        textOrNull(ref.ResolutionScope),
			ResolvedTargetSymbolID: resolveSymbol(ref.ResolvedTargetSymbolID, symbolMap),
			Confidence:             textOrNull(ref.Confidence),
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert reference %s in %s: %w", ref.TargetName, parsed.FilePath, err)
		}
	}

	// 8. Insert JSX usages.
	for _, jsx := range parsed.JsxUsages {
		_, err := w.queries.InsertJsxUsage(ctx, db.InsertJsxUsageParams{
			ProjectID:              projectID,
			IndexSnapshotID:        snapshotID,
			FileID:                 file.ID,
			SourceSymbolID:         resolveSymbol(jsx.SourceSymbolID, symbolMap),
			ComponentName:          jsx.ComponentName,
			IsIntrinsic:            jsx.IsIntrinsic,
			IsFragment:             jsx.IsFragment,
			Line:                   jsx.Line,
			Column:                 jsx.Column,
			ResolvedTargetSymbolID: resolveSymbol(jsx.ResolvedTargetSymbolID, symbolMap),
			Confidence:             textOrNull(jsx.Confidence),
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert jsx usage %s in %s: %w", jsx.ComponentName, parsed.FilePath, err)
		}
	}

	// 9. Insert network calls.
	for _, nc := range parsed.NetworkCalls {
		_, err := w.queries.InsertNetworkCall(ctx, db.InsertNetworkCallParams{
			ProjectID:       projectID,
			IndexSnapshotID: snapshotID,
			FileID:          file.ID,
			SourceSymbolID:  resolveSymbol(nc.SourceSymbolID, symbolMap),
			ClientKind:      nc.ClientKind,
			Method:          nc.Method,
			UrlLiteral:      textOrNull(nc.URLLiteral),
			UrlTemplate:     textOrNull(nc.URLTemplate),
			IsRelative:      nc.IsRelative,
			StartLine:       nc.StartLine,
			StartColumn:     nc.StartColumn,
			Confidence:      textOrNull(nc.Confidence),
		})
		if err != nil {
			return nil, fmt.Errorf("artifact: insert network call in %s: %w", parsed.FilePath, err)
		}
	}

	return &WriteResult{
		FileID: file.ID,
		Chunks: chunks,
	}, nil
}

// sha256Hex returns the SHA-256 hex digest of s.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// textOrNull returns a valid pgtype.Text if s is non-empty, NULL otherwise.
func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// jsonbOrNull serializes v to JSON bytes, returning nil (SQL NULL) if v is nil.
func jsonbOrNull(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// resolveSymbol looks up a parser-local symbol ID in the map and returns the
// database UUID. Returns a NULL UUID if the ID is empty or not found.
func resolveSymbol(id string, m map[string]pgtype.UUID) pgtype.UUID {
	if id == "" {
		return pgtype.UUID{}
	}
	if dbID, ok := m[id]; ok {
		return dbID
	}
	return pgtype.UUID{}
}
