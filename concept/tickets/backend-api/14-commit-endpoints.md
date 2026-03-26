# 14 — Commit History, Detail & Diffs

## Status
Done

## Goal
Implemented commit browsing API endpoints: a paginated commit list with text search and date range filters, single commit detail with parent hashes and diff stats, and per-file diffs with an `include_patch` toggle and single-diff lazy loading via `diff_id`. These power the backoffice commit browser and enable efficient per-file patch fetching.

## Depends On
- Task 17 (Index Job Creation — commits ingested by worker)
- Migration 000001_init (`commits`, `commit_parents`, `commit_file_diffs` tables)
- sqlc queries in `datastore/postgres/queries/commits.sql`

## Scope

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/projects/{id}/commits` | Paginated commit list with optional search/date filters |
| GET | `/v1/projects/{id}/commits/{hash}` | Single commit detail with parents and diff stats |
| GET | `/v1/projects/{id}/commits/{hash}/diffs` | Per-file diffs for a commit |

### Commit List with Filters

`GET /commits?limit=20&offset=0` returns paginated commits ordered by `committer_date DESC, id DESC`. Three optional filter parameters extend the base listing:

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `search` | string | — | Case-insensitive ILIKE substring match on commit message |
| `from_date` | RFC3339 | — | Filter commits with `committer_date >= value` |
| `to_date` | RFC3339 | — | Filter commits with `committer_date <= value` |
| `limit` | int | 20 | Page size (max 100) |
| `offset` | int | 0 | Pagination offset |

When any filter is present, a dynamic SQL builder (`commitquery.go`) constructs parameterized queries, following the same pattern as the symbol query builder. When no filters are present, the existing static sqlc queries are used for backward compatibility.

### Commit Detail

`GET /commits/{hash}` returns the full commit with:
- `parents` array — each with `parent_commit_id`, `parent_commit_hash`, `parent_short_hash`, `ordinal`
- `diff_stats` — `files_changed`, `total_additions`, `total_deletions`
- All summary fields: `commit_hash`, `short_hash`, `author_name`, `author_date`, `committer_name`, `committer_date`, `message`, `message_subject`

Email fields (`author_email`, `committer_email`) are conditionally included based on caller role.

### Per-File Diffs with Lazy Patch Loading

`GET /commits/{hash}/diffs` supports two modes:

1. **Bulk mode** (default): returns all file diffs for the commit. `include_patch=true` includes the full unified diff; `include_patch=false` (or omitted) returns metadata only (file paths, change type, additions, deletions) via a separate query that omits the `patch` column.
2. **Single-diff mode**: when `diff_id={uuid}` is provided, returns only that one diff with patch content. This avoids transferring all patches when the user expands a single file. `diff_id` implies `include_patch=true`.

Bulk mode pagination: default limit 200, max 500.

### Response Shapes

Commit list: `{ items: [CommitSummary], total, limit, offset }`

Commit detail: `CommitSummary` extended with `parents: [CommitParent]` and `diff_stats: { files_changed, total_additions, total_deletions }`

Diffs: `{ commit_hash, diffs: [FileDiff], total, limit, offset }` where each FileDiff includes `id`, `old_file_path`, `new_file_path`, `change_type`, `patch` (omitempty), `additions`, `deletions`, `parent_commit_id`.

## Key Files

| File | Description |
|------|-------------|
| `backend-api/internal/handler/commits.go` | `CommitHandler` with `HandleList`, `HandleGet`, `HandleDiffs`; response types; `toCommitSummary`, `toCommitParents`, `toFileDiff` helpers |
| `backend-api/internal/handler/commitquery.go` | Dynamic SQL builder for commit list with search and date range filters |
| `backend-api/internal/app/app.go` | `CommitHandler` wiring |
| `backend-api/internal/app/routes.go` | Three commit routes registered in project member group |
| `datastore/postgres/queries/commits.sql` | `ListProjectCommits`, `CountProjectCommits`, `GetCommitByHash`, `GetCommitDiffStats`, `GetCommitParents`, `ListCommitFileDiffs`, `ListCommitFileDiffsMeta`, `CountCommitFileDiffs`, `GetCommitFileDiff` |

## Acceptance Criteria
- [x] `GET /commits` returns paginated commit list ordered by `committer_date DESC, id DESC`
- [x] Default limit 20, max 100; `total` count included for pagination
- [x] Each commit includes `short_hash` (7 chars) and `message_subject` (first line)
- [x] `search=feat` filters to commits with message containing "feat" (case-insensitive)
- [x] `from_date` and `to_date` filter by committer date range (RFC3339 format)
- [x] Invalid date formats return 400 with descriptive message
- [x] All filters combinable; pagination works correctly with filters
- [x] Dynamic SQL uses parameterized queries; COUNT query applies same filters
- [x] `GET /commits/{hash}` returns full commit with `parents` and `diff_stats`
- [x] Returns 404 for unknown commit hash
- [x] `GET /commits/{hash}/diffs` returns per-file diffs with change_type, file paths, additions, deletions
- [x] `include_patch=true` includes unified diff patch content; omitted by default
- [x] `diff_id={uuid}` returns single diff with patch (lazy loading); invalid UUID returns 400
- [x] `diff_id` for wrong commit or wrong project returns 404
- [x] Empty project (no commits) returns `{ items: [], total: 0 }` (not error)
- [x] All three endpoints require project membership (dual-auth)
- [x] All three endpoints use the consistent error envelope `{error, code, details}`
