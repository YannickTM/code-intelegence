// Package incremental implements the incremental-index workflow handler.
package incremental

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/logger"
	"myjungle/backend-worker/internal/vectorstore"
	db "myjungle/datastore/postgres/sqlc"
)

// CopyForwardResult holds the outcome of copy-forward for unchanged files.
type CopyForwardResult struct {
	FilesCopied    int
	ChunksCopied   int
	ChunkIDMapping map[string]string // old chunk UUID string → new chunk UUID string
}

// CopyForwardFiles copies file artifacts (files, symbols, chunks, dependencies,
// exports, symbol references, JSX usages, network calls) from oldSnapshotID to
// newSnapshotID for unchanged files.
// It returns a mapping of old chunk DB IDs to new chunk DB IDs for vector copy.
func CopyForwardFiles(
	ctx context.Context,
	q db.Querier,
	projectID, oldSnapshotID, newSnapshotID pgtype.UUID,
	unchangedPaths map[string]bool,
	oldFiles []db.File,
) (*CopyForwardResult, error) {
	result := &CopyForwardResult{
		ChunkIDMapping: make(map[string]string),
	}

	for _, oldFile := range oldFiles {
		if !unchangedPaths[oldFile.FilePath] {
			continue
		}

		if err := copyOneFile(ctx, q, projectID, oldSnapshotID, newSnapshotID, oldFile, result); err != nil {
			return nil, fmt.Errorf("copyforward: file %s: %w", oldFile.FilePath, err)
		}
	}

	return result, nil
}

