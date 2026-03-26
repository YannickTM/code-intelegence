package incremental

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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/artifact"
	"myjungle/backend-worker/internal/embedding"
	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/gitclient"
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

// diffComputer computes git diff between two commits.
type diffComputer interface {
	DiffNameStatus(ctx context.Context, repoDir, baseCommit, targetCommit string) ([]gitclient.DiffEntry, error)
}

// commitIndexer indexes git commit history into the database.
type commitIndexer interface {
	IndexAll(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*commits.Result, error)
	IndexSince(ctx context.Context, projectID pgtype.UUID, repoDir, sinceCommit string) (*commits.Result, error)
}

// Handler implements workflow.Handler for the "incremental-index" workflow.
type Handler struct {
	repo          jobRepo
	workspace     workspacePreparer
	parser        fileParser
	queries       db.Querier
	activate      indexing.ActivateFunc
	writer        *artifact.Writer
	vectorDB      vectorstore.VectorStore
	differ        diffComputer
	commitIndexer commitIndexer
}

// NewHandler creates an incremental-index workflow handler.
func NewHandler(
	repo jobRepo,
	ws workspacePreparer,
	p fileParser,
	q db.Querier,
	activate indexing.ActivateFunc,
	w *artifact.Writer,
	v vectorstore.VectorStore,
	d diffComputer,
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
		differ:        d,
		commitIndexer: ci,
	}
}

