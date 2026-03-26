# PostgreSQL

## Responsibility

PostgreSQL is the source of truth for:

- Project metadata and lifecycle
- SSH key library and assignments
- API keys and project authorization mapping
- Embedding configuration and version history
- Index snapshots, indexing jobs, and status
- Files, symbols, dependencies, and raw code chunks
- Query logs and analytics base tables

It answers: "What exists, how is it related, and what state is it in?"

## Required Extensions

- `pgcrypto`
- optional `citext`

## Core Schema Domains

### Projects and Grouping

- `projects`
- `project_groups` (optional)
- `project_group_members`

### Git Authentication

- `ssh_keys`
- `project_ssh_key_assignments`

Constraints:

- One active SSH key assignment per project
- A single key may be shared by multiple projects
- Private key is encrypted and never returned by API

### Embedding Configuration

- `embedding_config` (active endpoint/model/dimensions)
- `embedding_versions` (immutable model version records)

### Indexing Lifecycle

- `index_snapshots`
- `indexing_jobs`

### Indexed Artifacts

- `files`
- `symbols`
- `code_chunks`
- `dependencies`

### Access and Observability

- `api_keys`
- `api_key_projects`
- `api_key_project_access` (view)
- `query_log`

## Key Relationship Rules

- `projects` owns snapshots/jobs/files/symbols/chunks/dependencies/query logs
- `api_keys` are many-to-many with projects via `api_key_projects`
- `index_snapshots` version all indexed artifacts by branch and embedding version
- `code_chunks` store raw text referenced from Qdrant payload via `raw_text_ref`
- `embedding_versions` tie each snapshot to exact model metadata
- Redis queue messages should only carry a job reference and minimal routing metadata; workflow-specific execution inputs belong in PostgreSQL and must be loaded by the worker using `job_id`

## Access Model

- API keys are hashed and checked against `api_key_project_access`
- Project-scoped authorization is evaluated on every backend query request
- Expired/inactive keys are excluded by view predicate

## Retention Baseline

- Keep full `query_log` for 30 days
- Roll up aggregates for long-term analytics
- Keep superseded snapshots for rollback grace period
- Retain inactive SSH key metadata for audit trail
- Optionally purge inactive encrypted private key material after grace period

## Migration and Query Tooling

- SQL migrations via `golang-migrate`
- Type-safe query generation via `sqlc`
- Explicit DDL in migration files (no runtime schema creation)

## Operational Notes

- Use regular logical backups (`pg_dump`) and point-in-time recovery strategy where needed
- Monitor table growth for `code_chunks`, `symbols`, and `query_log`
- Add targeted indexes only after query shape validation to control write amplification
