// Package fullindex implements the full-index workflow handler that orchestrates
// end-to-end indexing: load context, prepare workspace, parse files, embed
// chunks, persist artifacts, and activate the snapshot.
package fullindex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/artifact"
	"myjungle/backend-worker/internal/embedding"
	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/indexing"
	"myjungle/backend-worker/internal/logger"
	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
	"myjungle/backend-worker/internal/repository"
	"myjungle/backend-worker/internal/vectorstore"
	"myjungle/backend-worker/internal/workflow"
	"myjungle/backend-worker/internal/workflow/commits"
	"myjungle/backend-worker/internal/workspace"
	db "myjungle/datastore/postgres/sqlc"
)

// jobRepo is the subset of repository.JobRepository used by the handler.
type jobRepo interface {
	LoadExecutionContext(ctx context.Context, jobID pgtype.UUID) (*execution.Context, error)
	ClaimJob(ctx context.Context, jobID pgtype.UUID, workerID string) error
	CreateSnapshot(ctx context.Context, params db.CreateIndexSnapshotParams) (db.IndexSnapshot, error)
	SetJobCompleted(ctx context.Context, jobID pgtype.UUID) (bool, error)
	SetJobFailed(ctx context.Context, jobID pgtype.UUID, errorDetails []byte) (bool, error)
	TryProjectLock(ctx context.Context, projectID pgtype.UUID) (unlock func(context.Context), err error)
}

// workspacePreparer prepares a git workspace for indexing.
type workspacePreparer interface {
	Prepare(ctx context.Context, execCtx *execution.Context) (*workspace.Result, func(), error)
}

// fileParser parses source files with the embedded parser engine.
type fileParser interface {
	ParseFilesBatched(ctx context.Context, projectID, branch, commitSHA string, files []parser.FileInput) ([]parser.ParsedFileResult, error)
}

// commitIndexer indexes git commit history into the database.
type commitIndexer interface {
	IndexAll(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error)
}

// Handler implements workflow.Handler for the "full-index" workflow.
type Handler struct {
	repo          jobRepo
	workspace     workspacePreparer
	parser        fileParser
	queries       db.Querier
	activate      indexing.ActivateFunc
	writer        *artifact.Writer
	vectorDB      vectorstore.VectorStore
	commitIndexer commitIndexer
}

// NewHandler creates a full-index workflow handler.
func NewHandler(
	repo jobRepo,
	ws workspacePreparer,
	p fileParser,
	q db.Querier,
	activate indexing.ActivateFunc,
	w *artifact.Writer,
	v vectorstore.VectorStore,
	ci commitIndexer,
) *Handler {
	return &Handler{
		repo:          repo,
		workspace:     ws,
		parser:        p,
		queries:       q,
		activate:      activate,
		writer:        w,
		vectorDB:      v,
		commitIndexer: ci,
	}
}

