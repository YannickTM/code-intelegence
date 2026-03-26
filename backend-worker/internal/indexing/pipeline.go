// Package indexing provides the storage pipeline that turns parser output
// into durable PostgreSQL artifacts, Qdrant vectors, and an activated snapshot.
package indexing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/artifact"
	"myjungle/backend-worker/internal/embedding"
	"myjungle/backend-worker/internal/logger"
	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/vectorstore"
	db "myjungle/datastore/postgres/sqlc"
)

// TxBeginner starts a database transaction. *pgxpool.Pool satisfies this.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ActivateFunc atomically deactivates the current active snapshot for a
// (project, branch) pair and activates a new one.
type ActivateFunc func(ctx context.Context, projectID pgtype.UUID, branch string, snapshotID pgtype.UUID) error

// TxActivator returns an ActivateFunc that wraps deactivation and activation
// in a database transaction, so the branch is never left without an active
// snapshot if the activation step fails.
func TxActivator(pool TxBeginner) ActivateFunc {
	return func(ctx context.Context, projectID pgtype.UUID, branch string, snapshotID pgtype.UUID) error {
		return ActivateSnapshotTx(ctx, pool, projectID, branch, snapshotID)
	}
}

// Pipeline coordinates the full indexing storage pipeline.
type Pipeline struct {
	queries  db.Querier
	activate ActivateFunc
	writer   *artifact.Writer
	embedder embedding.Embedder
	vectorDB vectorstore.VectorStore
}

// NewPipeline creates a storage pipeline.
func NewPipeline(q db.Querier, activate ActivateFunc, w *artifact.Writer, e embedding.Embedder, v vectorstore.VectorStore) *Pipeline {
	return &Pipeline{
		queries:  q,
		activate: activate,
		writer:   w,
		embedder: e,
		vectorDB: v,
	}
}

// ActivateSnapshotTx deactivates the current active snapshot for the
// (project, branch) pair and activates the given one inside a single
// database transaction. If activation fails, the deactivation is rolled
// back so the branch is never left without an active snapshot.
func ActivateSnapshotTx(ctx context.Context, pool TxBeginner, projectID pgtype.UUID, branch string, snapshotID pgtype.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txq := db.New(tx)
	if err := txq.DeactivateActiveSnapshot(ctx, db.DeactivateActiveSnapshotParams{
		ProjectID: projectID,
		Branch:    branch,
	}); err != nil {
		return fmt.Errorf("deactivate snapshot: %w", err)
	}
	if err := txq.ActivateSnapshot(ctx, snapshotID); err != nil {
		return fmt.Errorf("activate snapshot: %w", err)
	}

	return tx.Commit(ctx)
}

// PipelineInput holds everything needed to run the storage pipeline.
type PipelineInput struct {
	ProjectID    pgtype.UUID
	SnapshotID   pgtype.UUID
	JobID        pgtype.UUID
	Branch       string
	GitCommit    string
	VersionLabel string
	Dimensions   int32
	ParsedFiles  []parser.ParsedFileResult
	FileContents map[string]string // file_path → raw content
	// AllFilePaths lists every file in the project (changed + unchanged).
	// Used by ResolveImportTargets to resolve extensionless imports that
	// reference files outside the ParsedFiles slice (e.g. unchanged files
	// during incremental indexing). Nil means ParsedFiles is the full set.
	AllFilePaths []string
	// GoModulePath is the Go module path from go.mod (e.g. "github.com/user/repo").
	// Used to classify Go imports as INTERNAL vs EXTERNAL. The caller should
	// extract this from go.mod at the repository root; during incremental
	// indexing go.mod may not be in FileContents, so this field ensures the
	// module path is always available.
	GoModulePath string
}

// Run executes the full pipeline:
//  1. ensure Qdrant collection exists
//  2. for each parsed file: persist artifacts, embed chunks, upsert vectors
//  3. activate snapshot only after all writes succeed
func (p *Pipeline) Run(ctx context.Context, input PipelineInput) error {
	if err := p.processFiles(ctx, input); err != nil {
		return err
	}

	// Activate snapshot (transactional in production via TxActivator).
	if err := p.activate(ctx, input.ProjectID, input.Branch, input.SnapshotID); err != nil {
		return fmt.Errorf("indexing: activate snapshot: %w", err)
	}

	logger.FromContext(ctx).Info("indexing: snapshot activated",
		slog.String("collection", vectorstore.CollectionName(input.ProjectID, input.VersionLabel)))

	return nil
}

// RunWithoutActivation executes the pipeline without activating the snapshot.
// This is used by the incremental workflow, which activates after copy-forward.
func (p *Pipeline) RunWithoutActivation(ctx context.Context, input PipelineInput) error {
	return p.processFiles(ctx, input)
}

// vectorUpsertBatchSize is the max number of points per Qdrant upsert call.
const vectorUpsertBatchSize = 100

