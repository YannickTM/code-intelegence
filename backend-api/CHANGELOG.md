# Changelog

## [Unreleased]

### Added
- **Lazy Job Reaper (Task 17):** On-demand dead-worker detection on API read paths
  - New `jobhealth.Checker` checks worker heartbeat in Redis when a `running` job is encountered
  - If the worker is dead or moved on, the job is transitioned to `failed` inline before the API response
  - Integrated into `GET /v1/projects/{id}`, `GET /v1/users/me/projects`, and `GET /v1/projects/{id}/jobs`
  - Publishes `job:failed` SSE event and cleans up orphaned `building` snapshots on reap
  - Fast paths: non-running status, empty worker ID, and fresh jobs (within `REAPER_STALE_THRESHOLD`, default 5m) skip Redis
  - Graceful degradation: Redis or DB errors are logged but never fail the parent request
  - `REAPER_STALE_THRESHOLD` env var configures the minimum job age before heartbeat checks
  - Unit tests with miniredis covering alive worker, dead worker, worker-moved-on, DB/Redis errors, nil checker
- **File Metadata V2 (Task 39):** Extended `GET /v1/projects/{projectID}/files/context` with four v2 JSONB fields from the `files` table
  - `file_facts`: boolean summary flags (has_jsx, has_default_export, has_tests, etc.)
  - `issues`: parser diagnostics (parse errors, long functions, deep nesting warnings)
  - `parser_meta`: parser and grammar version metadata
  - `extractor_statuses`: per-extractor success/failure status
  - All fields use `json.RawMessage` passthrough and are omitted when null (`omitempty`)
  - Updated both `GetFileWithContent` and `GetFileWithContentAndAST` sqlc queries
- **File Analysis Endpoints (Task 38):** Four new file-scoped GET endpoints exposing v2 parser data for the backoffice File Detail View
  - `GET /files/exports?file_path=...` — exports with export_kind, exported_name, local_name, source_module, line/column
  - `GET /files/references?file_path=...` — symbol references with reference_kind, target_name, line/column range, resolution_scope, confidence
  - `GET /files/jsx-usages?file_path=...` — JSX component usages with component_name, is_intrinsic, is_fragment, line/column
  - `GET /files/network-calls?file_path=...` — network/API calls with client_kind, method, url_literal/url_template, is_relative
  - All endpoints return empty items array when no active snapshot or file not found
  - All endpoints require project membership (dual-auth)
  - Added lightweight `GetFileBySnapshotAndPath` sqlc query for efficient file ID lookup
- **Symbol V2 Fields (Task 37):** Extended `GET /v1/projects/{projectID}/symbols` and `GET /v1/projects/{projectID}/symbols/{symbolID}` with four new fields from parser v2 metadata
  - `flags`: JSON object with boolean symbol flags (`is_exported`, `is_async`, `is_static`, etc.); omitted when null
  - `modifiers`: string array of symbol modifiers (e.g. `["async","export"]`); empty `[]` when no data
  - `return_type`: nullable string for function/method return types; omitted when null
  - `parameter_types`: string array of parameter types; empty `[]` when no data
  - Updated `GetSymbolByID` sqlc query and dynamic list query to include new columns
  - Three new integration tests: v2 fields in list, v2 fields in get-by-id, empty arrays for null data
- **Codebase Text Search (Task 35):** `POST /v1/projects/{projectID}/query/search` — full-text code search through indexed `code_chunks.content`
  - `query` param (required): text pattern to search for in code content
  - `search_mode` param: `insensitive` (default ILIKE), `sensitive` (LIKE), `regex` (PostgreSQL POSIX `~`)
  - `language` param: exact match filter on file language
  - `file_pattern` param: comma-separated glob patterns on file path (e.g. `*.go`, `**/*.test.ts`)
  - `include_dir` / `exclude_dir` params: comma-separated glob patterns for directory filtering
  - Response includes `match_count` per chunk (computed Go-side), ordered by file path then line number
  - Dynamic SQL builder (`codesearchquery.go`) reuses `symbolquery.go` utilities; all user input via `$N` parameters
  - Pagination: default 20, max 100 results per page
- **Commit Search & Date Range Filters (Task 36):** Extended `GET /v1/projects/{projectID}/commits` with three new optional query parameters
  - `search` param: case-insensitive substring match on commit message (ILIKE)
  - `from_date` param: filter commits with `committer_date >= value` (RFC3339 timestamp)
  - `to_date` param: filter commits with `committer_date <= value` (RFC3339 timestamp)
  - All filters combinable with existing `limit`/`offset` pagination; total count reflects filtered results
  - Dynamic SQL builder (`commitquery.go`) follows `symbolquery.go` pattern; all user input passed via `$N` parameters
  - Backward-compatible: no-filter requests use existing static sqlc queries unchanged
- **Platform Worker Status (Task 33):** `GET /v1/platform-management/workers` returns live worker heartbeat data from Redis for platform admins
  - New `redisclient.Reader` package with SCAN/MGET utility for general-purpose Redis key reads
  - `domain.WorkerStatus` type mirroring `contracts/redis/worker-status.v1.schema.json`
  - Endpoint conditionally registered only when `REDIS_URL` is set (no route when Redis unavailable)
  - Returns 502 with descriptive message on Redis connection or read failure
  - Malformed worker entries are skipped with warning log, not failing the request
  - Results sorted by `worker_id` for deterministic ordering
- **Multi-Provider Platform Configs (Task 32):** Extended platform embedding and LLM settings to support multiple concurrent global provider configurations
  - `POST /v1/platform-management/settings/embedding` — create additional non-default global embedding provider (returns 201)
  - `PATCH /v1/platform-management/settings/embedding/{configId}` — partial in-place update of a specific config (COALESCE-based, preserves config ID for project FK references)
  - `DELETE /v1/platform-management/settings/embedding/{configId}` — soft-delete (deactivate) a non-default config; blocked if default (400) or referenced by projects (409)
  - `POST /v1/platform-management/settings/embedding/{configId}/promote` — atomically demote current default and promote target; idempotent for already-default configs
  - `POST /v1/platform-management/settings/embedding/{configId}/test` — test connectivity for a specific global config by ID
  - All five endpoints mirrored for LLM under `/v1/platform-management/settings/llm/`
  - `CreateAdditionalGlobalConfig`, `UpdateGlobalConfigByID`, `DeleteGlobalConfig`, `PromoteToDefault`, `TestGlobalConnectivityByID` service methods for both embedding and LLM
  - `GlobalPatchRequest` type for partial updates with pointer fields
  - 16 new sqlc queries: `GetGlobalEmbeddingProviderConfigByID`, `CreateGlobalEmbeddingProviderConfigNonDefault`, `DeactivateGlobalEmbeddingProviderConfigByID`, `DemoteGlobalEmbeddingProviderConfigDefault`, `PromoteGlobalEmbeddingProviderConfigToDefault`, `CountProjectsUsingEmbeddingConfig`, `UpdateGlobalEmbeddingProviderConfig`, `DeactivateDefaultGlobalEmbeddingProviderConfig`, and LLM equivalents
  - Credential carry-forward rule enforced on PATCH when provider or endpoint_url changes
  - Embedding version tracking on create and update (when provider/model/dimensions change)
- **Symbol Search Filters (Task 34):** Extended `GET /v1/projects/{projectID}/symbols` with three new optional query parameters for advanced symbol search
  - `search_mode` param: `insensitive` (default, backward-compatible ILIKE), `sensitive` (case-sensitive LIKE), `regex` (PostgreSQL POSIX regex with `~` operator)
  - `include_dir` param: comma-separated glob patterns to restrict results to matching file paths (OR logic, max 10 patterns)
  - `exclude_dir` param: comma-separated glob patterns to exclude matching file paths (AND NOT logic, max 10 patterns)
  - VS Code-style glob support: `**` (any depth), `*` (single level), `?` (single char), plain directory prefix matching
  - Regex patterns pre-validated in Go via `regexp/syntax.Parse` before PostgreSQL execution; invalid patterns return 422
  - Dynamic SQL builder (`symbolquery.go`) replaces 8 static sqlc queries with parameterized query construction; all user input passed via `$N` parameters

### Changed
- **Job queue timeouts**: Increased `DefaultIndexFullTimeout` from 30 min to 2 hours, `DefaultIndexIncrementalTimeout` from 10 min to 30 min to accommodate large repos.
- **Publisher per-workflow task options**: `queue.NewPublisher` now accepts `PublisherConfig` with per-workflow timeout and max-retry settings. `EnqueueIndexJob` passes workflow-specific `asynq.Timeout` and `asynq.MaxRetry` options so asynq enforces them instead of relying on defaults (which caused retry storms for failed jobs).
- **PUT /embedding and PUT /llm global update** now only deactivates the current default config (via `DeactivateDefaultGlobalEmbeddingProviderConfig`) instead of all global configs, preserving non-default providers across default updates