// Handle executes the full-index workflow for a queued job.
func (h *Handler) Handle(ctx context.Context, task workflow.WorkflowTask) error {
	jobID, err := embedding.UUIDFromString(task.JobID)
	if err != nil {
		return fmt.Errorf("fullindex: invalid job_id %q: %w", task.JobID, err)
	}

	log := slog.With(slog.String("job_id", task.JobID), slog.String("workflow", "full-index"))
	ctx = logger.WithLogger(ctx, log)

	// Load execution context (pre-claim — error lets queue retry).
	execCtx, err := h.repo.LoadExecutionContext(ctx, jobID)
	if err != nil {
		log.Error("failed to load execution context", slog.String("error", err.Error()))
		return fmt.Errorf("fullindex: load context: %w", err)
	}

	// Acquire project-level advisory lock (pre-claim — error lets queue retry).
	projectUnlock, err := h.repo.TryProjectLock(ctx, execCtx.ProjectID)
	if err != nil {
		log.Warn("failed to acquire project lock", slog.String("error", err.Error()))
		return fmt.Errorf("fullindex: project lock: %w", err)
	}
	defer projectUnlock(ctx)

	// Claim the job atomically (queued → running). Pre-claim — error lets queue retry.
	if err := h.repo.ClaimJob(ctx, jobID, task.WorkerID); err != nil {
		if errors.Is(err, repository.ErrJobNotQueued) {
			// Job already claimed/completed/failed — stale retry, nothing to do.
			log.Warn("job already claimed or finished, skipping stale retry",
				slog.String("error", err.Error()))
			return nil
		}
		log.Error("failed to claim job", slog.String("error", err.Error()))
		return fmt.Errorf("fullindex: claim job: %w", err)
	}

	log.Info("job claimed",
		slog.String("project_id", formatUUID(execCtx.ProjectID)),
		slog.String("branch", execCtx.Branch))

	// From here: any error marks the job failed (return nil to prevent queue retry).

	// Resolve embedding version.
	embVersion, err := embedding.ResolveVersion(ctx, h.queries, execCtx.Embedding)
	if err != nil {
		h.failJob(ctx, log, jobID, "context_load", err, "resolve_embedding_version")
		return nil
	}
	versionLabel := embedding.VersionLabel(execCtx.Embedding.Provider, execCtx.Embedding.Model, execCtx.Embedding.Dimensions)

	// Prepare workspace (clone/fetch, worktree, file list).
	wsResult, cleanup, err := h.workspace.Prepare(ctx, execCtx)
	if err != nil {
		h.failJob(ctx, log, jobID, "repo_access", err, "prepare_workspace")
		return nil
	}
	defer cleanup()

	log.Info("workspace ready",
		slog.String("commit", wsResult.CommitSHA),
		slog.Int("source_files", len(wsResult.SourceFiles)))

	// Index commit history (non-fatal).
	var commitResult *commits.Result
	if h.commitIndexer != nil {
		var commitErr error
		commitResult, commitErr = h.commitIndexer.IndexAll(ctx, execCtx.ProjectID, wsResult.RepoDir, 5000)
		if commitErr != nil {
			log.Warn("commit indexing failed (non-fatal)", slog.String("error", commitErr.Error()))
			commitResult = nil
		} else {
			log.Info("commit history indexed",
				slog.Int("commits", commitResult.CommitsIndexed),
				slog.Int("diffs", commitResult.DiffsIndexed))
		}
	}

	// Read file contents from disk.
	log.Debug("reading source files", slog.Int("files", len(wsResult.SourceFiles)))
	fileContents, err := readSourceFiles(wsResult.RepoDir, wsResult.SourceFiles)
	if err != nil {
		h.failJob(ctx, log, jobID, "repo_access", err, "read_source_files")
		return nil
	}

	// Split files into parser-supported and non-parseable.
	parseableFiles, rawFiles := splitByParserSupport(wsResult.SourceFiles)
	log.Debug("files split by parser support",
		slog.Int("parseable", len(parseableFiles)),
		slog.Int("raw", len(rawFiles)),
		slog.Int("with_content", len(fileContents)))

	// Build parser inputs with language detection.
	fileInputs := buildFileInputs(parseableFiles, fileContents)

	// Parse supported files with the embedded parser engine.
	parsedFiles, err := h.parser.ParseFilesBatched(
		ctx,
		formatUUID(execCtx.ProjectID),
		execCtx.Branch,
		wsResult.CommitSHA,
		fileInputs,
	)
	if err != nil {
		h.failJob(ctx, log, jobID, "parser", err, "parse_files")
		return nil
	}

	// Build synthetic results for non-parseable files.
	rawResults := buildRawFileResults(rawFiles, fileContents)
	parsedFiles = append(parsedFiles, rawResults...)

	// Ensure no chunk exceeds the embedding limit. The parser engine
	// produces semantic chunks (functions, classes) that may be larger than
	// the embedding model's context window. Split those on line boundaries
	// so we don't silently truncate at embed time.
	splitOversizedChunks(parsedFiles, embedding.DefaultMaxInputChars)

	log.Info("files parsed",
		slog.Int("parser", len(parsedFiles)-len(rawResults)),
		slog.Int("raw", len(rawResults)))

	// Create snapshot.
	snapshot, err := h.repo.CreateSnapshot(ctx, db.CreateIndexSnapshotParams{
		ProjectID:          execCtx.ProjectID,
		Branch:             execCtx.Branch,
		EmbeddingVersionID: embVersion.ID,
		GitCommit:          wsResult.CommitSHA,
	})
	if err != nil {
		h.failJob(ctx, log, jobID, "artifact_write", err, "create_snapshot")
		return nil
	}

	// Clean up any stale artifacts from a prior failed attempt for this snapshot.
	if err := h.queries.DeleteSnapshotArtifacts(ctx, snapshot.ID); err != nil {
		h.failJob(ctx, log, jobID, "artifact_write", err, "cleanup_stale_artifacts")
		return nil
	}

	// Build per-job embedder and pipeline.
	embedder := embedding.NewOllamaClient(
		execCtx.Embedding.EndpointURL,
		execCtx.Embedding.Model,
		execCtx.Embedding.Dimensions,
		int(execCtx.Embedding.MaxTokens),
	)
	pipeline := indexing.NewPipeline(h.queries, h.activate, h.writer, embedder, h.vectorDB)

	// Detect Go module path for import resolution.
	var goModulePath string
	if data, err := os.ReadFile(filepath.Join(wsResult.RepoDir, "go.mod")); err == nil {
		goModulePath = indexing.ExtractGoModulePath(string(data))
	}

	// Run the storage pipeline (artifacts, embeddings, vectors, activation).
	pipelineInput := indexing.PipelineInput{
		ProjectID:    execCtx.ProjectID,
		SnapshotID:   snapshot.ID,
		JobID:        jobID,
		Branch:       execCtx.Branch,
		GitCommit:    wsResult.CommitSHA,
		VersionLabel: versionLabel,
		Dimensions:   execCtx.Embedding.Dimensions,
		ParsedFiles:  parsedFiles,
		FileContents: fileContents,
		GoModulePath: goModulePath,
	}
	if err := pipeline.Run(ctx, pipelineInput); err != nil {
		category := categorizePipelineError(err)
		h.failJob(ctx, log, jobID, category, err, "pipeline")
		return nil
	}

	// Link snapshot to HEAD commit (non-fatal).
	if commitResult != nil && commitResult.HeadCommitDBID.Valid {
		if _, linkErr := h.queries.UpdateSnapshotCommitID(ctx, db.UpdateSnapshotCommitIDParams{
			ProjectID: execCtx.ProjectID,
			ID:        snapshot.ID,
			CommitID:  commitResult.HeadCommitDBID,
		}); linkErr != nil {
			log.Warn("failed to link snapshot to commit (non-fatal)", slog.String("error", linkErr.Error()))
		}
	}

	// Mark job completed (guarded: only from running).
	// Use a detached context so the transition is not skipped if the
	// task context was canceled (e.g. asynq timeout or shutdown).
	completeCtx, completeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer completeCancel()
	transitioned, err := h.repo.SetJobCompleted(completeCtx, jobID)
	if err != nil {
		log.Warn("failed to mark job completed (work is done)",
			slog.String("error", err.Error()))
	} else if !transitioned {
		log.Warn("job completion no-op (job no longer running, possible stale worker)")
	}

	log.Info("full-index completed",
		slog.String("snapshot_id", formatUUID(snapshot.ID)),
		slog.String("commit", wsResult.CommitSHA))

	return nil
}

