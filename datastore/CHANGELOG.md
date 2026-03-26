# Changelog

## [Unreleased]

### Added
- Migration `000003_add_worker_id`: adds `worker_id TEXT` column to `indexing_jobs` table for worker heartbeat tracking
- `FailStaleJob` sqlc query — transitions a running job to failed with error details (used by lazy job reaper)
- `DeleteOrphanedBuildingSnapshot` sqlc query — deletes building/inactive snapshots orphaned by dead workers
- Updated `ClaimQueuedIndexingJob` to accept `worker_id` parameter (stamps worker identity on claim)
- Added `active_job_worker_id` and `active_job_started_at` to `GetProjectWithHealth`, `ListUserProjectsWithHealth`, and `ListAllProjectsWithHealth` LATERAL join queries

## [0.11.0] - 2026-03-21

### Added
- 10 new sqlc queries in `symbols.sql` for symbol list, search, detail, and count operations:
  - `SearchSymbolsByName` — ILIKE pattern search with exact-match sort priority
  - `SearchSymbolsByKind` — exact kind filter
  - `SearchSymbolsByNameAndKind` — combined name pattern + kind filter
  - `ListAllSymbols` — unfiltered paginated list ordered by file path and line
  - `GetSymbolByID` — single symbol with joined file metadata (file_path, language)
  - `ListSymbolChildren` — child symbols by parent_symbol_id ordered by start_line
  - `CountSymbolsBySnapshot`, `CountSymbolsBySnapshotAndName`, `CountSymbolsBySnapshotAndKind`, `CountSymbolsBySnapshotNameAndKind` — per-filter-combination counts for pagination

## [0.10.0] - 2026-03-21

### Added
- `idx_deps_target` partial index on `dependencies(project_id, index_snapshot_id, target_file_path)` for reverse dependency lookups
- 7 new sqlc queries in `dependencies.sql`: `ListDependenciesFromFile`, `ListDependenciesToFile`, `ListExternalDependencies`, `ListFileDependencyCounts`, `CountFilesWithDependencies`, `ListDependenciesFromFiles` (batch), `ListDependenciesToFiles` (batch)

## [0.9.0] - 2026-03-19

### Added
- `max_tokens INT NOT NULL DEFAULT 8000 CHECK (max_tokens >= 1 AND max_tokens <= 131072)` column on `embedding_provider_configs` — configurable maximum token budget for embedding input truncation, replacing the previously hardcoded limit
- Updated `CreateEmbeddingProviderConfig` and `CreateEmbeddingDefaultProviderConfigIfMissing` sqlc queries to include `max_tokens` parameter
- Updated `ListAvailableGlobalEmbeddingProviderConfigs` query to include `max_tokens` in the explicit SELECT list

## [0.8.0] - 2026-03-18

### Added
- `GetEmbeddingVersionByLabel` sqlc query — looks up an embedding version by its deterministic version label (worker embedding version resolution)
- `InsertFile` sqlc query — inserts a file row scoped by project and snapshot, linked to deduplicated `file_contents` via `file_content_id`
- `InsertSymbol` sqlc query — inserts a symbol row with optional parent linking via `parent_symbol_id`
- `InsertCodeChunk` sqlc query — inserts a code chunk row linked to file and optionally to a symbol
- `InsertDependency` sqlc query — inserts a dependency row capturing import relationships between files

## [0.7.0] - 2026-03-12

### Added
- `GetIndexingJob` sqlc query — loads a single indexing job by ID (worker execution context loading)
- `GetProjectSSHKeyForExecution` sqlc query — single JOIN across `projects`, `project_ssh_key_assignments`, and `ssh_keys` returning SSH key fields (id, name, public_key, private_key_encrypted, key_type, fingerprint) plus project repo_url and default_branch; filters for active assignment and active key

## [0.6.0] - 2026-03-09

### Added
- `embedding_provider_config_id UUID REFERENCES embedding_provider_configs(id) ON DELETE SET NULL` column on `indexing_jobs` — pins the resolved embedding provider config at job creation time
- `llm_provider_config_id UUID REFERENCES llm_provider_configs(id) ON DELETE SET NULL` column on `indexing_jobs` — pins the resolved LLM provider config at job creation time (nullable)
- `FindActiveIndexingJobForProjectAndType` sqlc query — returns the most recent active (`queued`/`running`) job for a project + job type combination; powers deduplication logic
- Updated `CreateIndexingJob` sqlc query to include `embedding_provider_config_id` and `llm_provider_config_id` parameters

## [0.5.0] - 2026-03-07

### Added
- `email TEXT NOT NULL` column with unique index `idx_users_email` on `users` table (merged into `000001_init`)
- `GetUserByEmail` sqlc query — lookup active user by email
- `email` parameter added to `CreateUser` query
- `email` column added to `UpdateUserProfile` query (via `COALESCE(sqlc.narg(email), email)`)
- `u.email` added to `ListProjectMembers` and `GetSessionByTokenHash` explicit SELECT lists