## [0.33.0] - 2026-03-23

### Added
- **Seed First Platform Admin (Task 31):** Initial platform admin role assignment via SQL seed migration and environment variable bootstrap
  - `000002_seed_admin_account.up.sql` creates `admin` user with `platform_admin` role (idempotent via `ON CONFLICT DO NOTHING`; pre-existing `admin` user is not promoted)
  - `000002_seed_admin_account.down.sql` revokes role and removes admin user (scoped to seeded email `admin@local.local`)
  - `PLATFORM_ADMIN_USERNAMES` env var enables promoting additional users to `platform_admin` at startup (comma-separated, trimmed, lowercased)
  - `ReservedUsername` validator in `internal/validate/validate.go` prevents registration with `admin` username (any casing)

## [0.32.0] - 2026-03-22

### Added
- **Platform Settings — Embedding & LLM Global Configuration (Task 30):** Replaced 403 stubs with real implementations for global embedding and LLM provider settings, gated behind `RequirePlatformAdmin` middleware
  - `GET /v1/platform-management/settings/embedding` — list all global embedding configs
  - `PUT /v1/platform-management/settings/embedding` — create/update global embedding default (immutable-append pattern with embedding version tracking)
  - `POST /v1/platform-management/settings/embedding/test` — test global embedding connectivity (active default or ad-hoc config)
  - `GET /v1/platform-management/settings/llm` — list all global LLM configs
  - `PUT /v1/platform-management/settings/llm` — create/update global LLM default (immutable-append pattern)
  - `POST /v1/platform-management/settings/llm/test` — test global LLM connectivity (active default or ad-hoc config)
  - `GlobalUpdateRequest` type and `ListGlobalConfigs`, `GetGlobalDefaultConfig`, `UpdateGlobalConfig`, `TestGlobalConnectivity` service methods for both embedding and LLM
  - 6 new sqlc queries: `ListGlobalEmbeddingProviderConfigs`, `DeactivateGlobalEmbeddingProviderConfigs`, `CreateGlobalEmbeddingProviderConfig`, and LLM equivalents
  - `is_available_to_projects` flag controllable via PUT request body (defaults to `true`)
  - Credential carry-forward logic: credentials preserved across updates unless provider/endpoint changes

### Changed
- Global settings routes moved from `/v1/settings/*` (user-only group) to `/v1/platform-management/settings/*` (platform admin group)

### Removed
- `writeProviderGlobalForbidden` helper and `"global_admin_required"` error code (no longer needed)
- `TestEmbeddingHandler_Global403` test (stubs replaced with real implementations)

## [0.31.0] - 2026-03-22

### Added
- **Platform Admin Foundation & User Management (Task 29):** Introduced the `platform_admin` role as a system-level authorization layer and implemented admin user management and platform role management endpoints
  - `user_platform_roles` table in `000001_init` migration with CHECK constraint, FK cascade, and unique index
  - `RequirePlatformAdmin` middleware gates `/v1/platform-management/*` routes (401 unauthenticated, 403 non-admin)
  - `RequireProjectRole` short-circuits to owner-level access for platform admins (with defensive DB fallback)
  - `GET /v1/platform-management/users` — paginated user list with `project_count`, `platform_roles`, search (ILIKE), `is_active` filter, and `sort` (username/created_at)
  - `GET /v1/platform-management/users/{userId}` — user detail with memberships and platform roles
  - `PATCH /v1/platform-management/users/{userId}` — update display_name and/or avatar_url
  - `POST /v1/platform-management/users/{userId}/deactivate` — soft-disable with session invalidation; last-admin guard via `FOR UPDATE` locking
  - `POST /v1/platform-management/users/{userId}/activate` — re-enable deactivated user
  - `GET /v1/platform-management/platform-roles` — list all active role assignments with user info
  - `POST /v1/platform-management/platform-roles` — grant platform role (idempotent via `ON CONFLICT DO NOTHING`)
  - `DELETE /v1/platform-management/platform-roles/{userId}/{role}` — revoke platform role; lock-first-then-delete pattern prevents deadlocks
  - `PlatformRoleAdmin` constant and `PlatformRoleKnown` validator in `domain/models.go`
  - Platform role context helpers (`ContextWithPlatformRoles`, `PlatformRolesFromContext`, `IsPlatformAdmin`) in `auth/session.go`
  - `UpsertPlatformRoleByUsername` query for seed migration and env-var bootstrap (Task 31)
  - 8 new sqlc queries in `platform_roles.sql`, 6 in `admin_users.sql`

## [0.30.0] - 2026-03-21

### Added
- **Symbol list & detail endpoints (Task 28):** Replaced `HandleListSymbols` and `HandleGetSymbol` stubs with real DB-backed implementations
  - `GET /v1/projects/{id}/symbols?name=...&kind=...&limit=50&offset=0` — paginated symbol list with optional name (ILIKE substring) and kind filtering; exact name matches ranked first
  - `GET /v1/projects/{id}/symbols/{symbolID}` — single symbol detail with joined file path and language
  - List endpoint requires active snapshot; returns empty items (not error) when none exists
  - Detail endpoint scoped by `project_id` + `symbolID` (no snapshot required) so deep links survive re-indexing
  - Response includes `total` count, `snapshot_id`, `limit`, `offset` pagination metadata
  - Symbols include joined `file_path` and `language` from the `files` table
- `symbolResponse` and `symbolListResponse` types, `makeSymbolResponse` and `nullableInt32` helpers

## [0.29.0] - 2026-03-21

### Added
- **File dependencies & graph endpoints (Task 27):** Replaced `HandleDependencies` stub with three real endpoints for dependency data
  - `GET /v1/projects/{id}/dependencies` — project-level dependency overview with external package aggregates and per-file import/imported-by counts (paginated)
  - `GET /v1/projects/{id}/files/dependencies?file_path=...` — bidirectional file-scoped dependency lookup (forward imports + reverse imported-by)
  - `GET /v1/projects/{id}/dependencies/graph?root=...&depth=2` — BFS graph traversal returning nodes and edges for dependency visualization, with 200-node cap and configurable depth (default 2, max 5)
- 7 new sqlc queries in `datastore/postgres/queries/dependencies.sql`: `ListDependenciesFromFile`, `ListDependenciesToFile`, `ListExternalDependencies`, `ListFileDependencyCounts`, `CountFilesWithDependencies`, `ListDependenciesFromFiles`, `ListDependenciesToFiles`
- `idx_deps_target` partial index on `dependencies(project_id, index_snapshot_id, target_file_path)` for reverse dependency lookups
- Helper functions: `toDependencyEdge`, `nullableString`, `deduplicateEdges`
- **File history endpoint (025):** `GET /v1/projects/{id}/files/history?file_path=...&limit=10&offset=0` returns paginated commit history for a specific file, powering the Editorial History card in the backoffice file viewer
  - Each entry includes `diff_id`, `commit_hash`, `short_hash`, `author_name`, `committer_date`, `message_subject`, `change_type`, `additions`, `deletions`
  - File path matches both `new_file_path` and `old_file_path` for partial rename tracking
  - Default limit 10, max 50; ordered by `committer_date DESC`
  - `file_path` query param required — returns 400 if missing
- New `ListFileDiffsByPathWithCommit` and `CountFileDiffsByPath` sqlc queries joining `commit_file_diffs` with `commits` for commit metadata
- 4 unit tests for mapping helper and handler validation; 6 integration tests covering ordering, pagination, rename matching, empty results, and project isolation

## [0.28.0] - 2026-03-19

### Added
- **Configurable `max_tokens` for embedding providers** — Added `max_tokens` as a first-class field on embedding provider configs (domain model, handler validation, service layer, DB conversion, OpenAPI contract). Replaces the previously hardcoded 7500-character truncation limit in the worker. Default: 8000, range: 1–131072. Configurable per embedding config via the API and backoffice UI.
- New `EMBEDDING_MAX_TOKENS` environment variable for overriding the bootstrap default
- Updated `PUT /v1/projects/{id}/settings/embedding` custom mode to require `max_tokens`
- Updated integration tests with `max_tokens` in all custom embedding PUT payloads

## [0.27.0] - 2026-03-11

### Added
- **Per-file patch fetching (024):** New `diff_id` query parameter on `GET /v1/projects/{id}/commits/{hash}/diffs` to fetch the patch for a single file instead of all files in a commit
  - `diff_id` implies `include_patch=true` — no need to set both
  - Returns same `CommitDiffsResponse` shape with `total: 1`
  - Invalid UUID returns 400; nonexistent or cross-project/cross-commit diff_id returns 404
  - Existing bulk behavior unchanged when `diff_id` is absent
- New `GetCommitFileDiff` sqlc query — single-row lookup by `(project_id, commit_id, id)` for efficient per-file patch retrieval