// failJob marks the job as failed with structured error details.
// It uses a detached context so the DB write is not skipped when the
// task context is canceled (e.g. asynq timeout or shutdown).
func (h *Handler) failJob(_ context.Context, log *slog.Logger, jobID pgtype.UUID, category string, origErr error, step string) {
	log.Error("full-index failed",
		slog.String("category", category),
		slog.String("step", step),
		slog.String("error", origErr.Error()))

	details := marshalErrorDetails([]ErrorDetail{{
		Category: category,
		Message:  origErr.Error(),
		Step:     step,
	}})
	failCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transitioned, err := h.repo.SetJobFailed(failCtx, jobID, details)
	if err != nil {
		log.Error("failed to mark job as failed",
			slog.String("error", err.Error()))
	} else if !transitioned {
		log.Warn("job failure no-op (job no longer running, possible stale worker)")
	}
}

// ErrorDetail is a structured error entry for the job's error_details JSONB column.
type ErrorDetail struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Step     string `json:"step"`
}

// readSourceFiles reads file contents from disk into a map keyed by relative path.
// Binary files are skipped so downstream code can safely store content in
// PostgreSQL TEXT columns and build meaningful embeddings. A file is considered
// binary if it contains invalid UTF-8 or null bytes (PostgreSQL rejects 0x00
// in TEXT columns even though Go's utf8.Valid considers it valid UTF-8).
func readSourceFiles(repoDir string, files []string) (map[string]string, error) {
	contents := make(map[string]string, len(files))
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(repoDir, f))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
			continue // Binary file — skip content; still gets a file record.
		}
		contents[f] = string(data)
	}
	return contents, nil
}