// copyOneFile copies a single file and all its artifacts.
func copyOneFile(
	ctx context.Context,
	q db.Querier,
	projectID, oldSnapshotID, newSnapshotID pgtype.UUID,
	oldFile db.File,
	result *CopyForwardResult,
) error {
	// 1. Copy file row.
	newFile, err := q.InsertFile(ctx, db.InsertFileParams{
		ProjectID:         projectID,
		IndexSnapshotID:   newSnapshotID,
		FilePath:          oldFile.FilePath,
		Language:          oldFile.Language,
		FileHash:          oldFile.FileHash,
		SizeBytes:         oldFile.SizeBytes,
		LineCount:         oldFile.LineCount,
		FileContentID:     oldFile.FileContentID,
		FileFacts:         oldFile.FileFacts,
		ParserMeta:        oldFile.ParserMeta,
		ExtractorStatuses: oldFile.ExtractorStatuses,
		Issues:            oldFile.Issues,
	})
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}

	// 2. Copy symbols (single-pass, ORDER BY start_line guarantees parents first).
	oldSymbols, err := q.ListSymbolsByFileID(ctx, oldFile.ID)
	if err != nil {
		return fmt.Errorf("list symbols: %w", err)
	}

	symbolMap := make(map[[16]byte]pgtype.UUID, len(oldSymbols)) // old UUID bytes → new UUID
	for _, sym := range oldSymbols {
		parentID := pgtype.UUID{}
		if sym.ParentSymbolID.Valid {
			if mapped, ok := symbolMap[sym.ParentSymbolID.Bytes]; ok {
				parentID = mapped
			}
		}

		newSym, err := q.InsertSymbol(ctx, db.InsertSymbolParams{
			ProjectID:       projectID,
			IndexSnapshotID: newSnapshotID,
			FileID:          newFile.ID,
			Name:            sym.Name,
			QualifiedName:   sym.QualifiedName,
			Kind:            sym.Kind,
			Signature:       sym.Signature,
			StartLine:       sym.StartLine,
			EndLine:         sym.EndLine,
			DocText:         sym.DocText,
			SymbolHash:      sym.SymbolHash,
			ParentSymbolID:  parentID,
			Flags:           sym.Flags,
			Modifiers:       sym.Modifiers,
			ReturnType:      sym.ReturnType,
			ParameterTypes:  sym.ParameterTypes,
		})
		if err != nil {
			return fmt.Errorf("insert symbol %s: %w", sym.Name, err)
		}
		symbolMap[sym.ID.Bytes] = newSym.ID
	}

	// 3. Copy chunks (remap file_id and symbol_id).
	oldChunks, err := q.ListChunksByFileID(ctx, oldFile.ID)
	if err != nil {
		return fmt.Errorf("list chunks: %w", err)
	}

	for _, chunk := range oldChunks {
		symbolID := pgtype.UUID{}
		if chunk.SymbolID.Valid {
			if mapped, ok := symbolMap[chunk.SymbolID.Bytes]; ok {
				symbolID = mapped
			}
		}

		newChunk, err := q.InsertCodeChunk(ctx, db.InsertCodeChunkParams{
			ProjectID:          projectID,
			IndexSnapshotID:    newSnapshotID,
			FileID:             newFile.ID,
			SymbolID:           symbolID,
			ChunkType:          chunk.ChunkType,
			ChunkHash:          chunk.ChunkHash,
			Content:            chunk.Content,
			ContextBefore:      chunk.ContextBefore,
			ContextAfter:       chunk.ContextAfter,
			StartLine:          chunk.StartLine,
			EndLine:            chunk.EndLine,
			EstimatedTokens:    chunk.EstimatedTokens,
			OwnerQualifiedName: chunk.OwnerQualifiedName,
			OwnerKind:          chunk.OwnerKind,
			IsExportedContext:  chunk.IsExportedContext,
			SemanticRole:       chunk.SemanticRole,
		})
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}

		result.ChunkIDMapping[formatUUID(chunk.ID)] = formatUUID(newChunk.ID)
		result.ChunksCopied++
	}

	// 4. Copy dependencies (remap source_symbol_id).
	oldDeps, err := q.ListDependenciesBySnapshotAndFile(ctx, db.ListDependenciesBySnapshotAndFileParams{
		IndexSnapshotID: oldSnapshotID,
		SourceFilePath:  oldFile.FilePath,
	})
	if err != nil {
		return fmt.Errorf("list dependencies: %w", err)
	}

	for _, dep := range oldDeps {
		sourceSymbolID := pgtype.UUID{}
		if dep.SourceSymbolID.Valid {
			if mapped, ok := symbolMap[dep.SourceSymbolID.Bytes]; ok {
				sourceSymbolID = mapped
			}
		}

		_, err := q.InsertDependency(ctx, db.InsertDependencyParams{
			ProjectID:       projectID,
			IndexSnapshotID: newSnapshotID,
			SourceSymbolID:  sourceSymbolID,
			SourceFilePath:  dep.SourceFilePath,
			TargetFilePath:  dep.TargetFilePath,
			ImportName:      dep.ImportName,
			ImportType:      dep.ImportType,
			PackageName:     dep.PackageName,
			PackageVersion:  dep.PackageVersion,
		})
		if err != nil {
			return fmt.Errorf("insert dependency: %w", err)
		}
	}

	// 5. Copy exports (remap file_id and symbol_id).
	oldExports, err := q.ListExportsByFileID(ctx, oldFile.ID)
	if err != nil {
		return fmt.Errorf("list exports: %w", err)
	}

	for _, exp := range oldExports {
		symbolID := remapSymbol(exp.SymbolID, symbolMap)
		_, err := q.InsertExport(ctx, db.InsertExportParams{
			ProjectID:       projectID,
			IndexSnapshotID: newSnapshotID,
			FileID:          newFile.ID,
			ExportKind:      exp.ExportKind,
			ExportedName:    exp.ExportedName,
			LocalName:       exp.LocalName,
			SymbolID:        symbolID,
			SourceModule:    exp.SourceModule,
			Line:            exp.Line,
			Column:          exp.Column,
		})
		if err != nil {
			return fmt.Errorf("insert export: %w", err)
		}
	}

	// 6. Copy symbol references (remap file_id, source and target symbol IDs).
	oldRefs, err := q.ListSymbolReferencesByFileID(ctx, oldFile.ID)
	if err != nil {
		return fmt.Errorf("list symbol references: %w", err)
	}

	for _, ref := range oldRefs {
		_, err := q.InsertSymbolReference(ctx, db.InsertSymbolReferenceParams{
			ProjectID:              projectID,
			IndexSnapshotID:        newSnapshotID,
			FileID:                 newFile.ID,
			SourceSymbolID:         remapSymbol(ref.SourceSymbolID, symbolMap),
			ReferenceKind:          ref.ReferenceKind,
			RawText:                ref.RawText,
			TargetName:             ref.TargetName,
			QualifiedTargetHint:    ref.QualifiedTargetHint,
			StartLine:              ref.StartLine,
			StartColumn:            ref.StartColumn,
			EndLine:                ref.EndLine,
			EndColumn:              ref.EndColumn,
			ResolutionScope:        ref.ResolutionScope,
			ResolvedTargetSymbolID: remapSymbol(ref.ResolvedTargetSymbolID, symbolMap),
			Confidence:             ref.Confidence,
		})
		if err != nil {
			return fmt.Errorf("insert symbol reference: %w", err)
		}
	}

	// 7. Copy JSX usages (remap file_id, source and target symbol IDs).
	oldJsx, err := q.ListJsxUsagesByFileID(ctx, oldFile.ID)
	if err != nil {
		return fmt.Errorf("list jsx usages: %w", err)
	}

	for _, jsx := range oldJsx {
		_, err := q.InsertJsxUsage(ctx, db.InsertJsxUsageParams{
			ProjectID:              projectID,
			IndexSnapshotID:        newSnapshotID,
			FileID:                 newFile.ID,
			SourceSymbolID:         remapSymbol(jsx.SourceSymbolID, symbolMap),
			ComponentName:          jsx.ComponentName,
			IsIntrinsic:            jsx.IsIntrinsic,
			IsFragment:             jsx.IsFragment,
			Line:                   jsx.Line,
			Column:                 jsx.Column,
			ResolvedTargetSymbolID: remapSymbol(jsx.ResolvedTargetSymbolID, symbolMap),
			Confidence:             jsx.Confidence,
		})
		if err != nil {
			return fmt.Errorf("insert jsx usage: %w", err)
		}
	}

	// 8. Copy network calls (remap file_id and source_symbol_id).
	oldCalls, err := q.ListNetworkCallsByFileID(ctx, oldFile.ID)
	if err != nil {
		return fmt.Errorf("list network calls: %w", err)
	}

	for _, nc := range oldCalls {
		_, err := q.InsertNetworkCall(ctx, db.InsertNetworkCallParams{
			ProjectID:       projectID,
			IndexSnapshotID: newSnapshotID,
			FileID:          newFile.ID,
			SourceSymbolID:  remapSymbol(nc.SourceSymbolID, symbolMap),
			ClientKind:      nc.ClientKind,
			Method:          nc.Method,
			UrlLiteral:      nc.UrlLiteral,
			UrlTemplate:     nc.UrlTemplate,
			IsRelative:      nc.IsRelative,
			StartLine:       nc.StartLine,
			StartColumn:     nc.StartColumn,
			Confidence:      nc.Confidence,
		})
		if err != nil {
			return fmt.Errorf("insert network call: %w", err)
		}
	}

	result.FilesCopied++
	return nil
}

