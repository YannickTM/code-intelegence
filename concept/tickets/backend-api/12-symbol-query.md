# 12 — Symbol Search & Detail Endpoints

## Status
Done

## Goal
Implemented the symbol list and detail API endpoints, enabling the backoffice Symbol Browser and MCP tools to query symbols extracted during indexing. The list endpoint supports name search with configurable search modes (case-insensitive, case-sensitive, regex), kind filtering, directory include/exclude glob patterns, and pagination. V2 fields (flags, modifiers, return_type, parameter_types) are exposed in all symbol responses.

## Depends On
- Datastore Task 07 (symbol search queries)
- Migration 000001_init (`symbols`, `files`, `index_snapshots` tables including v2 columns: `flags`, `modifiers`, `return_type`, `parameter_types`)

## Scope

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/projects/{id}/symbols` | List symbols with optional filters and pagination |
| GET | `/v1/projects/{id}/symbols/{symbolID}` | Get single symbol detail |

### List Query Parameters

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | — | Symbol name filter pattern |
| `kind` | string | — | Exact match on symbol kind (function, class, etc.) |
| `search_mode` | enum | `insensitive` | One of: `insensitive` (ILIKE), `sensitive` (LIKE), `regex` (POSIX `~`) |
| `include_dir` | string | — | Comma-separated glob patterns; only symbols from matching file paths |
| `exclude_dir` | string | — | Comma-separated glob patterns; exclude symbols from matching file paths |
| `limit` | int | 50 | Page size (max 200) |
| `offset` | int | 0 | Pagination offset |

### Dynamic SQL Builder

The combinatorial explosion of filter parameters (search_mode x include_dir x exclude_dir x name x kind) made static sqlc queries impractical. A handler-local dynamic SQL builder (`symbolquery.go`) constructs parameterized data and COUNT queries from the same WHERE clause. All user input is passed via `$N` placeholders.

### Glob-to-Regex Conversion

Directory filter patterns use VS Code-style glob syntax (`**`, `*`, `?`) converted to PostgreSQL POSIX regex via `globToRegex`. Patterns are validated both in Go (`regexp/syntax.Parse`) and at the PostgreSQL level (SQLSTATE `2201B` fallback). Maximum 10 patterns per parameter, no `..` traversal, no absolute paths, 500-char limit per pattern.

### V2 Symbol Fields

Each symbol response includes:
- `flags` — JSON object with boolean properties: `is_exported`, `is_default_export`, `is_async`, `is_generator`, `is_static`, `is_abstract`, `is_readonly`, `is_optional`, `is_arrow_function`, `is_react_component_like`, `is_hook_like`
- `modifiers` — string array
- `return_type` — nullable string
- `parameter_types` — string array

Fields are omitted from JSON when null/empty via `omitempty`.

### Response Shape (list)

```json
{
  "items": [{ "id", "name", "qualified_name", "kind", "signature", "start_line", "end_line", "doc_text", "file_path", "language", "flags", "modifiers", "return_type", "parameter_types" }],
  "total": 42,
  "snapshot_id": "uuid",
  "limit": 50,
  "offset": 0
}
```

## Key Files

| File | Description |
|------|-------------|
| `backend-api/internal/handler/symbolquery.go` | Dynamic SQL builder, glob-to-regex converter, input validators |
| `backend-api/internal/handler/project.go` | `HandleListSymbols`, `HandleGetSymbol`, response types, `toSymbolResponse` / `makeSymbolResponse` helpers |
| `datastore/postgres/queries/symbols.sql` | `GetSymbolByID`, `ListSymbolChildren` (8 obsolete static queries removed) |

## Acceptance Criteria
- [x] `GET /v1/projects/{id}/symbols` returns real symbol data from the database
- [x] Returns empty `items` array (not error) when no active snapshot exists
- [x] Supports `name` query param for substring search with exact-match priority ranking
- [x] Supports `kind` query param for exact kind filtering
- [x] Supports combined `name` + `kind` filtering
- [x] `search_mode=insensitive` (default) uses ILIKE matching
- [x] `search_mode=sensitive` uses case-sensitive LIKE matching
- [x] `search_mode=regex` uses PostgreSQL POSIX regex matching
- [x] Invalid regex patterns return 422, not 500
- [x] `include_dir` filters symbols to matching file paths (OR logic across patterns)
- [x] `exclude_dir` excludes symbols from matching file paths (AND NOT logic)
- [x] Glob patterns supported: `**`, `*`, `?`, plain prefix
- [x] Directory filters validated: max 10 patterns, no `..`, no absolute paths, max 500 chars
- [x] All dynamic SQL uses parameterized queries
- [x] Pagination with `limit`/`offset` (default 50, max 200) and `total` count
- [x] `GET /v1/projects/{id}/symbols/{symbolID}` returns single symbol with full detail
- [x] Returns 400 for invalid (non-UUID) symbol ID; 404 when symbol does not exist
- [x] V2 fields (`flags`, `modifiers`, `return_type`, `parameter_types`) included in list and detail responses
- [x] `flags` returned as JSON object; `modifiers`/`parameter_types` as arrays
- [x] Both endpoints require project membership (dual-auth)
- [x] Both endpoints use the consistent error envelope `{error, code, details}`