// Handle executes the incremental-index workflow for a queued job.
func (h *Handler) Handle(ctx context.Context, task workflow.WorkflowTask) error {
	jobID, err := embedding.UUIDFromString(task.JobID)
	if err != nil {
		return fmt.Errorf("incremental: invalid job_id %q: %w", task.JobID, err)
	}

	log := slog.With(slog.String("job_id", task.JobID), slog.String("workflow", "incremental-index"))
	ctx = logger.WithLogger(ctx, log)

	// Load execution context (pre-claim — error lets queue retry).
	execCtx, err := h.repo.LoadExecutionContext(ctx, jobID)
	if err != nil {
		log.Error("failed to load execution context", slog.String("error", err.Error()))
		return fmt.Errorf("incremental: load context: %w", err)
	}

	// Acquire project-level advisory lock (pre-claim — error lets queue retry).
	projectUnlock, err := h.repo.TryProjectLock(ctx, execCtx.ProjectID)
	if err != nil {
		log.Warn("failed to acquire project lock", slog.String("error", err.Error()))
		return fmt.Errorf("incremental: project lock: %w", err)
	}
	defer projectUnlock(ctx)

	// Claim the job atomically (queued → running).
	if err := h.repo.ClaimJob(ctx, jobID, task.WorkerID); err != nil {
		if errors.Is(err, repository.ErrJobNotQueued) {
			log.Warn("job already claimed or finished, skipping stale retry",
				slog.String("error", err.Error()))
			return nil
		}
		log.Error("failed to claim job", slog.String("error", err.Error()))
		return fmt.Errorf("incremental: claim job: %w", err)
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

	// Attempt incremental base resolution.
	fallback, baseSnapshot, diff := h.resolveIncrementalBase(ctx, log, execCtx, embVersion, wsResult)

	if fallback {
		log.Info("falling back to full-index semantics")
		return h.runFullPipeline(ctx, log, jobID, execCtx, embVersion, versionLabel, wsResult)
	}

	return h.runIncrementalPipeline(ctx, log, jobID, execCtx, embVersion, versionLabel, wsResult, baseSnapshot, diff)
}

// resolveIncrementalBase attempts to find a valid incremental base.
// Returns (fallback=true, ...) if incremental is not possible.
func (h *Handler) resolveIncrementalBase(
	ctx context.Context,
	log *slog.Logger,
	execCtx *execution.Context,
	embVersion db.EmbeddingVersion,
	wsResult *workspace.Result,
) (fallback bool, baseSnapshot db.IndexSnapshot, diff []gitclient.DiffEntry) {
	// Load active snapshot for branch.
	baseSnapshot, err := h.queries.GetActiveSnapshotForBranch(ctx, db.GetActiveSnapshotForBranchParams{
		ProjectID: execCtx.ProjectID,
		Branch:    execCtx.Branch,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			log.Info("no active snapshot found, will fall back")
		} else {
			log.Warn("failed to load active snapshot, will fall back",
				slog.String("error", err.Error()))
		}
		return true, db.IndexSnapshot{}, nil
	}

	// Check embedding version compatibility.
	if baseSnapshot.EmbeddingVersionID != embVersion.ID {
		log.Info("embedding version mismatch, will fall back",
			slog.String("base_version", formatUUID(baseSnapshot.EmbeddingVersionID)),
			slog.String("current_version", formatUUID(embVersion.ID)))
		return true, db.IndexSnapshot{}, nil
	}

	// Compute diff.
	diff, err = h.differ.DiffNameStatus(ctx, wsResult.RepoDir, baseSnapshot.GitCommit, wsResult.CommitSHA)
	if err != nil {
		log.Warn("failed to compute diff, will fall back",
			slog.String("error", err.Error()))
		return true, db.IndexSnapshot{}, nil
	}

	log.Info("incremental base resolved",
		slog.String("base_commit", baseSnapshot.GitCommit),
		slog.String("target_commit", wsResult.CommitSHA),
		slog.Int("diff_entries", len(diff)))

	return false, baseSnapshot, diff
}

// runFullPipeline processes all source files (fallback behavior).
func (h *Handler) runFullPipeline(
	ctx context.Context,
	log *slog.Logger,
	jobID pgtype.UUID,
	execCtx *execution.Context,
	embVersion db.EmbeddingVersion,
	versionLabel string,
	wsResult *workspace.Result,
) error {
	// Index commit history (non-fatal).
	var commitResult *commits.Result
	if h.commitIndexer != nil {
		var commitErr error
		commitResult, commitErr = h.commitIndexer.IndexAll(ctx, execCtx.ProjectID, wsResult.RepoDir, 5000)
		if commitErr != nil {
			log.Warn("commit indexing failed (non-fatal)", slog.String("error", commitErr.Error()))
			commitResult = nil
		} else {
			log.Info("commit history indexed (full fallback)",
				slog.Int("commits", commitResult.CommitsIndexed),
				slog.Int("diffs", commitResult.DiffsIndexed))
		}
	}

	// Read all file contents from disk.
	fileContents, err := readSourceFiles(wsResult.RepoDir, wsResult.SourceFiles)
	if err != nil {
		h.failJob(ctx, log, jobID, "repo_access", err, "read_source_files")
		return nil
	}

	// Split files into parser-supported and non-parseable.
	parseableFiles, rawFiles := splitByParserSupport(wsResult.SourceFiles)

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

	// Ensure no chunk exceeds the embedding limit.
	splitOversizedChunks(parsedFiles, embedding.DefaultMaxInputChars)

	log.Info("files parsed (full)",
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
	// When commit indexing returns empty results, the commit may already
	// exist in the DB from a prior run — fall back to a direct lookup.
	headCommitDBID := commitHeadDBID(commitResult)
	if !headCommitDBID.Valid {
		dbCommit, lookupErr := h.queries.GetCommitByHash(ctx, db.GetCommitByHashParams{
			ProjectID:  execCtx.ProjectID,
			CommitHash: wsResult.CommitSHA,
		})
		if lookupErr != nil {
			if lookupErr != pgx.ErrNoRows {
				log.Warn("failed to look up commit by SHA (non-fatal)", slog.String("error", lookupErr.Error()))
			}
		} else {
			headCommitDBID = dbCommit.ID
		}
	}
	if headCommitDBID.Valid {
		if _, linkErr := h.queries.UpdateSnapshotCommitID(ctx, db.UpdateSnapshotCommitIDParams{
			ProjectID: execCtx.ProjectID,
			ID:        snapshot.ID,
			CommitID:  headCommitDBID,
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

	log.Info("incremental-index completed (full fallback)",
		slog.String("snapshot_id", formatUUID(snapshot.ID)),
		slog.String("commit", wsResult.CommitSHA))
	return nil
}

// runIncrementalPipeline processes only changed files and copies forward unchanged.
func (h *Handler) runIncrementalPipeline(
	ctx context.Context,
	log *slog.Logger,
	jobID pgtype.UUID,
	execCtx *execution.Context,
	embVersion db.EmbeddingVersion,
	versionLabel string,
	wsResult *workspace.Result,
	baseSnapshot db.IndexSnapshot,
	diff []gitclient.DiffEntry,
) error {
	// Detect Go module path for import resolution (always available,
	// even when go.mod is unchanged and not in FileContents).
	var goModulePath string
	if data, err := os.ReadFile(filepath.Join(wsResult.RepoDir, "go.mod")); err == nil {
		goModulePath = indexing.ExtractGoModulePath(string(data))
	}

	// Classify diff entries.
	changedPaths := make(map[string]bool) // added + modified
	deletedPaths := make(map[string]bool)
	for _, entry := range diff {
		switch entry.Status {
		case "A", "M":
			changedPaths[entry.Path] = true
		case "D":
			deletedPaths[entry.Path] = true
		}
	}

	// Build list of changed source files that exist on disk.
	var changedFiles []string
	for _, f := range wsResult.SourceFiles {
		if changedPaths[f] {
			changedFiles = append(changedFiles, f)
		}
	}

	// Load old snapshot file list to determine unchanged files.
	oldFiles, err := h.queries.ListSnapshotFiles(ctx, baseSnapshot.ID)
	if err != nil {
		h.failJob(ctx, log, jobID, "context_load", err, "list_old_files")
		return nil
	}

	unchangedPaths := make(map[string]bool)
	for _, f := range oldFiles {
		if !changedPaths[f.FilePath] && !deletedPaths[f.FilePath] {
			unchangedPaths[f.FilePath] = true
		}
	}

	log.Info("incremental scope",
		slog.Int("changed", len(changedFiles)),
		slog.Int("deleted", len(deletedPaths)),
		slog.Int("unchanged", len(unchangedPaths)))

	// Index new commits since base (non-fatal).
	var commitResult *commits.Result
	if h.commitIndexer != nil {
		var commitErr error
		commitResult, commitErr = h.commitIndexer.IndexSince(ctx, execCtx.ProjectID, wsResult.RepoDir, baseSnapshot.GitCommit)
		if commitErr != nil {
			log.Warn("commit indexing failed (non-fatal)", slog.String("error", commitErr.Error()))
			commitResult = nil
		} else {
			log.Info("commit history indexed (incremental)",
				slog.Int("commits", commitResult.CommitsIndexed),
				slog.Int("diffs", commitResult.DiffsIndexed))
		}
	}

	// Read and parse only changed files.
	var parsedFiles []parser.ParsedFileResult
	var fileContents map[string]string
	if len(changedFiles) > 0 {
		fileContents, err = readSourceFiles(wsResult.RepoDir, changedFiles)
		if err != nil {
			h.failJob(ctx, log, jobID, "repo_access", err, "read_changed_files")
			return nil
		}

		// Split changed files into parser-supported and non-parseable.
		parseableChanged, rawChanged := splitByParserSupport(changedFiles)

		fileInputs := buildFileInputs(parseableChanged, fileContents)

		parsedFiles, err = h.parser.ParseFilesBatched(
			ctx,
			formatUUID(execCtx.ProjectID),
			execCtx.Branch,
			wsResult.CommitSHA,
			fileInputs,
		)
		if err != nil {
			h.failJob(ctx, log, jobID, "parser", err, "parse_changed_files")
			return nil
		}

		// Build synthetic results for non-parseable changed files.
		rawResults := buildRawFileResults(rawChanged, fileContents)
		parsedFiles = append(parsedFiles, rawResults...)

		// Ensure no chunk exceeds the embedding limit.
		splitOversizedChunks(parsedFiles, embedding.DefaultMaxInputChars)

		log.Info("changed files parsed",
			slog.Int("parser", len(parsedFiles)-len(rawResults)),
			slog.Int("raw", len(rawResults)))
	} else {
		fileContents = make(map[string]string)
	}

	// Create new snapshot.
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

	// Run pipeline for changed files only (persist artifacts, embed, upsert vectors).
	embedder := embedding.NewOllamaClient(
		execCtx.Embedding.EndpointURL,
		execCtx.Embedding.Model,
		execCtx.Embedding.Dimensions,
		int(execCtx.Embedding.MaxTokens),
	)

	if len(parsedFiles) > 0 {
		// Build full file path list (changed + unchanged) so import
		// resolution can match targets that point to unchanged files.
		allPaths := make([]string, 0, len(changedFiles)+len(unchangedPaths))
		allPaths = append(allPaths, changedFiles...)
		for fp := range unchangedPaths {
			allPaths = append(allPaths, fp)
		}

		pipeline := indexing.NewPipeline(h.queries, h.activate, h.writer, embedder, h.vectorDB)
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
			AllFilePaths: allPaths,
			GoModulePath: goModulePath,
		}
		// Run without activation — we'll activate after copy-forward.
		if err := pipeline.RunWithoutActivation(ctx, pipelineInput); err != nil {
			category := categorizePipelineError(err)
			h.failJob(ctx, log, jobID, category, err, "pipeline_changed")
			return nil
		}
	}

	// Copy-forward unchanged files.
	if len(unchangedPaths) > 0 {
		cfResult, err := CopyForwardFiles(ctx, h.queries, execCtx.ProjectID, baseSnapshot.ID, snapshot.ID, unchangedPaths, oldFiles)
		if err != nil {
			h.failJob(ctx, log, jobID, "artifact_write", err, "copy_forward_files")
			return nil
		}

		log.Info("copy-forward complete",
			slog.Int("files", cfResult.FilesCopied),
			slog.Int("chunks", cfResult.ChunksCopied))

		// Copy-forward vectors for unchanged chunks.
		if cfResult.ChunksCopied > 0 {
			collection := vectorstore.CollectionName(execCtx.ProjectID, versionLabel)
			if err := CopyForwardVectors(ctx, h.vectorDB, collection, cfResult.ChunkIDMapping, formatUUID(snapshot.ID), wsResult.CommitSHA); err != nil {
				h.failJob(ctx, log, jobID, "vector_write", err, "copy_forward_vectors")
				return nil
			}
		}

		// Update progress counters to include both pipeline and copied artifacts.
		var pipelineChunks int32
		for _, pf := range parsedFiles {
			pipelineChunks += int32(len(pf.Chunks))
		}
		totalFiles := int32(len(parsedFiles)) + int32(cfResult.FilesCopied)
		totalChunks := pipelineChunks + int32(cfResult.ChunksCopied)
		if err := h.queries.SetIndexingJobProgress(ctx, db.SetIndexingJobProgressParams{
			ID:             jobID,
			FilesProcessed: totalFiles,
			ChunksUpserted: totalChunks,
			VectorsDeleted: 0,
		}); err != nil {
			log.Warn("failed to update progress", slog.String("error", err.Error()))
		}
	}

	// Activate snapshot (transactional in production via TxActivator).
	if err := h.activate(ctx, execCtx.ProjectID, execCtx.Branch, snapshot.ID); err != nil {
		h.failJob(ctx, log, jobID, "activation", err, "activate_snapshot")
		return nil
	}

	// Link snapshot to HEAD commit (non-fatal).
	// When IndexSince returns empty results (HEAD unchanged), the commit
	// already exists in the DB from a prior run — fall back to a direct lookup.
	headCommitDBID := commitHeadDBID(commitResult)
	if !headCommitDBID.Valid {
		dbCommit, lookupErr := h.queries.GetCommitByHash(ctx, db.GetCommitByHashParams{
			ProjectID:  execCtx.ProjectID,
			CommitHash: wsResult.CommitSHA,
		})
		if lookupErr != nil {
			if lookupErr != pgx.ErrNoRows {
				log.Warn("failed to look up commit by SHA (non-fatal)", slog.String("error", lookupErr.Error()))
			}
		} else {
			headCommitDBID = dbCommit.ID
		}
	}
	if headCommitDBID.Valid {
		if _, linkErr := h.queries.UpdateSnapshotCommitID(ctx, db.UpdateSnapshotCommitIDParams{
			ProjectID: execCtx.ProjectID,
			ID:        snapshot.ID,
			CommitID:  headCommitDBID,
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

	log.Info("incremental-index completed",
		slog.String("snapshot_id", formatUUID(snapshot.ID)),
		slog.String("commit", wsResult.CommitSHA),
		slog.Int("changed", len(changedFiles)),
		slog.Int("unchanged", len(unchangedPaths)),
		slog.Int("deleted", len(deletedPaths)))

	return nil
}

// failJob marks the job as failed with structured error details.
// It uses a detached context so the DB write is not skipped when the
// task context is canceled (e.g. asynq timeout or shutdown).
func (h *Handler) failJob(_ context.Context, log *slog.Logger, jobID pgtype.UUID, category string, origErr error, step string) {
	log.Error("incremental-index failed",
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

// commitHeadDBID extracts HeadCommitDBID from a commit-index result,
// returning an invalid pgtype.UUID when the result is nil or has no head.
func commitHeadDBID(r *commits.Result) pgtype.UUID {
	if r != nil && r.HeadCommitDBID.Valid {
		return r.HeadCommitDBID
	}
	return pgtype.UUID{}
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
// that have no embedded parser support. Each file is split into one or more
// "raw" chunks on line boundaries so the full content remains available for
// embedding and retrieval.
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
				lineCount++
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
// that exceed the embedding model's context window.
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