## [0.26.0] - 2026-03-11

### Added
- **Commit history endpoints:** Three new project-scoped, member-level endpoints for browsing git commit history and per-file diffs
  - `GET /v1/projects/{id}/commits` — paginated commit list ordered by `committer_date DESC`, default limit 20, max 100
  - `GET /v1/projects/{id}/commits/{hash}` — single commit detail with parent references and aggregate diff stats (`files_changed`, `total_additions`, `total_deletions`)
  - `GET /v1/projects/{id}/commits/{hash}/diffs` — per-file diffs with optional `?include_patch=true` to fetch unified diff content; default limit 200, max 500
- New `CommitHandler` in `internal/handler/commits.go` with response types, mapping helpers (`toCommitSummary`, `toCommitParents`, `toFileDiff`, `toFileDiffFromMeta`, `parsePagination`, `callerShowEmails`), and 16 unit tests
- New `ListCommitFileDiffsMeta` sqlc query — metadata-only variant of `ListCommitFileDiffs` that omits the `patch` column, used when `include_patch=false` to avoid fetching large diff content
- All endpoints use existing sqlc queries from `commits.sql` — no new migrations needed

## [0.25.0] - 2026-03-11

### Added
- **File structure endpoint:** `GET /v1/projects/{id}/structure` returns a nested file tree built from the active index snapshot — directories sorted before files at each level, includes `snapshot_id`, `git_commit`, `branch`, and `file_count`
- **File context endpoint:** `GET /v1/projects/{id}/files/context?file_path=...` returns file content and metadata (`language`, `size_bytes`, `line_count`, `content_hash`) via LEFT JOIN to `file_contents`; optional `?include_ast=true` includes Tree-sitter AST JSONB
- New `datastore/postgres/queries/files.sql` with 4 sqlc queries: `GetActiveSnapshotForProject`, `ListSnapshotFiles`, `GetFileWithContent`, `GetFileWithContentAndAST`
- `buildFileTree` helper converts flat file path list into nested directory tree with directories-first sorting
- 6 unit tests for `buildFileTree` (empty, single file, nested, deep nesting, sorting, language/size)

## [0.24.1] - 2026-03-10

### Changed
- **Membership events are now personal notifications:** `member:added`, `member:removed`, and `member:role_updated` are delivered only to the targeted user (the user being added/removed/role-updated) instead of being broadcast to all project members
- New `Hub.SendToUser(userID, data)` and `Hub.PublishToUser(userID, evt)` methods route events by `Client.UserID` instead of `Client.ProjectIDs`
- `Subscriber.applyMembershipDelta` now returns the target user ID and handles all three membership event types for routing
- `MembershipHandler` uses `PublishToUser` instead of `Publish` for all three handlers (HandleAdd, HandleUpdate, HandleRemove)
- Updated subscriber and integration tests to verify user-targeted delivery and bystander exclusion

## [0.24.0] - 2026-03-10

### Added
- **Redis pub/sub subscriber:** new `internal/sse/subscriber.go` — subscribes to `myjungle:events` channel and bridges worker job lifecycle events (`job:queued`, `job:started`, `job:progress`, `job:completed`, `job:failed`, `snapshot:activated`) to the SSE Hub
- **Membership SSE events:** `MembershipHandler` now publishes real-time events (`member:added`, `member:removed`, `member:role_updated`) to the SSE Hub after successful mutations — includes `user_id`, `role`, and `actor_user_id` data fields
- Subscriber lifecycle wired in `app.go` — starts on `Run()`, gracefully shuts down on context cancellation; skips silently when `REDIS_URL` is empty
- `go-redis/v9` promoted from indirect to direct dependency
- 8 subscriber unit tests (miniredis-based: message delivery, malformed JSON handling, missing fields, SSE frame format, context cancellation, nil-safe close)
- 4 integration tests (member added/role updated/removed event delivery, non-member event filtering)

## [0.23.0] - 2026-03-09

### Added
- **SSE Hub:** new `internal/sse` package with connection management Hub — tracks concurrent SSE clients, enforces `MaxSSEConnections` capacity limit, broadcasts events filtered by project membership
- **`GET /v1/events/stream`** replaced stub with Hub-aware handler: authenticates user, loads project membership set, registers SSE client, delivers real-time project events with keepalive pings
- **`GET /v1/projects/{projectID}/logs/stream`** new stub endpoint for project activity logs — sends `log:connected` event and keepalives, requires member-level access (dual-auth: user session or API key)
- `Hub.Publish(SSEEvent)` convenience method for API-originated events — JSON-marshals, formats as SSE frame, broadcasts to matching clients; nil-safe for optional Hub wiring
- `Hub.HasCapacity()` for early rejection before expensive DB queries
- `ListUserProjectIDs` SQL query — lightweight project UUID lookup for SSE membership loading
- 11 unit tests (Hub + handler) and 5 integration tests (auth, headers, connected events, non-member rejection, API key access)

## [0.22.0] - 2026-03-09

### Added
- **Queue integration:** `POST /v1/projects/{id}/index` now enqueues newly created jobs to Redis/asynq after persisting the job row in PostgreSQL
- New `internal/queue` package with `JobEnqueuer` interface and asynq `Publisher` implementation
- Queue message conforms to `contracts/queue/workflow-task.v1.schema.json` — carries only `job_id`, `workflow`, `enqueued_at`, and optional `project_id`/`requested_by` (no secrets or config blobs)
- Workflow mapping: `"full"` → `"full-index"`, `"incremental"` → `"incremental-index"`
- `requested_by` observability hint derived from auth context (`user:{id}` or `api-key:{type}`)
- Enqueue failure gracefully marks the job as `failed` with structured error details (`"stage":"enqueue"`) and returns `500` — no jobs left stuck in `queued` status
- Dedup-hit responses (existing active job) do not re-enqueue
- Publisher initialized at app startup from `REDIS_URL`, shut down gracefully on exit; nil-safe when Redis is not configured
- 5 new integration tests covering enqueue success, workflow mapping, dedup skip, failure handling, and failure recovery

### Dependencies
- Added `github.com/hibiken/asynq v0.26.0` (Redis-backed task queue)

## [0.21.0] - 2026-03-09

### Changed
- **Structured logging:** replaced all `log` package usage with `log/slog` across ~25 files (~50 calls) — production logs are now structured JSON by default with leveled output (debug/info/warn/error)
- New `internal/logger` package providing `New(Config)` factory, `FromContext()`/`WithLogger()` context helpers, and unit tests
- `LOG_LEVEL` env var controls minimum log level (default: `info`); `LOG_FORMAT` selects JSON or text output (default: `json`)
- Request logging middleware emits structured fields (`request_id`, `method`, `path`, `status`, `duration`, `error_code`) with level based on status code (5xx→Error, 4xx→Warn, else→Info)
- Config startup summary consolidated from ~40 separate `log.Printf` lines into a single `slog.Info` event with grouped attributes
- `log.Fatalf` replaced with `slog.Error` + `os.Exit(1)` (slog has no Fatal by design)
- Backend worker logging also migrated to `log/slog`
- Zero new dependencies — `log/slog` is Go stdlib since 1.21

## [0.20.0] - 2026-03-09

### Added
- Real database-backed `POST /v1/projects/{id}/index` replacing stub — creates durable `indexing_jobs` row in PostgreSQL with deduplication and provider config pinning
- Real database-backed `GET /v1/projects/{id}/jobs` replacing stub — returns paginated job history ordered by `created_at DESC`
- Job deduplication: repeated equivalent requests (same project + job type) return the existing active job instead of creating duplicates; only `queued`/`running` jobs are considered active
- Provider config pinning: resolved `embedding_provider_config_id` and `llm_provider_config_id` stored on the job at creation time so the worker knows exactly which providers to use
- Pre-flight validation: rejects projects without an active SSH key assignment (409) or without a resolved embedding config (409)
- `indexingJobToMap` serialization helper for `IndexingJob` → JSON response including nullable UUID/timestamp handling
- 14 integration tests: job creation (202), config pinning, dedup queued, dedup running, completed/failed don't block, different projects, different job types, no SSH key (409), no embedding config (409), invalid job_type (422), empty body defaults to full, list jobs, list pagination

### Changed
- **BREAKING:** `handler.NewProjectHandler()` signature changed from `(*postgres.DB, *sshkey.Service)` to `(*postgres.DB, *sshkey.Service, *embedding.Service, *llm.Service)` — requires embedding and LLM services for provider config resolution
- Removed `stubJob()` helper (replaced by real implementation)

## [0.19.0] - 2026-03-08

