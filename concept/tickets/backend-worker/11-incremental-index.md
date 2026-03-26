# 11 — Incremental-Index Workflow

## Status
Done

## Goal
Implement the `incremental-index` workflow that re-indexes only files that changed since the last active snapshot, copies forward unchanged artifacts and vectors, and falls back to full-index semantics when incremental is not possible.

## Depends On
04-workspace, 09-pipeline, 10-full-index

## Scope

### Handler (`internal/workflow/incremental/handler.go`)

The `Handler` struct implements `workflow.Handler` and is registered under the `incremental-index` task type. The pre-claim phase (load context, advisory lock, claim job) is identical to full-index. After claiming, the handler resolves the incremental base and branches into one of two code paths.

### Incremental Base Resolution (`resolveIncrementalBase`)

Determines whether incremental indexing is possible:

1. **Load active snapshot:** `GetActiveSnapshotForBranch(project_id, branch)` -- if no active snapshot exists, fall back
2. **Check embedding version compatibility:** if the base snapshot's `embedding_version_id` differs from the current version, fall back (vectors would be incompatible)
3. **Compute git diff:** `DiffNameStatus(repoDir, baseCommit, targetCommit)` returns `[]DiffEntry` with status (A/M/D) and path

Fallback triggers: no prior snapshot, embedding version mismatch, diff computation failure.

### Incremental Pipeline (`runIncrementalPipeline`)

When incremental is possible:

1. **Classify diff entries:** build `changedPaths` (A+M) and `deletedPaths` (D) sets
2. **Build changed file list:** intersect `changedPaths` with `wsResult.SourceFiles` (files present on disk)
3. **Load old snapshot files:** `ListSnapshotFiles(baseSnapshotID)` to determine `unchangedPaths` (not changed, not deleted)
4. **Index new commits** (non-fatal): `commitIndexer.IndexSince(projectID, repoDir, baseSnapshot.GitCommit)` indexes only commits since the base snapshot
5. **Read and parse changed files only:** read from disk, split by parser support, parse via embedded engine, build raw results for non-parseable files, split oversized chunks
6. **Create new snapshot:** `CreateSnapshot` with `building` status; clean stale artifacts
7. **Run pipeline without activation:** `Pipeline.RunWithoutActivation()` persists artifacts, generates embeddings, and upserts vectors for changed files only. Import resolution receives `AllFilePaths` (changed + unchanged) so targets pointing to unchanged files are still resolved
8. **Copy-forward unchanged files:** `CopyForwardFiles()` duplicates PostgreSQL artifact rows (files, symbols, chunks, dependencies, exports, references, JSX usages, network calls) from the base snapshot to the new snapshot for all unchanged paths. Returns a `ChunkIDMapping` (old chunk ID -> new chunk ID)
9. **Copy-forward vectors:** `CopyForwardVectors()` reads existing Qdrant points by old chunk IDs, creates new points with new chunk IDs and updated `index_snapshot_id` and `git_commit` payload fields
10. **Update progress:** combined count of pipeline-processed and copy-forwarded files/chunks
11. **Activate snapshot:** `activate(projectID, branch, snapshotID)` -- separate call because the pipeline ran without activation
12. **Link snapshot to commit** (non-fatal): looks up HEAD commit DB ID from commit indexer result, with fallback to `GetCommitByHash` for already-indexed commits
13. **Mark job completed**

### Full Fallback (`runFullPipeline`)

When incremental is not possible, the handler executes full-index semantics within the incremental handler:

- Indexes all commit history (`IndexAll` with 5000 limit)
- Reads, parses, and persists all source files
- Runs the standard pipeline with activation
- Links snapshot to HEAD commit with the same fallback lookup pattern

### Copy-Forward Mechanism

`CopyForwardFiles` creates new artifact rows in the new snapshot by reading from the base snapshot:
- For each unchanged file: copies the file row, all symbols (re-linking parent IDs), all chunks (building old-to-new ID mapping), all dependencies, exports, references, JSX usages, and network calls
- Preserves all metadata, flags, and v2 fields from the original rows
- Returns `CopyForwardResult` with counts and the chunk ID mapping for vector copy-forward

`CopyForwardVectors` reads existing Qdrant points by their old chunk IDs (in batches of 100), creates new points with updated IDs and metadata, and upserts them to the same collection.

### Deleted File Handling

Files in `deletedPaths` are simply not carried forward -- they have no entry in the new snapshot's artifacts, and their vectors are not copied. Old vectors remain in Qdrant but are scoped to the previous `index_snapshot_id` and will not appear in queries filtered by the new snapshot.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/workflow/incremental/handler.go` | Incremental workflow handler, base resolution, incremental and full code paths |
| `internal/workflow/incremental/copy_forward.go` | `CopyForwardFiles`, `CopyForwardVectors`, chunk ID mapping |
| `internal/workflow/incremental/handler_test.go` | Unit tests with mock dependencies |
| `internal/gitclient/gitclient.go` | `DiffNameStatus` for computing file-level changes between commits |

## Acceptance Criteria
- [x] Handler registered on asynq ServeMux under `incremental-index` task type
- [x] Incremental base resolved from active snapshot for `(project_id, branch)`
- [x] Embedding version mismatch triggers fallback to full-index
- [x] Missing prior snapshot triggers fallback to full-index
- [x] Diff computation failure triggers fallback to full-index
- [x] Only changed (added/modified) files are read, parsed, and embedded
- [x] Unchanged file artifacts copied forward from base snapshot to new snapshot
- [x] Unchanged vectors copied forward in Qdrant with updated snapshot metadata
- [x] Deleted files excluded from new snapshot (not copied forward)
- [x] Import resolution receives full file path list (changed + unchanged)
- [x] New commits indexed incrementally since base snapshot's commit (non-fatal)
- [x] Full fallback path exercises complete indexing semantics
- [x] Snapshot activated after both pipeline and copy-forward complete
- [x] Snapshot linked to HEAD commit with fallback lookup for existing commits
- [x] Progress counters include both pipeline-processed and copy-forwarded counts
