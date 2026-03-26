# 13 — File Analysis & Context Endpoints

## Status
Done

## Goal
Built the complete file analysis API surface: file content with v2 metadata, per-file git history, bidirectional dependency lookups, a BFS dependency graph, and four v2 parser data endpoints (exports, references, JSX usages, network calls). These endpoints power the backoffice File Detail View, dependency graph visualization, and editorial history cards.

## Depends On
- Task 17 (Index Job Creation — snapshots, files, dependencies populated by worker)
- Migration 000001_init (all tables: `files` with v2 columns `file_facts`/`parser_meta`/`extractor_statuses`/`issues`, `file_contents`, `dependencies`, `commits`, `commit_file_diffs`, `exports`, `symbol_references`, `jsx_usages`, `network_calls`)

## Scope

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/projects/{id}/files/context` | File content + v2 metadata (file_facts, issues, parser_meta, extractor_statuses) |
| GET | `/v1/projects/{id}/files/history` | Paginated git log for a specific file path |
| GET | `/v1/projects/{id}/files/dependencies` | Bidirectional import/importer lookup for a file |
| GET | `/v1/projects/{id}/dependencies/graph` | BFS DAG traversal from a root file, configurable depth, 200-node cap |
| GET | `/v1/projects/{id}/files/exports` | Exports declared by a file |
| GET | `/v1/projects/{id}/files/references` | Symbol references made within a file |
| GET | `/v1/projects/{id}/files/jsx-usages` | JSX component usages in a file |
| GET | `/v1/projects/{id}/files/network-calls` | Detected HTTP/fetch calls in a file |
| GET | `/v1/projects/{id}/conventions` | Project-level coding conventions |

### File Context (v2 Metadata)

The `GET /files/context` endpoint returns file content alongside four JSONB fields on the `files` table:
- `file_facts` — boolean summary flags (has_jsx, has_default_export, has_tests, has_fetch_calls, etc.)
- `issues` — parser diagnostics array (parse errors, long function warnings with code, message, line, severity)
- `parser_meta` — parser and grammar version metadata
- `extractor_statuses` — per-extractor success/failure status

All four fields pass through as raw JSON and are omitted when null.

### File History

`GET /files/history?file_path=...&limit=10&offset=0` returns paginated commits that modified a specific file, ordered by `committer_date DESC`. Each entry includes `diff_id`, `commit_hash`, `short_hash`, `author_name`, `committer_date`, `message_subject`, `change_type`, `additions`, `deletions`. File path matching covers both `new_file_path` and `old_file_path` to handle renames.

### File Dependencies & Graph

`GET /files/dependencies?file_path=...` returns two arrays: `imports` (forward — what this file imports) and `imported_by` (reverse — what imports this file). Each edge includes source/target paths, import name/type, and optional package info.

`GET /dependencies/graph?root=...&depth=2` performs application-level BFS traversal expanding both forward and reverse edges. Default depth 2, max 5. Node cap of 200 with `truncated` flag. External packages appear as `ext:` prefixed nodes. Edges are deduplicated by source+target+import_name.

### V2 Parser Data Endpoints

Four endpoints follow an identical pattern: look up active snapshot, find file by path, query the relevant table by file_id, return items with total and snapshot_id. Each returns file-scoped data populated by the worker during indexing.

## Key Files

| File | Description |
|------|-------------|
| `backend-api/internal/handler/project.go` | `HandleFileContext`, `HandleFileHistory`, `HandleFileDependencies`, `HandleDependencyGraph`, `HandleFileExports`, `HandleFileReferences`, `HandleFileJsxUsages`, `HandleFileNetworkCalls`, `HandleConventions`; response types and mapping helpers |
| `backend-api/internal/app/routes.go` | Route registration for all file-scoped endpoints under project member group |
| `datastore/postgres/queries/commits.sql` | `ListFileDiffsByPathWithCommit`, `CountFileDiffsByPath` |
| `datastore/postgres/queries/dependencies.sql` | Forward/reverse/batch dependency queries, external packages, file counts |
| `datastore/postgres/queries/files.sql` | `GetFileWithContent`, `GetFileWithContentAndAST`, `GetActiveSnapshotForProject` |

## Acceptance Criteria
- [x] `GET /files/context?file_path=...` returns content, language, size, line count, content hash, and snapshot metadata
- [x] V2 metadata fields (`file_facts`, `issues`, `parser_meta`, `extractor_statuses`) included when data exists, omitted when null
- [x] `?include_ast=true` includes `tree_sitter_ast` JSONB in the response
- [x] `GET /files/history?file_path=...` returns paginated commit history (default limit 10, max 50)
- [x] File history matches both `new_file_path` and `old_file_path` for rename coverage
- [x] `file_path` is required on all file-scoped endpoints; missing returns 400
- [x] `GET /files/dependencies?file_path=...` returns bidirectional `imports` and `imported_by` arrays
- [x] `GET /dependencies/graph?root=...&depth=2` returns nodes and edges via BFS traversal
- [x] Graph depth is configurable (default 2, max 5, clamped); node cap is 200 with `truncated` flag
- [x] External dependencies appear as `is_external: true` nodes with `ext:` prefix
- [x] Graph edges are deduplicated
- [x] `GET /files/exports` returns export_kind, exported_name, local_name, source_module, line, column
- [x] `GET /files/references` returns reference_kind, target_name, line/column range, resolution_scope, confidence
- [x] `GET /files/jsx-usages` returns component_name, is_intrinsic, is_fragment, line, column
- [x] `GET /files/network-calls` returns client_kind, method, url_literal/url_template, is_relative
- [x] All endpoints return empty `items: []` when no active snapshot or file not found
- [x] All endpoints require project membership (dual-auth)
- [x] All endpoints use the consistent error envelope `{error, code, details}`