### Added
- Shared `internal/providers` registry with Phase 1 `ollama` support for both embedding and LLM capabilities
- Authenticated `GET /v1/settings/providers` endpoint returning runtime-supported provider IDs
- Project-scoped embedding and LLM provider settings flows: `GET /v1/projects/{id}/settings/{embedding|llm}/available`, mode-aware `GET/PUT/DELETE /v1/projects/{id}/settings/{embedding|llm}`, `POST /v1/projects/{id}/settings/{embedding|llm}/test`, and `GET /v1/projects/{id}/settings/{embedding|llm}/resolved`
- `internal/llm/service.go` with project setting resolution, availability listing, connectivity testing, and default bootstrap
- Encrypted provider credential storage helper with response-side `has_credentials` redaction
- Focused integration coverage for supported providers, embedding setting lifecycle, embedding connectivity tests, and LLM setting lifecycle

### Changed
- **BREAKING:** project embedding settings now use mode-based responses (`default`, `global`, `custom`) instead of only exposing a custom override
- **BREAKING:** resolved provider responses now report `source` as `custom`, `global`, or `default`
- **BREAKING:** provider config storage migrated from `embedding_config` to `embedding_provider_configs`; added `llm_provider_configs`; projects now store selected global config IDs for both capabilities
- **BREAKING:** `embedding_versions.embedding_config_id` renamed to `embedding_provider_config_id`
- **BREAKING (deployment):** `backend-api` now requires `PROVIDER_ENCRYPTION_SECRET` to be set at startup; `internal/config/config.go` panics if it is absent or empty
- Shared provider validation and Ollama connectivity handling across embedding and LLM services
- Fresh projects now resolve provider settings against seeded defaults at `http://host.docker.internal:11434` instead of returning `404` when no custom config exists
- Added default global LLM bootstrap/config wiring and reserved Global Admin LLM routes returning `global_admin_required` until Phase 2
- Operator action: set `PROVIDER_ENCRYPTION_SECRET` in each deployment environment before upgrading to `0.19.0`

## [0.18.0] - 2026-03-07

### Added
- `email` field on `User` domain model
- `GET /v1/users/lookup?q={query}` endpoint — resolves a username or email to a minimal user profile wrapped in `{"user": {...}}`; tries username first, then email; returns 404 if not found
- `validate.Email()` validator — trims, rejects embedded whitespace and multiple `@`, checks for non-empty local/domain parts, normalizes to lowercase
- `UniqueViolationConstraint()` helper in `postgres` package — extracts constraint name from `pgconn.PgError` for distinguishing username vs email unique violations

### Changed
- `HandleRegister` (`POST /v1/users`) now requires `email` field; returns 409 with specific message for email vs username conflicts
- `HandleUpdateMe` (`PATCH /v1/users/me`) accepts optional `email` field; validates format and handles unique violation
- `dbconv.DBUserToDomain`, `dbconv.SessionRowToUser` now map `Email` field

## [0.17.0] - 2026-03-07

### Added
- `GetProjectWithHealth` sqlc query — enriches `GET /v1/projects/{id}` with per-project health fields: `index_git_commit`, `index_branch`, `index_activated_at`, `active_job_id`, `active_job_status`, `failed_job_id`, `failed_job_finished_at`, `failed_job_type` via `LEFT JOIN LATERAL` subqueries (same pattern as `ListUserProjectsWithHealth`)
- `projectHealthDetailToMap` serializer for project detail with health fields
- 5 integration tests: project detail health fields (never indexed, active index, active job, recent failed job, stale failed job)

### Changed
- `HandleGet` switched from `GetProject` to `GetProjectWithHealth` — response now includes per-project index health fields (nullable, backward-compatible)

## [0.16.0] - 2026-03-06

### Added
- Real database-backed `GET /v1/dashboard/summary` endpoint replacing hardcoded stub — returns `projects_total`, `jobs_active`, `jobs_failed_24h`, `query_count_24h`, `p95_latency_ms_24h` scoped to the authenticated user's projects
- 3 new sqlc queries: `CountActiveJobsForUser`, `CountFailedJobsForUser24h`, `GetQueryStats24hForUser` (p95 via `percentile_cont`)
- `ListUserProjectsWithHealth` sqlc query — enriches `GET /v1/users/me/projects` with per-project health fields: `index_git_commit`, `index_branch`, `index_activated_at`, `active_job_id`, `active_job_status`, `failed_job_id`, `failed_job_finished_at`, `failed_job_type` via `LEFT JOIN LATERAL` subqueries (uses snapshot PK as presence sentinel)
- 13 integration tests: dashboard summary (empty state, unauthenticated, active job, failed job, stale failed job, query stats, multi-user isolation) and project health fields (never indexed, active index, active job, failed job, stale failed job)

### Changed
- **BREAKING:** `handler.NewDashboardHandler()` signature changed from `()` to `(*postgres.DB)` — requires database connection
- `HandleMyProjects` switched from `ListUserProjects` to `ListUserProjectsWithHealth` — response now includes per-project index health fields (nullable, backward-compatible)

### Removed
- `HandleJobs` stub method and `GET /v1/dashboard/jobs` route — job status is now embedded in the project list via health fields

## [0.15.0] - 2026-03-06

### Added
- Root `Makefile` with targets: `test-unit`, `test-integration`, `build`, `lint`, `sqlc-generate`
- Root `.dockerignore` to keep Docker build context lean
- `command: ["--migrate"]` on `backend-api` in Docker Compose for dev auto-migration convenience

### Changed
- **BREAKING (Docker):** `backend-api/Dockerfile` rewritten for Go workspace multi-module build — build context is now repo root (`.`) instead of `./backend-api`
- **BREAKING (Docker):** `backend-worker/Dockerfile` rewritten with same workspace-aware pattern — build context is repo root
- `docker-compose.yaml`: `backend-api` and `backend-worker` build context changed from service directory to `.` with explicit `dockerfile:` path
- `DOCKER.md`: updated build sources section to reflect repo root context

### Fixed
- Docker build for `backend-api` and `backend-worker` — previously broken because `go.work` and `datastore/` module were outside the build context, causing Go workspace module resolution to fail

## [0.14.0] - 2026-03-05

### Added
- Two-tier embedding configuration system: global server defaults (403 in Phase 1) and per-project overrides (fully functional)
- `GET /v1/projects/{id}/settings/embedding` — returns project-level override config or 404 (member+)
- `PUT /v1/projects/{id}/settings/embedding` — creates/replaces project embedding config via immutable append pattern (admin+)
- `DELETE /v1/projects/{id}/settings/embedding` — deactivates project override, falls back to global (admin+)
- `POST /v1/projects/{id}/settings/embedding/test` — tests Ollama connectivity with optional body; if empty, tests resolved config (admin+)
- `GET /v1/projects/{id}/settings/embedding/resolved` — returns effective config with `source` field ("project" or "global") or 404 (member+)
- `internal/embedding/` package: `service.go` (CRUD + resolution + connectivity), `ollama_client.go` (Ollama `/api/tags` connectivity test with 5s timeout), `provider.go` (Phase 1: only `"ollama"`)
- `domain.EmbeddingConfig` model with `ProjectID` field for project-scoped configs
- `dbconv.DBEmbeddingConfigToDomain` conversion helper
- `embedding_config` table extended with nullable `project_id` column and two scoped unique indexes (global + per-project) — merged into `000001_init`
- sqlc queries: `GetActiveGlobalEmbeddingConfig`, `GetActiveProjectEmbeddingConfig`, `DeactivateGlobalEmbeddingConfig`, `DeactivateProjectEmbeddingConfig`, `CreateEmbeddingConfig` (nullable project_id), `CreateEmbeddingVersion`, `GetResolvedEmbeddingConfig` (ORDER BY project_id NULLS LAST)
- Embedding version tracking: each PUT creates an `embedding_versions` row with label `"{provider}-{model}-{random_hex}"`
- 12 integration test functions (44 subtests): global 403, project config lifecycle, validation, connectivity test with/without body, global fallback resolution, immutable-append DB verification, embedding_versions row verification, invalid URL validation, unauthenticated access, auth non-member, auth member read-only, edge-case test endpoint (empty JSON body, whitespace body, userinfo URL, ftp scheme, no-dimensions, oversized body)

### Changed
- **BREAKING:** `handler.NewEmbeddingHandler()` signature changed from `(endpointURL, model string)` to `(*embedding.Service)` — requires embedding service
- **BREAKING:** Global embedding routes (`GET/PUT/POST /v1/settings/embedding`) now return 403 with `global_admin_required` error code (Global Admin role deferred to Phase 2)
- Merged embedding scope migration (`000002_embedding_scope`) into `000001_init` — single migration file with final schema; removed separate `000002` up/down files
- Removed old embedding stub handler and hardcoded responses
- Replaced `TestEmbeddingHandler` stub test with `TestEmbeddingHandler_Global403`