// splitByParserSupport splits files into those with embedded parser support
// and those that will be indexed as raw content.
func splitByParserSupport(files []string) (parseable, raw []string) {
	for _, f := range files {
		if workspace.HasParserSupport(filepath.Ext(f)) {
			parseable = append(parseable, f)
		} else {
			raw = append(raw, f)
		}
	}
	return
}

// buildFileInputs creates parser FileInput entries with language detection.
func buildFileInputs(files []string, contents map[string]string) []parser.FileInput {
	inputs := make([]parser.FileInput, 0, len(files))
	for _, f := range files {
		lang := languageForExt(filepath.Ext(f))
		if lang == "" {
			continue
		}
		inputs = append(inputs, parser.FileInput{
			FilePath: f,
			Content:  contents[f],
			Language: lang,
		})
	}
	return inputs
}

// buildRawFileResults creates synthetic ParsedFileResult entries for files
// that have no embedded parser support. Large files are split into multiple
// chunks on line boundaries, each ≤ embedding.DefaultMaxInputChars, so
// that the full content is preserved for embedding and retrieval.
func buildRawFileResults(files []string, contents map[string]string) []parser.ParsedFileResult {
	results := make([]parser.ParsedFileResult, 0, len(files))
	for _, f := range files {
		content := contents[f]
		h := sha256.Sum256([]byte(content))
		fileHash := hex.EncodeToString(h[:])

		// Use registry language ID when available, fall back to raw extension.
		lang := strings.TrimPrefix(filepath.Ext(f), ".")
		if detected, err := registry.DetectLanguage(f); err == nil {
			lang = detected
		}

		result := parser.ParsedFileResult{
			FilePath:  f,
			Language:  lang,
			FileHash:  fileHash,
			SizeBytes: int64(len(content)),
		}

		// Skip creating a chunk for empty files: lineCount would be 0,
		// producing an invalid StartLine=1/EndLine=0 range, and an
		// empty-content chunk provides no embedding value.
		if len(content) > 0 {
			lineCount := int32(strings.Count(content, "\n"))
			if content[len(content)-1] != '\n' {
				lineCount++ // Count last line if no trailing newline.
			}
			result.LineCount = lineCount
			result.Chunks = splitRawChunks(content, lineCount)
		}

		results = append(results, result)
	}
	return results
}

// splitRawChunks splits raw file content into line-boundary chunks of at most
// embedding.DefaultMaxInputChars bytes each. This preserves the full file
// content for embedding instead of silently truncating large files.
func splitRawChunks(content string, totalLines int32) []parser.Chunk {
	maxBytes := embedding.DefaultMaxInputChars

	// Fast path: content fits in a single chunk.
	if len(content) <= maxBytes {
		ch := sha256.Sum256([]byte(content))
		return []parser.Chunk{{
			ChunkType: "raw",
			ChunkHash: hex.EncodeToString(ch[:]),
			Content:   content,
			StartLine: 1,
			EndLine:   totalLines,
		}}
	}

	var chunks []parser.Chunk
	remaining := content
	var currentLine int32 = 1

	for len(remaining) > 0 {
		end := len(remaining)
		if end > maxBytes {
			// Find the last newline at or before the maxBytes boundary
			// so we split on a line boundary.
			cut := strings.LastIndex(remaining[:maxBytes], "\n")
			if cut > 0 {
				end = cut + 1 // include the newline in this chunk
			} else {
				// No newline found within the limit — very long single line.
				// Fall back to maxBytes with UTF-8 safety.
				end = maxBytes
				for end > 0 && end < len(remaining) && remaining[end-1] >= 0x80 {
					if utf8.RuneStart(remaining[end]) {
						break
					}
					end--
				}
			}
		}

		chunk := remaining[:end]
		remaining = remaining[end:]

		// Count lines in this chunk.
		newlines := int32(strings.Count(chunk, "\n"))
		endLine := currentLine + newlines
		if newlines > 0 || len(remaining) == 0 {
			// If chunk ends with newline, endLine is the last full line.
			// If it's the final chunk without trailing newline, count the partial line.
			if len(remaining) == 0 && len(chunk) > 0 && chunk[len(chunk)-1] != '\n' {
				// Last chunk doesn't end with newline — the partial line is counted.
			} else if newlines > 0 {
				endLine = currentLine + newlines - 1
			}
		}

		ch := sha256.Sum256([]byte(chunk))
		chunks = append(chunks, parser.Chunk{
			ChunkType: "raw",
			ChunkHash: hex.EncodeToString(ch[:]),
			Content:   chunk,
			StartLine: currentLine,
			EndLine:   endLine,
		})

		currentLine = endLine + 1
	}
	return chunks
}