## [0.4.0] - 2026-03-05

### Added
- Dual-type API key schema in `000001_init.up.sql`: `key_type`, `name`, `project_id` columns on `api_keys` with CHECK constraint, UNIQUE `key_hash` index, `role_rank()` function, and `api_key_project_access` view
- sqlc queries: `CreateAPIKey`, `ListProjectKeys`, `ListPersonalKeys`, `GetAPIKeyByID`, `SoftDeleteAPIKey`, `CheckAPIKeyProjectAccess`, `UpdateAPIKeyLastUsed`, `GetAPIKeyByHash`
- `CountProjectKeys` and `CountPersonalKeys` sqlc queries for key limit enforcement
- `role_rank()` SQL function extracted from inline CASE for reuse in view
- `LockProjectRow` and `LockUserRow` sqlc queries for transaction-safe key limit enforcement

### Fixed
- **Security:** `idx_api_keys_key_hash` is now `UNIQUE` — prevents duplicate key hash collisions
- **Security:** `api_key_project_access` view fail-closed on unknown roles — `role_rank()` returns 0 for unknown values, filtered out by `WHERE role_rank(...) > 0`
- `ListProjectKeys` query: added explicit `key_type = 'project'` filter
- `UpdateAPIKeyLastUsed` query: added `AND is_active = TRUE` filter
- Expired keys now excluded from `ListProjectKeys`, `ListPersonalKeys`, `CountProjectKeys`, `CountPersonalKeys`, and `UpdateAPIKeyLastUsed` queries
- `sqlc.yaml` schema list reduced to single `000001_init.up.sql`

### Changed
- Merged `000002_api_keys_v2` migration into `000001_init` — single migration with final dual-type API key schema (no separate up/down files for v2)
- Removed `000002_api_keys_v2.up.sql` and `000002_api_keys_v2.down.sql`
- Simplified API key roles from 3 to 2: `read` and `write` only (removed `admin`). Updated CHECK constraint on `api_keys.role`, `role_rank()` function, and `api_key_project_access` view accordingly

### Removed
- `api_key_projects` junction table (replaced by direct `project_id` FK on `api_keys`)
- Old API key queries from `keys_and_settings.sql`

## [0.3.2] - 2026-03-05

### Added
- `GetProjectMemberForUpdate` sqlc query — `SELECT * FROM project_members ... FOR UPDATE` for transaction-safe single-row locking; use inside a transaction to prevent TOCTOU races on role checks before mutations

## [0.3.1] - 2026-03-04

### Added
- `CountProjectOwnersForUpdate` sqlc query — counts project owners with `FOR UPDATE OF pm` row locking via subquery pattern (aggregates cannot use `FOR UPDATE` directly); use inside a transaction to serialize concurrent owner demotion/removal

## [0.3.0] - 2026-03-04

### Added
- `CountProjectsByMember` and `ListProjectsByMember` sqlc queries for member-scoped project pagination
- `GetProjectSSHKeyWithDetails` sqlc query — JOIN on `project_ssh_key_assignments` and `ssh_keys` returning key id, name, fingerprint, public_key, key_type, created_at for the active assignment

### Changed
- `CreateProject` sqlc query now includes `created_by` parameter; `default_branch` and `status` use `sqlc.narg` with COALESCE defaults (`'main'` and `'active'`)

## [0.2.0] - 2026-03-04

### Added
- `sessions` table in `000001_init.up.sql` — server-side session storage with `token_hash` (unique), `user_id` (FK → users), `ip_address`, `user_agent`, `expires_at`, indexes on `user_id` and `expires_at`
- sqlc queries: `CreateSession`, `GetSessionByTokenHash` (joins users, filters expired/inactive), `DeleteSession`, `DeleteUserSessions`, `DeleteExpiredSessions`

### Changed
- Sessions merged into `000001_init` migration (no separate 000002 — schema is pre-release)

## [0.1.0] - 2026-03-03

### Added
- Go module `myjungle/datastore` with `go.mod` for shared database packages
- Migration runner `RunMigrations(dsn)` in `postgres/migrate.go` using `golang-migrate/v4` with `iofs` source and `pgx/v5` database driver
- Embedded migration SQL files via `//go:embed migrations/*.sql`
- DSN validation: rejects empty DSNs, unsupported schemes, and key=value format with specific error messages
- `stripScheme()` helper to convert `postgres://` / `postgresql://` DSNs to the `pgx5://` scheme that golang-migrate expects
- `ErrNoChange` handled gracefully (logged, returns nil)
- sqlc-generated query layer in `postgres/sqlc/` (package `db`) with `DBTX` interface, `Queries`, and `WithTx` support
- Initial database schema migration (`000001_init.up.sql`)