### Fixed
- **TOCTOU race:** `DeleteProjectOverride` now performs existence check inside the transaction (was previously outside)
- **Asymmetric model matching:** Ollama connectivity test now normalizes both sides when comparing with `:latest` suffix (e.g. requesting `model:latest` against Ollama listing `model` now matches correctly)
- **DELETE response:** `DELETE /v1/projects/{id}/settings/embedding` now returns 200 with JSON message body per spec (was 204 No Content)
- **Down migration:** Added comment clarifying golang-migrate wraps each migration file in a transaction automatically
- **Nil-pointer guard:** `TestConnectivity` now returns a clear error when called with a nil `*EmbeddingConfig` instead of panicking
- **URL scheme validation:** `endpoint_url` now rejects non-http/https schemes (e.g. `ftp://`, `file://`)
- **Userinfo redaction:** Ollama connectivity error messages now strip userinfo from endpoint URLs to prevent credential leakage
- **Whitespace handling:** `HandleProjectTest` now trims whitespace when detecting empty request bodies
- **Test determinism:** Integration test `ORDER BY created_at` queries now include `id` tiebreaker; added `rows.Err()` check after iteration
- **DB constraint:** `embedding_config.dimensions` column now has `CHECK (dimensions > 0)` for defence-in-depth
- **URL parse error:** Ollama connectivity test no longer leaks raw URL parse errors (including potential userinfo) in `ConnectivityResult.Message`
- **Version label collision:** Replaced timestamp-based version labels with `crypto/rand` hex suffix to prevent collisions under concurrent updates
- **Body handling:** `HandleProjectTest` now uses bounded `io.LimitReader` and robust JSON empty-object detection instead of brittle string comparison
- **No-panic labels:** `versionLabel` returns `(string, error)` instead of panicking on `rand.Read` failure; callers propagate the error
- **413 status:** Oversized `/test` payloads now return 413 (`domain.ErrPayloadTooLarge`) instead of 400
- **redactURL safety:** Returns `<invalid URL>` placeholder on parse failure instead of leaking the raw input string
- **Whitespace-only body:** `HandleProjectTest` trims whitespace before JSON probe so whitespace-only bodies are treated as empty
- **Transport error leakage:** Ollama connectivity error now sanitizes `*url.Error` from `http.Client.Do` to strip embedded URL (including potential userinfo) from transport error messages
- **Service-level validation:** `UpdateGlobalConfig` and `UpdateProjectConfig` now validate all fields (endpoint_url, model, dimensions) at the service boundary, not just the provider — prevents non-HTTP callers from bypassing invariants
- **Validate normalization:** Handler `validate()` now captures trimmed return values from `validate.Required` and `validate.URL`, ensuring normalized values are used downstream
- **Dynamic provider list:** `ValidateProvider` error message now dynamically lists supported providers from the map instead of hardcoding `"ollama"`
- **Input normalization:** `validateUpdateRequest` now trims whitespace and lowercases provider before validation and persistence, preventing whitespace-padded or case-mismatched values from reaching the database
- **Credential rejection:** `endpoint_url` validation now rejects URLs containing userinfo (e.g. `http://user:pass@host`) to prevent credentials from being stored in the config
- **TestConnectivity normalization:** Provider string is now trimmed and lowercased before validation and switch dispatch, consistent with the update path
- **Handler userinfo check:** Handler-level `validate()` and `validateForTest()` now reject `endpoint_url` with userinfo (was only checked at service level, inconsistent defence-in-depth)
- **Test endpoint dimensions:** `POST /test` with explicit body no longer requires `dimensions` field — connectivity test does not use dimensions, so requiring them was unnecessary friction
- **TOCTOU in delete:** `DeleteProjectOverride` now uses `:execrows` row count from the UPDATE instead of a separate SELECT+UPDATE, eliminating the TOCTOU window under READ COMMITTED isolation
- **DRY transaction logic:** Extracted shared `upsertConfigTx` helper from `UpdateGlobalConfig` and `UpdateProjectConfig` to deduplicate the immutable-append transaction pattern
- **Nil-pointer guard in handler:** `HandleProjectResolved` and `HandleProjectTest` now check that `GetResolvedConfig` did not return nil before dereferencing `resolved.Config`
- **Conflict on concurrent upsert:** `upsertConfigTx` now detects unique-constraint violations (23505) on `CreateEmbeddingConfig` and returns 409 Conflict instead of a generic internal error
- **Variable shadowing:** Renamed local `url` to `tagsURL` in `testOllamaConnectivity` to avoid shadowing the `net/url` package import
- **Explicit HTTP client:** App wiring now passes an explicit `*http.Client{Timeout: 10s}` to `embedding.NewService` instead of relying on the nil-default fallback
- **Order-independent test:** `TestEmbedding_ImmutableAppend` now matches config rows by ID instead of relying on `ORDER BY` position

## [0.13.2] - 2026-03-05

### Fixed
- **Race condition:** `CreateProjectKey` and `CreatePersonalKey` count+insert now wrapped in a transaction with `SELECT ... FOR UPDATE` row lock on the parent (project or user) to prevent concurrent over-limit creation
- Expired keys now excluded from `ListProjectKeys`, `ListPersonalKeys`, `CountProjectKeys`, `CountPersonalKeys`, and `UpdateAPIKeyLastUsed` queries via `AND (expires_at IS NULL OR expires_at > NOW())`
- `keygen_test.go`: length assertion now uses `t.Fatalf` (stops test) before any slicing of `plaintext` to prevent panic on short output
- Dynamic future timestamp in `TestAPIKey_CreateWithValidExpiresAt` (was hardcoded `2030-01-01`)
- Alphabetical import ordering in `app.go` (moved `apikey` before `config`)

### Added
- `LockProjectRow` and `LockUserRow` sqlc queries for transaction-safe key limit enforcement

### Changed
- Merged `000002_api_keys_v2` migration into `000001_init` — single migration file with final dual-type API key schema; removed separate `000002` up/down files
- `sqlc.yaml` schema list reduced to single `000001_init.up.sql` entry

## [0.13.1] - 2026-03-05

### Fixed
- **Security:** `key_hash` index is now UNIQUE — prevents duplicate key hash collisions
- **Security:** `api_key_project_access` view now fail-closed on unknown roles — unknown role values produce no row (access denied) instead of defaulting to `'read'`
- **TOCTOU fix:** `DeleteProjectKey` and `DeletePersonalKey` now wrap read-check-delete in a single transaction via `db.WithTx`
- Migration data migration gap: existing rows are migrated from `api_key_projects` junction table before adding CHECK constraint (prevents constraint violations during upgrade)
- `ListProjectKeys` query now includes explicit `key_type = 'project'` filter (defense-in-depth)
- `UpdateAPIKeyLastUsed` query now filters `is_active = TRUE` (prevents touching soft-deleted keys)
- Service-layer role validation: `CreateProjectKey` and `CreatePersonalKey` reject invalid role values before reaching the database

### Added
- Key count limits: max 50 project keys per project, max 50 personal keys per user (prevents resource exhaustion)
- `CountProjectKeys` and `CountPersonalKeys` sqlc queries for limit enforcement
- `dbconv.TimeToPgTimestamptz` helper (moved from service-local function)
- `role_rank()` SQL helper function for role comparison (extracted from inline CASE)
- `TestGenerateAPIKey_HashRoundTrip` unit test — verifies `HashKey(plaintext)` matches hash from `GenerateAPIKey`
- 5 new integration tests: non-member list (404), valid `expires_at`, double-delete project key (404), double-delete personal key (404), delete nonexistent key (404)

## [0.13.0] - 2026-03-05

### Added
- Dual-type API key management: **project keys** (scoped to one project) and **personal keys** (scoped to all user memberships)
- `POST /v1/projects/{projectID}/keys` — admin+ creates project-scoped API key (201, plaintext shown once)
- `GET /v1/projects/{projectID}/keys` — member+ lists project keys (without plaintext)
- `DELETE /v1/projects/{projectID}/keys/{keyID}` — admin+ soft-deletes project key (204)
- `POST /v1/users/me/keys` — creates personal API key (201, plaintext shown once)
- `GET /v1/users/me/keys` — lists personal keys for authenticated user
- `DELETE /v1/users/me/keys/{keyID}` — soft-deletes personal key (ownership verified)
- `internal/apikey/` package: `keygen.go` (crypto/rand key generation, SHA-256 hashing), `service.go` (CRUD for both key types)
- Key format: `mj_proj_<32 base62 chars>` for project keys, `mj_pers_<32 base62 chars>` for personal keys
- `domain.APIKeyInfo` model with key type/role constants (`KeyTypeProject`, `KeyTypePersonal`, `KeyRoleRead`, `KeyRoleWrite`)
- `dbconv.DBAPIKeyToDomain` conversion helper
- Migration `000002_api_keys_v2`: adds `key_type`, `name`, `project_id` columns to `api_keys`; drops `api_key_projects` junction table; recreates `api_key_project_access` view as UNION ALL (project keys via direct FK, personal keys via `project_members` JOIN with `LEAST(role_rank)` effective role)
- CHECK constraint enforces `(key_type='project' AND project_id IS NOT NULL) OR (key_type='personal' AND project_id IS NULL)`
- sqlc queries: `CreateAPIKey`, `ListProjectKeys`, `ListPersonalKeys`, `GetAPIKeyByID`, `SoftDeleteAPIKey`, `CheckAPIKeyProjectAccess`, `UpdateAPIKeyLastUsed`, `GetAPIKeyByHash`
- 14 integration tests covering RBAC (admin creates, member forbidden), ownership isolation, cross-project boundary, input validation (invalid role, expired date, name too long), default role

