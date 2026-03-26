# 10 — Full-Index Workflow

## Status
Done

## Goal
Implement end-to-end orchestration for the `full-index` workflow, connecting all previously built components into a single handler that takes a queued job through to an activated, searchable index snapshot.

## Depends On
02-queue-dispatch, 03-job-lifecycle, 04-workspace, 05-parser-engine, 09-pipeline

## Scope

### Handler (`internal/workflow/fullindex/handler.go`)

The `Handler` struct implements `workflow.Handler` and is registered on the asynq ServeMux under the `full-index` task type. It receives a `WorkflowTask` (job_id, workflow name) and executes the following steps in order:

**Pre-claim phase** (errors returned to asynq for retry):

1. **Load execution context:** `LoadExecutionContext(jobID)` fetches job, project, SSH key, and embedding config from PostgreSQL
2. **Acquire advisory lock:** `TryProjectLock(projectID)` acquires a session-level PostgreSQL advisory lock to prevent concurrent indexing of the same project; returns `ErrProjectLocked` if already held
3. **Claim job:** `ClaimJob(jobID)` atomically transitions `queued -> running`; stale retries (job already claimed) are detected and silently skipped

**Post-claim phase** (errors mark the job failed, return nil to prevent queue retry):

4. **Resolve embedding version:** derives deterministic `version_label` and finds or creates the `embedding_versions` row
5. **Prepare workspace:** clone/fetch repository, create per-job worktree, enumerate tracked files, record HEAD commit SHA
6. **Index commit history** (non-fatal): `commitIndexer.IndexAll(projectID, repoDir, 5000)` indexes up to 5000 commits with file diffs and patches; failure is logged as a warning but does not fail the job
7. **Read source files:** reads all tracked files from disk, skipping binary files (invalid UTF-8 or null bytes)
8. **Split files:** separates into parser-supported (by extension) and raw (non-parseable) files
9. **Parse files:** sends parser-supported files through the embedded tree-sitter engine via `ParseFilesBatched`; builds synthetic `ParsedFileResult` entries for raw files with full-file chunks
10. **Split oversized chunks:** any chunk exceeding `DefaultMaxInputChars` (7500) is split on line boundaries, preserving the original chunk's metadata (type, symbol info, semantic role)
11. **Create snapshot:** `CreateSnapshot` with `status = 'building'`, branch, embedding version, and commit SHA
12. **Clean stale artifacts:** `DeleteSnapshotArtifacts` removes any leftover rows from a prior failed attempt for the same snapshot
13. **Run pipeline:** constructs per-job `OllamaClient` and `Pipeline`, then calls `Pipeline.Run()` which executes artifacts -> embeddings -> vectors -> activation
14. **Link snapshot to commit** (non-fatal): `UpdateSnapshotCommitID` associates the activated snapshot with the HEAD commit's database ID
15. **Mark job completed:** `SetJobCompleted` with detached context (10s timeout) to ensure the transition is recorded even if the task context was canceled

### Error Handling

Post-claim errors are categorized via `categorizePipelineError`:
- `vector_write`: collection creation or vector upsert failures
- `embedding`: Ollama embedding errors
- `activation`: snapshot activation failures
- `artifact_write`: default for PostgreSQL write errors

Each failure is persisted as structured JSONB in the job's `error_details` column with `category`, `message`, and `step` fields. The `failJob` method uses a detached context (10s timeout) so the failure record is written even during asynq shutdown.

### Raw File Handling

Files without parser support get synthetic `ParsedFileResult` entries with:
- Language inferred from file extension
- SHA-256 file hash
- Line count and byte size
- Full-file content split into chunks at line boundaries, each no larger than `DefaultMaxInputChars`

### Progress Reporting

`files_processed / files_total` updated via `SetIndexingJobProgress` during the pipeline's artifact persistence phase (every 500 files and at completion). Chunk and vector counts updated in the final progress call.

### Handler Dependencies (injected via constructor)

| Dependency | Interface | Implementation |
|---|---|---|
| `jobRepo` | Load context, claim, create snapshot, set completed/failed, project lock | `repository.JobRepository` |
| `workspacePreparer` | Clone/fetch, worktree, file enumeration | `workspace.Preparer` |
| `fileParser` | `ParseFilesBatched` | `parser/engine.Engine` |
| `queries` | `db.Querier` | sqlc-generated queries |
| `activate` | `ActivateFunc` | `indexing.TxActivator(pool)` |
| `writer` | `*artifact.Writer` | PostgreSQL artifact writer |
| `vectorDB` | `vectorstore.VectorStore` | `vectorstore.QdrantClient` |
| `commitIndexer` | `IndexAll` | `commits.Indexer` |

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/workflow/fullindex/handler.go` | Full-index workflow handler, orchestration, error categorization |
| `internal/workflow/fullindex/handler_test.go` | Unit tests with mock dependencies |
| `internal/indexing/pipeline.go` | Storage pipeline called by handler |
| `internal/workspace/` | Workspace preparation called by handler |
| `internal/app/app.go` | Bootstrap wiring: constructs handler with all dependencies |

## Acceptance Criteria
- [x] Handler registered on asynq ServeMux under `full-index` task type
- [x] Execution context loaded from PostgreSQL (job, project, SSH key, embedding config)
- [x] Advisory lock acquired before claiming job; concurrent indexing of same project prevented
- [x] Stale retries detected (job already claimed) and silently skipped
- [x] Workspace prepared: clone/fetch, worktree, file enumeration, commit SHA recorded
- [x] Commit history indexed as non-fatal sub-step (up to 5000 commits)
- [x] All tracked source files read from disk; binary files skipped
- [x] Parser-supported files parsed via embedded engine; raw files get synthetic results
- [x] Oversized chunks split on line boundaries before embedding
- [x] Snapshot created with `building` status; stale artifacts cleaned before pipeline
- [x] Pipeline executes all three phases: artifacts, embeddings, vectors
- [x] Snapshot activated only after all writes succeed
- [x] Snapshot linked to HEAD commit on success (non-fatal)
- [x] Job marked completed with detached context
- [x] Post-claim errors categorized and persisted as structured JSONB
- [x] Job failure recorded with detached context even during shutdown