// splitOversizedChunks walks all parsed files and splits any chunk whose
// content exceeds maxBytes into smaller sub-chunks on line boundaries.
// This handles parser-produced semantic chunks (large functions, classes)
// that exceed the embedding model's context window. The original chunk's
// metadata (type, symbol info, etc.) is preserved on all sub-chunks.
func splitOversizedChunks(results []parser.ParsedFileResult, maxBytes int) {
	for i := range results {
		var out []parser.Chunk
		changed := false
		for _, c := range results[i].Chunks {
			if len(c.Content) <= maxBytes {
				out = append(out, c)
				continue
			}
			changed = true
			subs := splitChunkContent(c, maxBytes)
			out = append(out, subs...)
		}
		if changed {
			results[i].Chunks = out
		}
	}
}

// splitChunkContent splits a single oversized chunk into sub-chunks on
// line boundaries, preserving the original chunk's metadata on each piece.
func splitChunkContent(orig parser.Chunk, maxBytes int) []parser.Chunk {
	var chunks []parser.Chunk
	remaining := orig.Content
	currentLine := orig.StartLine

	for len(remaining) > 0 {
		end := len(remaining)
		if end > maxBytes {
			cut := strings.LastIndex(remaining[:maxBytes], "\n")
			if cut > 0 {
				end = cut + 1
			} else {
				end = maxBytes
				for end > 0 && end < len(remaining) && remaining[end-1] >= 0x80 {
					if utf8.RuneStart(remaining[end]) {
						break
					}
					end--
				}
			}
		}

		chunk := remaining[:end]
		remaining = remaining[end:]

		newlines := int32(strings.Count(chunk, "\n"))
		endLine := currentLine + newlines
		if len(remaining) == 0 && len(chunk) > 0 && chunk[len(chunk)-1] != '\n' {
			// Last sub-chunk without trailing newline — partial line counted.
		} else if newlines > 0 {
			endLine = currentLine + newlines - 1
		}

		ch := sha256.Sum256([]byte(chunk))
		chunks = append(chunks, parser.Chunk{
			ChunkType:          orig.ChunkType,
			ChunkHash:          hex.EncodeToString(ch[:]),
			Content:            chunk,
			StartLine:          currentLine,
			EndLine:            endLine,
			SymbolID:           orig.SymbolID,
			OwnerQualifiedName: orig.OwnerQualifiedName,
			OwnerKind:          orig.OwnerKind,
			IsExportedContext:  orig.IsExportedContext,
			SemanticRole:       orig.SemanticRole,
		})

		currentLine = endLine + 1
	}
	return chunks
}

// languageForExt maps file extensions to parser language identifiers
// using the language registry as the single source of truth.
func languageForExt(ext string) string {
	langID, ok := registry.GetLanguageByExtension(ext)
	if !ok {
		return ""
	}
	return langID
}

// categorizePipelineError inspects the pipeline error message to determine
// the failure category for structured error reporting.
func categorizePipelineError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "ensure collection"):
		return "vector_write"
	case strings.Contains(msg, "upsert vectors"):
		return "vector_write"
	case strings.Contains(msg, "embed"):
		return "embedding"
	case strings.Contains(msg, "activate snapshot"):
		return "activation"
	default:
		return "artifact_write"
	}
}

// marshalErrorDetails serializes error details to JSON for the job's error_details column.
func marshalErrorDetails(details []ErrorDetail) []byte {
	b, err := json.Marshal(details)
	if err != nil {
		return []byte(`[{"category":"internal","message":"failed to marshal error details"}]`)
	}
	return b
}

// formatUUID renders a pgtype.UUID as a hex string with dashes.
func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return "<nil>"
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