### Changed
- **BREAKING:** `handler.NewAPIKeyHandler()` signature changed from `()` to `(*apikey.Service)` — requires API key service
- **BREAKING:** API key routes restructured from flat `/v1/keys` to dual `/v1/users/me/keys` (personal) and `/v1/projects/{projectID}/keys` (project-scoped with RBAC)
- Removed `api_key_projects` junction table (replaced by direct `project_id` FK on project keys)
- Removed old API key sqlc queries from `keys_and_settings.sql` (moved to dedicated `api_keys.sql`)
- Removed `TestAPIKeyHandler` stub test (replaced by integration tests)

## [0.12.4] - 2026-03-05

### Added
- `POST /v1/projects/{id}/members` endpoint — admin+ can add members directly; admin cannot assign owner role (owner-only); validates target user exists and is active; returns 409 on duplicate membership
- `membership.Service.Add()` method with role hierarchy enforcement consistent with `UpdateRole`

## [0.12.3] - 2026-03-05

### Fixed
- **TOCTOU race condition:** `UpdateRole` and `Remove` now wrap the entire read-check-mutate sequence inside a single transaction with `SELECT ... FOR UPDATE` on the target membership row, preventing concurrent role changes from bypassing permission checks or the last-owner invariant
- **Defense-in-depth:** `UpdateRole` now explicitly verifies actor is admin+ before proceeding (previously relied solely on route middleware)

### Changed
- Extracted `parseUUIDParam(paramName, fieldName)` helper in `MembershipHandler`, replacing duplicated `parseProjectID` / `parseUserID` methods
- Updated `ProjectMemberWithUser` doc comment to clarify it is a list-response projection (not an extension of `ProjectMember`)

### Added
- `GetProjectMemberForUpdate` sqlc query — `SELECT ... FOR UPDATE` on a single membership row for transaction-safe role checks
- `TestMembership_OwnerDemotionRemovalRace` — concurrent race test verifying at most one of two simultaneous owner demotions succeeds (5 iterations)

## [0.12.2] - 2026-03-04

### Fixed
- Removed duplicate `TestMembership_AdminCannotModifyOwner` (identical to `AdminCannotDemoteOwner`)
- Added `display_name` field assertion in `TestMembership_ListMembers`
- Added empty-role `{"role": ""}` test case in `TestMembership_InvalidRole`

## [0.12.1] - 2026-03-04

### Fixed
- **Race condition:** concurrent owner demotions/removals could leave a project with zero owners; `CountProjectOwnersForUpdate` now acquires row-level locks (`FOR UPDATE OF pm`) inside transactions to serialize these operations
- Restructured `TestMembership_CannotDemoteLastOwner` into two focused tests: `TestMembership_AdminCannotDemoteOwner` (403) and `TestMembership_OwnerDemotesOwnerSucceedsWhenMultiple` (200)

### Added
- `CountProjectOwnersForUpdate` sqlc query — counts owners with `FOR UPDATE` row locking for transaction safety
- `TestMembership_RemoveTargetNotMember` — verifies DELETE on non-existent member returns 404
- Expanded `TestMembership_ListMembers` to verify `id`, `project_id`, `user_id`, `created_at` fields in response

## [0.12.0] - 2026-03-04

### Added
- Project membership management endpoints with real database-backed implementations
- `GET /v1/projects/{id}/members` — lists members with joined user info (username, display_name, avatar_url)
- `PATCH /v1/projects/{id}/members/{user_id}` — updates member role with full RBAC enforcement
- `DELETE /v1/projects/{id}/members/{user_id}` — removes member or allows self-removal (leave)
- `internal/membership/` package with `Service` implementing business rules and ownership invariants
- `domain.ProjectMemberWithUser` model for list responses with joined user data
- `dbconv.DBMemberWithUserToDomain` converter for `ListProjectMembersRow` → domain model
- Role hierarchy enforcement: owner can set any role; admin can set admin/member but not owner; member cannot change roles
- Ownership invariants: last owner cannot be demoted or removed; self-role-change rejected; admin cannot modify owner
- `CountProjectOwners` checked within transactions to mitigate race windows on owner demotion/removal (definitive fix via `CountProjectOwnersForUpdate` with row-level locking in 0.12.1)
- Self-removal support: members can leave projects via DELETE on their own membership (moved route to member-level group)
- 16 integration tests covering all RBAC scenarios: list, promote, demote, remove, self-remove, last-owner protection, insufficient role, invalid role, target not found

### Changed
- **BREAKING:** `handler.NewMembershipHandler()` signature changed from `()` to `(*postgres.DB)` — requires database connection
- `DELETE /v1/projects/{id}/members/{user_id}` route moved from admin-level to member-level group (service enforces authorization for removing others)
- Removed `TestMembershipHandler` stub test from `stubs_test.go` (replaced by integration tests)

## [0.11.1] - 2026-03-04

### Fixed
- Status validation now accepts `"active"` or `"paused"` (was incorrectly allowing `"archived"`)
- Pagination max limit changed from 100 to 200 per spec
- `repo_url` validation now accepts SCP-style SSH URLs (`git@github.com:org/repo.git`) via new `validate.RepoURL()` function

### Added
- `validate.RepoURL()` — accepts `git@` SCP-style and standard `https://`/`ssh://` URLs
- 8 new integration tests: SCP-style URL, empty name, name too long, invalid repo URL, invalid status on update, empty PATCH body, pagination cap, cross-user SSH key rejection

## [0.11.0] - 2026-03-04

### Added
- Project CRUD endpoints: create, list (paginated), get, update (partial), delete
- SSH key management per project: get assigned key, reassign existing key, generate-and-assign new key, remove assignment
- `POST /v1/projects` creates project in atomic transaction: verify SSH key → insert project → add creator as owner → assign key; returns project with `ssh_key` summary
- `GET /v1/projects` supports `limit`/`offset` pagination with `total` count
- `PATCH /v1/projects/{id}` partial update using COALESCE pattern (name, repo_url, default_branch, status)
- `DELETE /v1/projects/{id}` hard-deletes project (owner only)
- `GET /v1/projects/{id}/ssh-key` returns assigned key details (member+)
- `PUT /v1/projects/{id}/ssh-key` supports reassign (`ssh_key_id`) and generate (`generate: true, name`) modes (admin+)
- `DELETE /v1/projects/{id}/ssh-key` deactivates key assignment, returns 204 (admin+)
- `domain.Project` and `domain.SSHKeySummary` models
- `dbconv.DBProjectToDomain` and `dbconv.DBSSHKeySummaryToDomain` conversion helpers
- `CountProjectsByMember` sqlc query for pagination total
- `GetProjectSSHKeyWithDetails` sqlc JOIN query returning key details for active assignment
- Integration tests: 16 tests covering create (with defaults, custom branch/status, validation, invalid key, unauthenticated), list (default, pagination), get (success, not found), update (partial, full), delete, SSH key (get, reassign, generate, remove)

### Changed
- **BREAKING:** `handler.NewProjectHandler()` signature changed from `()` to `(*postgres.DB, *sshkey.Service)` — requires database and SSH key service
- `CreateProject` sqlc query now includes `created_by` parameter and uses `sqlc.narg` for optional `default_branch` and `status`
- Removed `stubProject()` helper and all project stub responses; replaced with real database-backed implementations
- Stub unit tests in `stubs_test.go` updated to only cover remaining stub methods (index, jobs, search, symbols, etc.)

## [0.10.0] - 2026-03-04

