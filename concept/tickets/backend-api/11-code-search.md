# 11 — Full-Text Code Search

## Status
Done

## Goal
Implemented a full-text code search endpoint that queries `code_chunks.content` using three search modes, with language filtering, file pattern globs, and directory include/exclude filters. The dynamic SQL builder reuses shared utilities from the symbol search infrastructure and returns paginated results with per-chunk match counts.

## Depends On
- Ticket 08 (Provider Settings -- active snapshot lookup requires indexed data)
- Symbol Search Filters (provides `symbolquery.go` utilities: `searchMode`, `globToRegex`, `parseDirFilter`, `parseSearchMode`, `isInvalidRegexError`)

## Scope

### Endpoint

`POST /v1/projects/{projectID}/query/search` (dual-auth: session or API key, member+)

### Request Body

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `query` | string | Yes | -- | Text pattern to search in `code_chunks.content` (max 1000 chars) |
| `search_mode` | enum | No | `"insensitive"` | `insensitive` (ILIKE), `sensitive` (LIKE), `regex` (POSIX `~`) |
| `language` | string | No | -- | Exact match on `files.language` |
| `file_pattern` | string | No | -- | Comma-separated glob patterns on `files.file_path` |
| `include_dir` | string | No | -- | Comma-separated glob patterns -- include only matching directories |
| `exclude_dir` | string | No | -- | Comma-separated glob patterns -- exclude matching directories |
| `limit` | int | No | 20 | Page size (max 100) |
| `offset` | int | No | 0 | Pagination offset |

### Search Modes

- **insensitive**: Case-insensitive matching via `ILIKE` with LIKE-escaped wildcards (`%`, `_`, `\` escaped with backslash)
- **sensitive**: Case-sensitive matching via `LIKE` with the same escaping
- **regex**: PostgreSQL POSIX regex matching via `~` operator. Patterns are pre-validated in Go via `regexp/syntax.Parse` before being sent to PostgreSQL. Invalid regex returns 422.

### Dynamic SQL Builder (`internal/handler/codesearchquery.go`)

`buildCodeSearchQuery` constructs both a data query and a COUNT query from the same WHERE clause. It builds parameterized SQL with `$N` placeholders -- no string interpolation of user input. Reuses `searchMode`, `globToRegex`, `parseDirFilter`, `parseSearchMode`, and `isInvalidRegexError` from `symbolquery.go` (same package).

The `escapeLike` function escapes `%`, `_`, and `\` characters in LIKE/ILIKE patterns using backslash as the escape character, paired with `ESCAPE '\'` in the SQL clause.

The `parseFilePattern` helper delegates to `parseDirFilter` since validation rules are identical: max 10 patterns, no `..`, no absolute paths, max 500 characters each.

### Match Count

`countMatches` computes the number of occurrences of the query pattern within each returned chunk's content on the Go side. For insensitive mode, both content and query are lowercased before counting. For regex mode, `regexp.FindAllStringIndex` is used. This avoids complex SQL expressions and only processes the paginated result set (max 100 rows).

### Response Shape

```json
{
  "items": [
    {
      "chunk_id": "uuid",
      "file_path": "src/handler/project.go",
      "language": "go",
      "start_line": 45,
      "end_line": 78,
      "content": "full chunk content...",
      "match_count": 3
    }
  ],
  "total": 142,
  "snapshot_id": "uuid",
  "limit": 20,
  "offset": 0
}
```

Results are ordered by `file_path ASC, start_line ASC, id ASC`. The `items` list returns `[]` (not `null`) when no results. If no active snapshot exists, returns 200 with empty items.

### Filter Composition

All filters combine with AND logic: `query AND language AND file_pattern AND include_dir AND NOT exclude_dir`. Within `file_pattern` and `include_dir`, multiple patterns combine with OR (match at least one). Each `exclude_dir` pattern is applied as `AND NOT`.

### Performance

No schema migration required. The `idx_chunks_snapshot(project_id, index_snapshot_id)` composite index narrows the search partition first, then PostgreSQL performs sequential scan on `content` within that partition. For typical project sizes (10K-100K chunks) this is fast enough. A `pg_trgm` GIN trigram index can be added later if needed.

## Key Files

| File | Purpose |
|------|---------|
| `backend-api/internal/handler/codesearchquery.go` | `buildCodeSearchQuery`, `codeSearchParams`, `codeSearchResult`, `escapeLike`, `parseFilePattern`, `countMatches` |
| `backend-api/internal/handler/project.go` | `HandleSearch` implementation, `codeSearchRequest`, `codeSearchMatch`, `codeSearchResponse` types |
| `backend-api/internal/handler/symbolquery.go` | Shared utilities: `searchMode`, `globToRegex`, `parseDirFilter`, `parseSearchMode`, `isInvalidRegexError` |
| `backend-api/internal/handler/codesearchquery_test.go` | Unit tests for dynamic SQL builder |

## Acceptance Criteria
- [x] `POST /v1/projects/{id}/query/search` returns 200 with paginated results
- [x] Empty/missing `query` returns 400 (not 200 with empty results)
- [x] Default search mode (insensitive) performs ILIKE matching on `code_chunks.content`
- [x] Sensitive mode performs case-sensitive LIKE matching
- [x] Regex mode performs PostgreSQL POSIX regex matching with Go-side pre-validation
- [x] Invalid regex returns 422, not 500
- [x] LIKE wildcards (`%`, `_`) in query text are escaped to match literally
- [x] `language` filter performs exact match on `files.language`
- [x] `file_pattern` converts globs to POSIX regex on `files.file_path`
- [x] `include_dir` and `exclude_dir` apply directory-level glob filtering
- [x] All filters combine with AND logic; pattern lists combine with OR
- [x] Pagination defaults to limit=20, caps at 100; `total` count applies all filters
- [x] Items list returns `[]` (not null) when no results
- [x] Each item includes `chunk_id`, `file_path`, `language`, `start_line`, `end_line`, `content`, `match_count`
- [x] `match_count` computed Go-side for each returned chunk
- [x] Results ordered by `file_path ASC, start_line ASC`
- [x] `snapshot_id` included in response
- [x] No active snapshot returns 200 with empty items
- [x] All dynamic SQL uses parameterized queries (no string interpolation of user input)
