# 12 — Commit Indexing Workflow

## Status
Done

## Goal
Persist git commit history during full-index and incremental-index workflows so the commit browser (backoffice and API) has data to serve. Includes git log extraction, file change stats with unified diff patches, and database persistence into `commits`, `commit_parents`, and `commit_file_diffs` tables.

## Depends On
04-workspace, 10-full-index

## Scope

### Git Log Extraction (`internal/gitclient/`)

Two new methods on the git client extract commit history from a repository checkout:

**`LogCommits(ctx, repoDir, sinceCommit, maxCount)`** extracts commit metadata via `git log` with a machine-parseable format using ASCII record/unit separators (`\x1E`/`\x1F`). Fields per commit: hash, parent hashes, author name, author email, author date (RFC 3339), committer name, committer email, committer date (RFC 3339), full message body.

- `sinceCommit` empty: full history with `--first-parent` (follow mainline only)
- `sinceCommit` set: range `sinceCommit..HEAD` (new commits only)
- `maxCount` maps to `--max-count` (0 = no limit)

**`DiffStatLog(ctx, repoDir, sinceCommit, maxCount)`** extracts per-commit file-level change data by running three git commands:

1. `git log --name-status -z --no-renames --diff-filter=ACDMT` -- change types (A/M/D per file)
2. `git log --numstat -z --no-renames --diff-filter=ACDMT` -- line additions/deletions per file
3. `git log -p --no-renames --diff-filter=ACDMT` -- unified diff patches per file

All commands use `--format='COMMIT:%H'` as a record separator. Commands 1 and 2 use `-z` for NUL-delimited output (safe for filenames with special characters). Outputs are parsed and merged by commit hash + file path into `map[string][]FileDiffEntry`.

Patch safeguards: binary files (`Binary files ... differ`) are detected and skipped. Patches exceeding 100 KB per file are discarded to prevent excessive database storage.

### Commit Indexer (`internal/workflow/commits/`)

The `Indexer` struct encapsulates commit persistence logic:

```go
type Indexer struct {
    queries db.Querier
    git     commitLogProvider
}
```

**`IndexAll(ctx, projectID, repoDir, maxCommits)`** -- indexes full history up to `maxCommits`.

**`IndexSince(ctx, projectID, repoDir, sinceCommit)`** -- indexes only commits newer than `sinceCommit`.

Core persistence flow (shared by both methods):

1. Extract commits via `LogCommits`
2. Extract file diffs via `DiffStatLog`
3. For each commit: `InsertCommit` (idempotent upsert by `(project_id, commit_hash)`)
4. For each commit's parents: resolve parent DB ID from current batch map or `GetCommitByHash` fallback; `InsertCommitParent` (skip if parent not found)
5. For each commit's file diffs: `InsertCommitFileDiff` with:
   - `change_type`: `A` -> `added`, `M` -> `modified`, `D` -> `deleted`
   - `old_file_path` / `new_file_path`: NULL for added/deleted respectively
   - `patch`: unified diff content (NULL for binary files or patches > 100 KB)
   - `additions` / `deletions`: from numstat output
6. Return HEAD commit's DB ID (first entry, newest-first order)

### Workflow Integration

Commit indexing is wired into both workflows as a **non-fatal** step. The primary value of indexing is file/chunk/vector data; commit history is supplementary.

**Full-index handler:**
- Calls `IndexAll(projectID, repoDir, 5000)` after workspace preparation
- 5000 commit limit prevents runaway on large repositories
- Links snapshot to HEAD commit via `UpdateSnapshotCommitID` on success

**Incremental-index handler:**
- In incremental path: calls `IndexSince(projectID, repoDir, baseSnapshot.GitCommit)` to index only new commits
- In full fallback path: calls `IndexAll(projectID, repoDir, 5000)`
- Links snapshot to HEAD commit with fallback to `GetCommitByHash` for already-indexed commits

### Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Failure behavior | Non-fatal (warn log) | Primary value is file/chunk indexing; commits are supplementary |
| Full-index commit limit | 5000 | Prevents runaway on large repos; covers most project histories |
| `--first-parent` for full history | Yes | Follows mainline; avoids exponential growth on merge-heavy repos |
| `--no-renames` | Yes | Consistent with existing `DiffNameStatus`; renames appear as delete+add |
| Patch size limit | 100 KB per file | Prevents excessive database storage for large diffs |
| Package location | `workflow/commits` | Colocated with other workflow packages under `internal/workflow/` |

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/workflow/commits/indexer.go` | Commit indexer: `IndexAll`, `IndexSince`, persistence logic |
| `internal/workflow/commits/indexer_test.go` | Unit tests with mock git client and querier |
| `internal/gitclient/gitclient.go` | `LogCommits`, `DiffStatLog`, `CommitLog`, `FileDiffEntry` types |
| `internal/gitclient/gitclient_test.go` | Tests for git log parsing, diff merging, patch extraction |
| `internal/workflow/fullindex/handler.go` | `commitIndexer` interface wiring, non-fatal step |
| `internal/workflow/incremental/handler.go` | `commitIndexer` interface wiring, `IndexSince` in incremental path |
| `internal/app/app.go` | Bootstrap: instantiates `commits.New()`, passes to handler constructors |

## Acceptance Criteria
- [x] `LogCommits` extracts commit metadata (hash, parents, author, committer, message, dates) from git log
- [x] `DiffStatLog` extracts per-commit file changes (status, path, additions, deletions) and unified diff patches
- [x] Binary files detected and patches omitted; patches > 100 KB discarded
- [x] Full-index workflow persists up to 5000 commits into `commits`, `commit_parents`, `commit_file_diffs`
- [x] Incremental-index workflow persists only new commits since base snapshot's commit
- [x] Incremental fallback-to-full also indexes commit history
- [x] Snapshot linked to HEAD commit via `UpdateSnapshotCommitID` on success
- [x] Commit indexing failure does not fail the indexing job (non-fatal, warn log)
- [x] Idempotent: re-indexing same commits does not produce duplicates (ON CONFLICT upserts)
- [x] Parent relationships correctly resolved within indexed range; missing parents gracefully skipped
- [x] `--first-parent` used for full history to follow mainline