// remapSymbol looks up an old symbol UUID in the copy-forward mapping and returns
// the new UUID. Returns a NULL UUID if the old ID is not valid or not found.
func remapSymbol(oldID pgtype.UUID, m map[[16]byte]pgtype.UUID) pgtype.UUID {
	if !oldID.Valid {
		return pgtype.UUID{}
	}
	if mapped, ok := m[oldID.Bytes]; ok {
		return mapped
	}
	return pgtype.UUID{}
}

// vectorCopyBatchSize is the maximum number of IDs per GetPoints call.
const vectorCopyBatchSize = 100

// CopyForwardVectors reads vectors for old chunk IDs from Qdrant and
// re-upserts them with new chunk IDs and updated snapshot metadata.
func CopyForwardVectors(
	ctx context.Context,
	vectorDB vectorstore.VectorStore,
	collection string,
	chunkMapping map[string]string,
	newSnapshotID string,
	gitCommit string,
) error {
	if len(chunkMapping) == 0 {
		return nil
	}

	// Collect old IDs.
	oldIDs := make([]string, 0, len(chunkMapping))
	for oldID := range chunkMapping {
		oldIDs = append(oldIDs, oldID)
	}

	// Process in batches.
	for start := 0; start < len(oldIDs); start += vectorCopyBatchSize {
		end := start + vectorCopyBatchSize
		if end > len(oldIDs) {
			end = len(oldIDs)
		}
		batch := oldIDs[start:end]

		points, err := vectorDB.GetPoints(ctx, collection, batch, true)
		if err != nil {
			return fmt.Errorf("copyforward: get vectors: %w", err)
		}

		newPoints := make([]vectorstore.Point, 0, len(points))
		for _, pt := range points {
			newID, ok := chunkMapping[pt.ID]
			if !ok {
				logger.FromContext(ctx).Warn("copyforward: vector not in chunk mapping, skipping",
					slog.String("point_id", pt.ID))
				continue
			}

			// Copy payload and update snapshot-specific fields.
			payload := make(map[string]interface{}, len(pt.Payload))
			for k, v := range pt.Payload {
				payload[k] = v
			}
			payload["index_snapshot_id"] = newSnapshotID
			payload["git_commit"] = gitCommit
			payload["raw_text_ref"] = fmt.Sprintf("db://code_chunks/%s", newID)

			newPoints = append(newPoints, vectorstore.Point{
				ID:      newID,
				Vector:  pt.Vector,
				Payload: payload,
			})
		}

		if len(newPoints) > 0 {
			if err := vectorDB.UpsertPoints(ctx, collection, newPoints); err != nil {
				return fmt.Errorf("copyforward: upsert vectors: %w", err)
			}
		}
	}

	return nil
}