### Added
- SSH key CRUD endpoints with Ed25519 key pair generation and AES-256-GCM encrypted private key storage
- Private key upload support: `POST /v1/ssh-keys` accepts optional `private_key` PEM field — server parses the key, derives public key and fingerprint, detects key type (Ed25519, RSA, ECDSA)
- `ParsePrivateKey()` function supporting Ed25519, RSA, and ECDSA keys with passphrase-protected key rejection
- `internal/sshkey/` package: `keygen.go` (key generation + parsing), `encrypt.go` (AES-256-GCM with SHA-256 key derivation), `service.go` (`Create` and `CreateFromPrivateKey` methods)
- `domain.SSHKey` model (intentionally excludes private key from API responses)
- `dbconv.DBSSHKeyToDomain` conversion helper
- `CountActiveAssignmentsByKey` sqlc query for retire pre-check (409 Conflict if key is still assigned)
- Unit tests: keygen (generate + parse Ed25519/RSA/ECDSA/invalid/passphrase), encrypt, service, dbconv
- Integration tests: 14 tests covering generate, upload, validation, invalid PEM (400), duplicate fingerprint (409), anonymous access, list, user-scoping, get, cross-user 404, list-projects, retire, cross-user retire 404, no-private-key-in-responses

### Changed
- **BREAKING:** `handler.NewSSHKeyHandler()` signature changed from `()` to `(*postgres.DB, *sshkey.Service)` — requires database and SSH key service
- All SSH key sqlc queries now enforce user-scoping via `created_by` parameter (`CreateSSHKey`, `GetSSHKey`, `ListSSHKeys`, `RetireSSHKey`, `ListProjectsBySSHKey`)
- `ListProjectsBySSHKey` adds JOIN on `ssh_keys` table for defense-in-depth ownership verification
- Removed `stubSSHKey()` helper from `handler/project.go`; project SSH key endpoints (`HandleGetSSHKey`, `HandleSetSSHKey`) now return `"not yet implemented"` stub (planned for future release)

## [0.9.0] - 2026-03-04

### Added
- Integration test infrastructure using testcontainers-go with `postgres:16-alpine`
- `tests/integration/setup_test.go`: `TestMain` with container lifecycle, `setupTestApp`, `truncateAll`, `doRequest`, `decodeJSON`, `registerUser`, `loginUser`, `authHeader` helpers
- `tests/integration/health_test.go`: 6 tests — liveness always 200, readiness up/down, Prometheus metrics, no-auth-required, liveness ignores DB state
- `tests/integration/user_test.go`: 12 tests — register (with defaults, duplicate, normalization, missing username), login/logout, session auth (Bearer token, cookie, invalid token), profile update (display_name, avatar_url, empty payload, blank name), list projects, multiple sessions independence
- `//go:build integration` tag isolates tests from `go test ./...`
- `TEST_POSTGRES_DSN` environment variable for CI override (skips testcontainers)

### Fixed
- `UpdateUserProfile` SQL query: added `::text` cast to `$3` parameter to prevent `could not determine data type of parameter` error when avatar_url is NULL (untyped)

## [0.8.0] - 2026-03-04

### Added
- `internal/health/` package with pluggable `Checker` interface, `CheckResult` type, and per-dependency health probes
- `PostgresChecker` pings the database pool with a 2-second timeout; returns `"skipped"` when DB is nil (test mode)
- `StubChecker` for `redis`, `parser_sidecar`, and `ollama` — returns `"skipped"` until those integrations are wired
- `internal/metrics/` package with `Collector` for Prometheus-compatible text exposition
- `Collector.RecordRequest(method, path, status)` accumulates per-route request counters (mutex-protected)
- `Collector.Render()` produces `myjungle_api_uptime_seconds` gauge and `myjungle_api_requests_total{method,path,status}` counter
- `middleware.Metrics(collector)` middleware records request counts via `statusRecorder`
- Readiness probe tests: all-up (200), dep-down (503), skipped-does-not-fail (200), no-checkers (200)

### Changed
- **BREAKING:** `handler.NewHealthHandler()` signature changed from `(time.Time)` to `(time.Time, []health.Checker, *metrics.Collector)`
- `GET /health/live` now includes `"version": "0.1.0"` in response
- `GET /health/ready` iterates registered health checkers; returns 200 with `"status":"ready"` when all deps are up/skipped, 503 with `"status":"degraded"` when any dep is `"down"`; response includes `"checks"` map with per-dependency status, latency, and error
- `GET /metrics` delegates to `Collector.Render()`: renamed metric from `myjungle_uptime_seconds` to `myjungle_api_uptime_seconds`; adds `myjungle_api_requests_total` counter
- Middleware chain extended: `Metrics` middleware added between `Logging` and `Recover`
- Moved `formatSeconds` helper from `handler/health.go` to `metrics/collector.go`

## [0.7.1] - 2026-03-04

### Changed
- **BREAKING:** `IdentityResolver` no longer accepts `X-Username` header — identity resolution now uses only session tokens (Bearer header or cookie); clients still sending `X-Username` should migrate to session-based auth via `POST /v1/auth/login`
- **BREAKING:** `X-Username` removed from CORS `Access-Control-Allow-Headers` — preflight requests including this header will no longer pass
- `IdentityResolver` signature changed from `(pdb, cookieName)` to `(pdb, SessionConfig)` — cookie clearing now mirrors login/logout session attributes (Secure, SameSite)

### Fixed
- `HandleLogout` removes dead `ErrNoRows` branch — `DeleteSession` is `:exec` (uses `Exec()`) which never returns `pgx.ErrNoRows`; any non-nil error is now treated as internal
- `writeMiddlewareError` nil-AppError fallback now uses `domain.ErrInternal` (consistent JSON content-type instead of `text/plain`)
- `HasBearerScheme` rejects over-matches like `"BearerX ..."` (tightened to exact `"Bearer"` or `"Bearer "` prefix)
- `IdentityResolver` Bearer path returns 500 on backend DB errors instead of masking them as 401
- `IdentityResolver` Cookie path returns 500 on backend DB errors instead of clearing a valid cookie and falling through
- Token tests (`TestGenerateSessionToken_Unique`, `TestHashToken_MatchesGenerate`) now check `GenerateSessionToken` errors instead of discarding them

## [0.7.0] - 2026-03-04

### Added
- Server-side session authentication with opaque random tokens (32 bytes, SHA-256 hashed before storage)
- `POST /v1/auth/login` (public) — authenticates by username, creates session, returns token + sets HttpOnly cookie
- `POST /v1/auth/logout` (authenticated) — destroys session, clears cookie, returns 204
- `sessions` table (merged into `000001_init`) with token hash, expiry, IP address, user agent tracking
- sqlc queries: `CreateSession`, `GetSessionByTokenHash` (joins users, filters expired/inactive), `DeleteSession`, `DeleteUserSessions`, `DeleteExpiredSessions`
- `auth.GenerateSessionToken()` — cryptographic random token generation with SHA-256 hashing
- `auth.HashToken()` — deterministic SHA-256 hex digest for token storage
- `auth.ExtractBearerToken()` — parses `Authorization: Bearer <token>` header (case-insensitive)
- `dbconv.SessionRowToUser()` — converts joined session+user query row to domain User
- `SessionConfig` in config: `SESSION_TTL` (default 24h), `SESSION_COOKIE_NAME` (default "session"), `SESSION_SECURE_COOKIE` (default false)

### Changed
- **BREAKING:** `IdentityResolver` middleware signature changed from `(pdb)` to `(pdb, cookieName)` — now resolves identity in priority order: `Authorization: Bearer` > session cookie > `X-Username` header (legacy fallback)
- Bearer token present but invalid/expired → hard 401 (no fallthrough to other methods)
- Invalid/expired session cookie → cleared (`MaxAge: -1`) and continues as anonymous
- Login does NOT auto-register — unknown username returns 401 `"invalid credentials"`
- Inactive users cannot log in (401)

## [0.6.0] - 2026-03-03

### Added
- `internal/auth/` package with context helpers: `ContextWithUser`, `UserFromContext`, `ContextWithMembership`, `MembershipFromContext`
- `IdentityResolver` middleware resolves user from `X-Username` header via database lookup on every request
- `RequireUser` middleware rejects anonymous requests with 401 (applied to authenticated routes)
- `RequireProjectRole` middleware enforces project-scoped RBAC with role hierarchy: owner > admin > member
- `domain.ProjectMember` model with role constants (`RoleOwner`, `RoleAdmin`, `RoleMember`) and hierarchy helpers (`RoleRank`, `RoleSufficient`)
- `postgres.IsUniqueViolation` helper for detecting PostgreSQL unique constraint errors
- Conversion helpers: `DBUserToDomain`, `DBMemberToDomain`, `PgUUIDToString`, `StringToPgUUID`

### Changed
- **BREAKING:** `handler.NewUserHandler()` now requires `*postgres.DB` parameter
- `HandleRegister` (POST /v1/users) now writes to PostgreSQL via `CreateUser` sqlc query; uses `validate.Required` (returns 422 instead of 400 for missing username)
- `HandleGetMe` (GET /v1/users/me) reads user from request context (set by IdentityResolver) instead of parsing X-Username header directly
- `HandleUpdateMe` (PATCH /v1/users/me) now functional: updates `display_name` and `avatar_url` via `UpdateUserProfile` sqlc query with URL validation
- `HandleMyProjects` (GET /v1/users/me/projects) now functional: queries `ListUserProjects` and returns projects with roles
- Route protection applied per ADR-018 protection matrix: health and user registration are public; user profile, projects (collection), SSH keys, API keys, settings, dashboard, and events require authentication; project-scoped routes enforce member/admin/owner role via middleware
- Middleware chain extended: `IdentityResolver` added after `BodyLimit`
- Removed in-memory user store (sync.RWMutex, maps) in favor of PostgreSQL-backed implementation
- Regenerated sqlc code to match Phase 1 schema (username/display_name/avatar_url) and project-member queries

