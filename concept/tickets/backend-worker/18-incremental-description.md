# 18 — Incremental Description Workflow

## Status
Pending

## Goal
Implement the `describe-incremental` scope within the existing description handler: content_hash-based change detection, dependency-driven reprocessing, and copy-forward of unchanged descriptions to new snapshots.

## Depends On
17-description-handler

## Scope

### Incremental File Selection (`internal/workflow/describe/incremental.go`)

The `describe-incremental` workflow reuses the handler from ticket 17 but adds a file selection strategy that identifies which files need new or updated descriptions.

```go
// SelectIncrementalFiles determines which files need (re-)description
// based on content hash changes and dependency-driven reprocessing.
func SelectIncrementalFiles(
    ctx context.Context,
    q db.Querier,
    projectID pgtype.UUID,
    currentSnapshotID pgtype.UUID,
    previousSnapshotID pgtype.UUID,
) (toDescribe []db.File, toSkip []string, err error)
```

### Change Detection Strategy

A file requires re-description when any of the following is true:

**Direct changes:**
1. File content hash differs between the current and previous description's `content_hash`
2. File is new (no existing description in any snapshot for this project+path)

**Dependency-driven changes:**
3. Any file imported by this file has a changed content hash
4. Any file imported by this file changed its public exports
5. Any file that imports this file (consumer) was added or removed

### Implementation

```
1. Load all files in the current (active) snapshot
2. Load existing description hashes from previous snapshot via GetExistingDescriptionHashes
3. For each file in the current snapshot:
   a. Compute input context hash
   b. Compare against stored content_hash on existing description
   c. If hash matches → skip
   d. If hash differs or no description exists → mark for re-description
4. Check dependency-driven changes:
   a. Build set of files whose exports changed between snapshots
   b. For each file that imports a changed-exports file → mark for re-description
   c. Build set of files with changed consumer lists
   d. Mark those files for re-description
5. Process the union of all marked files through the description pipeline
```

### Input Context Hash

The `content_hash` is SHA-256 of the concatenation of: file content hash (from `files.file_hash`) + symbols hash (sorted symbol names/kinds) + imports hash (sorted import names) + exports hash (sorted exported names) + consumers hash (sorted consumer file paths).

```go
// ComputeContextHash produces a deterministic hash of all inputs that
// affect a file's description. If any component changes, the hash changes.
func ComputeContextHash(
    fileHash string,
    symbols []db.Symbol,
    imports []db.Dependency,
    exports []db.Export,
    consumers []string,
) string
```

### Copy-Forward

Unchanged descriptions are copied from the previous snapshot to the current snapshot using `CopyForwardDescriptions` (defined in ticket 16). This ensures the active snapshot always has a complete set of descriptions.

Copy-forward runs before processing changed files so that if the job fails partway through, the snapshot still has descriptions for all unchanged files.

### Previous Snapshot Resolution

Find the most recent snapshot with existing descriptions:

```sql
-- name: GetPreviousDescribedSnapshot :one
SELECT s.id
FROM index_snapshots s
WHERE s.project_id = $1
  AND s.branch = $2
  AND s.id != $3
  AND EXISTS (
    SELECT 1 FROM file_descriptions fd
    WHERE fd.index_snapshot_id = s.id
  )
ORDER BY s.activated_at DESC NULLS LAST
LIMIT 1;
```

If no previous described snapshot exists, fall back to full-describe behavior (all files).

### Exports Change Detection

```sql
-- name: ListExportsBySnapshot :many
SELECT file_id, exported_name, export_kind
FROM exports
WHERE index_snapshot_id = $1
ORDER BY file_id, exported_name;
```

Compare export sets between old and new snapshots. A file's exports have changed if its set of `(exported_name, export_kind)` pairs differs.

### Handler Integration

The `Handle` method in `handler.go` branches on workflow name:
- `describe-full`: all files (ticket 17)
- `describe-file`: single file (ticket 17)
- `describe-incremental`: `SelectIncrementalFiles` → copy forward → process changed files

The processing pipeline (context assembly, prompt, LLM call, persist, embed) is identical across all scopes.

### Qdrant Vectors for Copy-Forward

Copy-forwarded descriptions are unchanged — the file content, symbols, imports, exports, and consumers are identical. Since the description text hasn't changed, the existing Qdrant vectors remain valid and do not need to be re-embedded or re-upserted. The snapshot-level Qdrant cleanup (which deletes vectors for superseded snapshots) is responsible for lifecycle management. Copy-forward only writes to PostgreSQL.

## Key Files

| File/Package | Purpose |
|---|---|
| `backend-worker/internal/workflow/describe/incremental.go` | `SelectIncrementalFiles`, `ComputeContextHash`, copy-forward orchestration |
| `backend-worker/internal/workflow/describe/handler.go` | Branch on workflow name for file selection |
| `datastore/postgres/queries/descriptions.sql` | `GetPreviousDescribedSnapshot`, `ListExportsBySnapshot` |
| `backend-worker/internal/workflow/incremental/handler.go` | Reference pattern for copy-forward logic |

## Acceptance Criteria
- [ ] `describe-incremental` selects only files whose content_hash has changed
- [ ] Files with changed exports trigger re-description of their importers
- [ ] Files with changed consumer lists are re-described
- [ ] Unchanged descriptions copied forward via `CopyForwardDescriptions`
- [ ] Copy-forward executes before processing changed files
- [ ] `ComputeContextHash` produces deterministic hash from file hash, symbols, imports, exports, consumers
- [ ] Falls back to full-describe behavior when no previous described snapshot exists
- [ ] Previous snapshot resolution finds the most recent snapshot with existing descriptions
- [ ] Qdrant vectors upserted only for newly generated descriptions; copy-forwarded descriptions skip Qdrant (existing vectors remain valid)
- [ ] Progress reporting reflects total files (changed + copy-forward) in the denominator
- [ ] Unit tests cover: all-changed (same as full), no-changed (pure copy-forward), mixed, export-driven, consumer-driven