// processFiles persists artifacts, embeds chunks, and upserts vectors for all files.
// The pipeline runs in three phases to maximize throughput:
//  1. Persist all artifacts to PostgreSQL (sequential, fast)
//  2. Embed all chunks in cross-file batches (batched Ollama calls)
//  3. Upsert all vectors to Qdrant in batches
func (p *Pipeline) processFiles(ctx context.Context, input PipelineInput) error {
	log := logger.FromContext(ctx)
	collection := vectorstore.CollectionName(input.ProjectID, input.VersionLabel)

	if err := p.vectorDB.EnsureCollection(ctx, collection, input.Dimensions); err != nil {
		return fmt.Errorf("indexing: ensure collection: %w", err)
	}

	// Detect Go module path for cross-language import resolution.
	// Prefer the explicit PipelineInput field (always available, including
	// incremental indexing). Fall back to extracting from FileContents for
	// callers that don't set it.
	goModulePath := input.GoModulePath
	if goModulePath == "" {
		if content, ok := input.FileContents["go.mod"]; ok {
			goModulePath = ExtractGoModulePath(content)
		}
	}

	// Resolve extensionless import targets (e.g. "src/foo" → "src/foo.ts")
	// against the known file set before persisting dependencies.
	ResolveImportTargets(input.ParsedFiles, input.AllFilePaths, goModulePath)

	// --- Phase 1: Persist all artifacts to PostgreSQL ---
	// Collect all chunks across files for cross-file batching.
	var allChunks []artifact.ChunkForEmbed
	for i, parsed := range input.ParsedFiles {
		fileContent := input.FileContents[parsed.FilePath]

		result, err := p.writer.WriteFile(ctx, input.ProjectID, input.SnapshotID, parsed, fileContent)
		if err != nil {
			return fmt.Errorf("indexing: file %d/%d %s: %w", i+1, len(input.ParsedFiles), parsed.FilePath, err)
		}

		allChunks = append(allChunks, result.Chunks...)

		// Update file progress (artifacts written, chunks pending embed).
		if (i+1)%500 == 0 || i+1 == len(input.ParsedFiles) {
			if err := p.queries.SetIndexingJobProgress(ctx, db.SetIndexingJobProgressParams{
				ID:             input.JobID,
				FilesProcessed: int32(i + 1),
				ChunksUpserted: 0,
				VectorsDeleted: 0,
			}); err != nil {
				log.Warn("indexing: failed to update progress",
					slog.String("error", err.Error()))
			}
		}
	}

	log.Info("indexing: artifacts persisted",
		slog.Int("files", len(input.ParsedFiles)),
		slog.Int("chunks", len(allChunks)))

	if len(allChunks) == 0 {
		return nil
	}

	// --- Phase 2: Embed all chunks in cross-file batches ---
	texts := make([]string, len(allChunks))
	for i, ch := range allChunks {
		texts[i] = ch.Content
	}

	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("indexing: embed chunks: %w", err)
	}
	if len(vectors) != len(texts) {
		return fmt.Errorf("indexing: embed: got %d vectors for %d texts", len(vectors), len(texts))
	}

	log.Info("indexing: embeddings generated", slog.Int("vectors", len(vectors)))

	// --- Phase 3: Upsert vectors to Qdrant in batches ---
	points := make([]vectorstore.Point, len(allChunks))
	for i, ch := range allChunks {
		points[i] = vectorstore.Point{
			ID:     formatUUID(ch.ChunkDBID),
			Vector: vectors[i],
			Payload: map[string]interface{}{
				"project_id":        formatUUID(input.ProjectID),
				"index_snapshot_id": formatUUID(input.SnapshotID),
				"file_path":         ch.FilePath,
				"language":          ch.Language,
				"symbol_name":       ch.SymbolName,
				"symbol_type":       ch.SymbolKind,
				"chunk_type":        ch.ChunkType,
				"chunk_hash":        ch.ChunkHash,
				"start_line":        ch.StartLine,
				"end_line":          ch.EndLine,
				"branch":            input.Branch,
				"git_commit":        input.GitCommit,
				"embedding_version": input.VersionLabel,
				"raw_text_ref":      fmt.Sprintf("db://code_chunks/%s", formatUUID(ch.ChunkDBID)),
			},
		}
	}

	for start := 0; start < len(points); start += vectorUpsertBatchSize {
		end := start + vectorUpsertBatchSize
		if end > len(points) {
			end = len(points)
		}
		if err := p.vectorDB.UpsertPoints(ctx, collection, points[start:end]); err != nil {
			return fmt.Errorf("indexing: upsert vectors batch %d-%d: %w", start, end, err)
		}
	}

	// Final progress update.
	if err := p.queries.SetIndexingJobProgress(ctx, db.SetIndexingJobProgressParams{
		ID:             input.JobID,
		FilesProcessed: int32(len(input.ParsedFiles)),
		ChunksUpserted: int32(len(allChunks)),
		VectorsDeleted: 0,
	}); err != nil {
		log.Warn("indexing: failed to update final progress",
			slog.String("error", err.Error()))
	}

	log.Info("indexing: vectors upserted",
		slog.Int("points", len(points)),
		slog.String("collection", collection))

	return nil
}

// ExtractGoModulePath extracts the module path from go.mod content.
// Returns "" if not found.
func ExtractGoModulePath(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "module" {
			continue
		}
		p := fields[1]
		// Strip inline comments (fields already split, but "module foo //comment"
		// produces ["module", "foo", "//comment"]).
		if strings.HasPrefix(p, "//") {
			continue
		}
		// Strip surrounding quotes (rare but spec-valid).
		p = strings.Trim(p, `"`)
		return p
	}
	return ""
}

// formatUUID renders a pgtype.UUID as a hex string.
func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