## [0.5.0] - 2026-03-03

### Added
- `Details any` field on `AppError` (omitted from JSON when nil) for structured validation errors
- `ErrUnauthorized` (401) and `ErrPayloadTooLarge` (413) sentinel errors
- Convenience constructors: `NotFound()`, `Conflict()`, `Forbidden()`, `BadRequest()`, `Unauthorized()`, `ValidationError()`
- New `internal/validate` package with batch error accumulation (`Errors` type) and validators: `Required`, `UUID`, `MinMax`, `OneOf`, `URL`, `MaxLength`
- `WriteAppError(w, err)` response helper — extracts `*AppError` via `errors.As`, falls back to 500; sets `X-Error-Code` response header for structured logging
- `DecodeJSON(w, r, dst)` helper — applies `MaxBytesReader` (1 MB), decodes JSON, returns 413/400 on failure
- `BodyLimit` middleware wraps all request bodies with `http.MaxBytesReader` (belt-and-suspenders with `DecodeJSON`)
- `Access-Control-Max-Age: 86400` on CORS preflight responses
- `Access-Control-Expose-Headers: X-Request-ID` on all responses
- Error code appended to log line (`code=xxx`) when `X-Error-Code` response header is present

### Changed
- `NotFound()` and `MethodNotAllowed()` response helpers now use `WriteAppError`, adding `"code"` field to JSON output
- `Recover` middleware encodes `domain.ErrInternal` directly (adds `"code":"internal_error"` to panic responses)
- `handler/user.go` migrated from inline `MaxBytesReader`/`WriteError` to `DecodeJSON`/`WriteAppError`
- `handler/event.go` migrated from `WriteError` to `WriteAppError`
- Middleware chain: added `BodyLimit(1<<20)` after CORS

## [0.4.0] - 2026-03-03

### Added
- PostgreSQL connection pool via `pgxpool` in `internal/storage/postgres/` package
- sqlc query layer integration via Go workspace (`go.work`) linking `backend-api`, `backend-worker`, and `datastore` modules
- Transaction helper `DB.WithTx(ctx, fn)` for atomic multi-query operations
- Migration runner using `golang-migrate` with embedded SQL files from `datastore/postgres/migrations/`
- `--migrate` CLI flag for dev convenience: `go run ./cmd/api --migrate` applies pending migrations before starting
- Database connection logged with pool stats (acquired/idle/total/max) on success
- 5-second connect timeout prevents startup from hanging when Postgres is unreachable

### Changed
- **BREAKING:** `app.New()` signature changed from `*App` to `(*App, error)` to propagate database connection errors
- `App` struct gains `DB *postgres.DB` field; pool is closed during graceful shutdown
- `config.LoadForTest()` sets `Postgres.DSN` to empty string (unit tests skip DB connection)
- `go.mod` gains `pgx/v5` and `golang-migrate/v4` dependencies

## [0.3.0] - 2026-03-03

### Added
- Full typed configuration layer with 9 sub-structs: Server, Postgres, Redis, Parser, SSH, Embedding, Indexing, Jobs, Events
- `config.LoadForTest()` helper with sensible test defaults (no env vars needed)
- Required env var validation: `POSTGRES_DSN` and `SSH_KEY_ENCRYPTION_SECRET` panic with descriptive message if missing
- Port validation (1-65535) and timeout validation (must be positive where required)
- Config summary logged at startup with secrets redacted (`SSH_KEY_ENCRYPTION_SECRET` always `REDACTED`, Postgres DSN password masked, Redis URL userinfo stripped)
- `redactDSN()` helper masks password in both URL-style and key-value-style Postgres connection strings
- `redactURL()` helper strips userinfo from Redis URLs for safe logging
- `parseBool()` accepts `yes`/`y`/`on` (and `no`/`n`/`off`) in addition to `strconv.ParseBool` values
- `SERVER_READ_HEADER_TIMEOUT` validated as positive (prevents disabling header-read protection)
- New parse helpers: `parseInt()`, `parseDuration()`, `requiredEnv()`
- Default constants centralized in `internal/config/defaults.go`

### Changed
- **BREAKING:** `Config` struct restructured from flat fields to nested sub-structs (e.g. `cfg.ServerPort` → `cfg.Server.Port`)
- **BREAKING:** `Config.Server.Port` is now `int` (was `string`)
- CORS fields moved from top-level to `Config.Server.CORSAllowedOrigins` and `Config.Server.CORSWildcard`
- Server timeouts (read, write, idle, shutdown) now configurable via env vars instead of hardcoded
- Embedding handler receives Ollama URL/model from `Config.Embedding` instead of reading env vars directly
- Removed duplicated `envOrDefault()` from `internal/app/app.go`

## [0.2.4] - 2026-03-03

### Fixed
- Add `t.Cleanup(cancel)` to SSE context-cancel test to prevent goroutine leak on early `t.Fatal` exit

## [0.2.3] - 2026-03-03

### Fixed
- Replace `time.Sleep` in SSE context-cancel test with race-free `firstWriteRecorder` using `sync.Once` channel signal
- Use `log.Writer()` to capture/restore previous log writer instead of hardcoding `os.Stderr` in logging and recover tests

## [0.2.2] - 2026-03-03

### Fixed
- Remove accidentally committed 8.9 MB Go binary (`backend-api/api`) from git tracking
- Add `**/out/` to `.gitignore` for build output directory

## [0.2.1] - 2026-03-03

### Fixed
- Fix `containsWord` test helper to use proper comma-separated token matching instead of substring scan
- Replace custom `discardWriter` type with stdlib `io.Discard` in recover and logging tests
- Add serial-run comments to tests that mutate global `log` output
- Consolidate duplicate `noFlushWriter` / `noFlushResponseWriter` into shared `internal/testutil` package

## [0.2.0] - 2026-03-03

### Added
- Comprehensive unit tests for all internal packages (14 test files, ~76 test functions)
- Test coverage: config 100%, domain 100%, middleware 100%, handler 95.5%, app 79.3%
- Table-driven tests for config parsing, username normalization, health metrics formatting, sentinel errors, and stub handlers
- Concurrency safety test for UserHandler registration (race detector verified)
- SSE event handler tests including no-flusher fallback and context cancellation
- Route integration tests verifying all endpoints, 404/405 JSON responses, and middleware chain

## [0.1.0] - 2026-03-03

### Added
- Project bootstrap with chi router, internal/ package layout, and dependency injection
- Middleware chain: X-Request-ID, request logging, panic recovery, CORS
- Handler stubs for all /v1 routes (projects, SSH keys, API keys, users, settings, dashboard, events)
- Domain error types and JSON response helpers
- Graceful shutdown with signal handling; App.Run() returns error on server failure
- Slim cmd/api/main.go entrypoint (~15 lines)
- 1 MB body size limit (http.MaxBytesReader) in HandleRegister
- Client-supplied X-Request-ID validated against UUID format; regenerated if invalid
- 405 responses include Allow header derived from registered routes (RFC 7231 §6.5.5)

### Changed
- Replaced net/http.ServeMux with github.com/go-chi/chi/v5 router
- Decomposed 739-line main.go into 20 files across internal/ packages
- Replaced manual UUID generation with github.com/google/uuid
- All existing route paths and stub responses preserved
- **BREAKING:** CORS wildcard is now opt-in via `CORS_WILDCARD=true` env var (accepts common truthy values: "true", "1", "t", "yes"). `CORS_ALLOWED_ORIGINS` defaults to empty instead of `*`; set `CORS_WILDCARD=true` to restore the previous open-access behavior
- Config-driven CORS origin allow-list via `CORS_ALLOWED_ORIGINS` env var
- X-Request-ID added to CORS allowed headers

### Fixed
- Replace log.Fatalf in goroutine with error channel for graceful server shutdown
- Add ReadTimeout (5s) and IdleTimeout (120s) to HTTP server; leave WriteTimeout at 0 for SSE
- Make SendSSE return error; handle write failures in SSE keepalive loop
- Add nil-base guard in domain.Errorf to prevent panic on nil sentinel
- Normalize X-Username header before emptiness check in HandleGetMe
- Use handler.MethodNotAllowed helper in 405 route handler (consistent JSON errors)
- Fix nondeterministic timestamps in stub helpers (single time.Now per call)
