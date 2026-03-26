# Relational DB v8 (PostgreSQL)

## Responsibility

PostgreSQL is the source of truth for structured metadata, indexing lifecycle, access control, Git credentials, provider configuration, commit history, and analytics.

PostgreSQL answers: "What exists, how is it related, and what state is it in?"

## Why PostgreSQL

- Reliable multi-user concurrency
- Strong indexing options
- JSONB for controlled flexibility
- Well-supported migrations and backup tooling
- Advisory locks for lightweight per-project serialization

## Required Extensions

- `pgcrypto` for UUID generation, hashing utilities, and SSH key encryption
- optional `citext` for case-insensitive identities

## Auth Model

### Identity and Sessions

Authentication is split between two layers:

- **Backoffice (interactive):** GitHub OAuth via the `better-auth` library in the Next.js backoffice. BetterAuth handles the OIDC redirect flow using an ephemeral in-memory SQLite store; upon successful authentication the bridge endpoint calls the Go backend to create a durable session.
- **Go backend:** Owns the `users` and `sessions` tables. The `POST /v1/auth/login` endpoint creates server-side sessions with hashed tokens. Identity resolution uses session tokens (Bearer header or HttpOnly cookie).
- **MCP / programmatic:** API keys (see below) provide project-scoped access independently of OAuth.

```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT,
  avatar_url TEXT,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  ip_address TEXT,
  user_agent TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Platform Roles

Platform-level roles are separate from project-scoped roles and support future expansion.

```sql
CREATE TABLE user_platform_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('platform_admin')),
  granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, role)
);
```

### Project Members (Project-Scoped RBAC)

Every project membership resolves to one role: `owner`, `admin`, or `member`.

```sql
CREATE TABLE project_members (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
  invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, user_id)
);
```

Ownership invariants enforced by backend logic:

- Each project must always have at least one `owner`
- `admin` cannot grant/remove `owner`
- Last owner cannot remove or demote themselves

## Core Tables

### Projects

```sql
CREATE TABLE projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  repo_url TEXT NOT NULL,
  default_branch TEXT NOT NULL DEFAULT 'main',
  status TEXT NOT NULL DEFAULT 'active',
  selected_embedding_global_config_id UUID REFERENCES embedding_provider_configs(id),
  selected_llm_global_config_id UUID REFERENCES llm_provider_configs(id),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Projects optionally reference a global embedding or LLM provider config. Trigger-based validation ensures these references point to shareable global configs (not project-owned ones).

### SSH Keys (Git Authentication)

SSH keys are managed as a per-user private key library. Projects reference keys through an assignment table. Private keys are encrypted at rest using `pgcrypto`.

```sql
CREATE TABLE ssh_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  public_key TEXT NOT NULL,
  private_key_encrypted BYTEA NOT NULL,
  key_type TEXT NOT NULL DEFAULT 'ed25519',
  fingerprint TEXT NOT NULL UNIQUE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  rotated_at TIMESTAMPTZ
);

CREATE TABLE project_ssh_key_assignments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  ssh_key_id UUID NOT NULL REFERENCES ssh_keys(id) ON DELETE RESTRICT,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  unassigned_at TIMESTAMPTZ,
  UNIQUE (project_id, ssh_key_id, assigned_at)
);
```

Key lifecycle:

- Generated manually from backoffice (per-user key library)
- Assigned to projects explicitly (one active assignment per project, enforced by partial unique index)
- A single key can be assigned to multiple projects at the same time
- Key replacement is performed by creating a new key and switching project assignments
- Private key is never exposed via API; only the public key and fingerprint are readable

## Provider Configuration

### Embedding Provider Configs

Supports a pluggable provider architecture. Configs can be platform-global or project-owned.

```sql
CREATE TABLE embedding_provider_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'ollama',
  endpoint_url TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INT NOT NULL CHECK (dimensions > 0 AND dimensions <= 65536),
  max_tokens INT NOT NULL DEFAULT 8000,
  settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  credentials_encrypted BYTEA,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  is_available_to_projects BOOLEAN NOT NULL DEFAULT FALSE,
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Two-tier resolution:

- `project_id IS NULL` = platform global config. Can be marked `is_default` and `is_available_to_projects`.
- `project_id IS NOT NULL` = project-owned custom config. Cannot be default or shared.
- Only one default global config at a time (enforced by partial unique index).
- Only one active project-owned config per project (enforced by partial unique index).
- Projects select a global config via `selected_embedding_global_config_id` or use their own project-owned config.

### LLM Provider Configs

Same structure and resolution strategy as embedding providers.

```sql
CREATE TABLE llm_provider_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'ollama',
  endpoint_url TEXT NOT NULL,
  model TEXT,                -- NULL allowed; LLM providers may choose a model dynamically
  settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  credentials_encrypted BYTEA,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  is_available_to_projects BOOLEAN NOT NULL DEFAULT FALSE,
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Credentials for both provider types are encrypted at rest in the `credentials_encrypted` column.

### Embedding Versions

Immutable records that tie each indexed snapshot to the exact embedding configuration used.

```sql
CREATE TABLE embedding_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  embedding_provider_config_id UUID NOT NULL REFERENCES embedding_provider_configs(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INT NOT NULL,
  version_label TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## Indexing Lifecycle Tables

### Index Snapshots

One logical indexed state per project + branch + embedding version. Links to the commit row for the indexed revision.

```sql
CREATE TABLE index_snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  branch TEXT NOT NULL,
  embedding_version_id UUID NOT NULL REFERENCES embedding_versions(id),
  git_commit TEXT NOT NULL,
  commit_id UUID,              -- FK to commits table
  is_active BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL,        -- building, active, superseded, failed
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  activated_at TIMESTAMPTZ,
  UNIQUE(project_id, branch, embedding_version_id, git_commit)
);
```

A partial unique index on `(project_id, branch) WHERE is_active = TRUE` ensures at most one active snapshot per project-branch pair.

### Indexing Jobs

```sql
CREATE TABLE indexing_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID REFERENCES index_snapshots(id) ON DELETE SET NULL,
  job_type TEXT NOT NULL,       -- full, incremental, rebuild
  status TEXT NOT NULL,         -- queued, running, completed, failed
  files_processed INT NOT NULL DEFAULT 0,
  chunks_upserted INT NOT NULL DEFAULT 0,
  vectors_deleted INT NOT NULL DEFAULT 0,
  error_details JSONB NOT NULL DEFAULT '[]'::jsonb,
  embedding_provider_config_id UUID REFERENCES embedding_provider_configs(id),
  llm_provider_config_id UUID REFERENCES llm_provider_configs(id),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Each job records the provider configs it was executed with for audit and reproducibility.

## Artifact Tables

### Files

```sql
CREATE TABLE files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_path TEXT NOT NULL,
  language TEXT,
  file_hash TEXT NOT NULL,
  size_bytes BIGINT,
  line_count INT,
  file_content_id UUID,        -- FK to deduplicated file_contents
  file_facts JSONB,            -- extracted file-level metadata and facts
  parser_meta JSONB,           -- parser execution metadata
  extractor_statuses JSONB,    -- per-extractor success/failure tracking
  issues JSONB,                -- diagnostics and parse issues
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, index_snapshot_id, file_path)
);
```

### File Contents (Deduplicated)

Content-addressable storage for raw file text, deduplicated per project by content hash. Optionally stores the tree-sitter AST for reuse.

```sql
CREATE TABLE file_contents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  content_hash TEXT NOT NULL,
  content TEXT NOT NULL,
  tree_sitter_ast JSONB,
  size_bytes BIGINT NOT NULL,
  line_count INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, content_hash)
);
```

### Symbols

```sql
CREATE TABLE symbols (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  qualified_name TEXT,
  kind TEXT NOT NULL,            -- function, class, method, interface, type, enum, variable, module, struct, trait, ...
  signature TEXT,
  start_line INT,
  end_line INT,
  doc_text TEXT,
  symbol_hash TEXT NOT NULL,
  parent_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  flags JSONB,                   -- language-specific boolean flags
  modifiers TEXT[],              -- public, private, static, async, ...
  return_type TEXT,
  parameter_types TEXT[],
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Code Chunks

Stores the raw source text for every chunk that was embedded and stored in Qdrant. Qdrant payload references these by `id` via `raw_text_ref`.

```sql
CREATE TABLE code_chunks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  chunk_type TEXT NOT NULL,          -- function, class, module_context, config, test
  chunk_hash TEXT NOT NULL,          -- sha256 of content for dedup
  content TEXT NOT NULL,             -- raw source text
  context_before TEXT,               -- leading context (imports, declarations)
  context_after TEXT,                -- trailing context
  start_line INT NOT NULL,
  end_line INT NOT NULL,
  estimated_tokens INT,
  owner_qualified_name TEXT,         -- qualified name of the owning symbol
  owner_kind TEXT,                   -- kind of the owning symbol
  is_exported_context BOOLEAN NOT NULL DEFAULT FALSE,
  semantic_role TEXT,                -- semantic annotation for retrieval hints
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Dependencies (Imports)

Stores import relationships extracted from source files. The `import_type` field classifies the import across all supported languages.

```sql
CREATE TABLE dependencies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  source_symbol_id UUID REFERENCES symbols(id) ON DELETE CASCADE,
  source_file_path TEXT NOT NULL,
  target_file_path TEXT,
  import_name TEXT NOT NULL,
  import_type TEXT NOT NULL,        -- INTERNAL, EXTERNAL, STDLIB
  package_name TEXT,
  package_version TEXT
);
```

Import type semantics across languages:

- `INTERNAL` -- resolved to a file within the project (e.g., `./utils`, `crate::handlers`, `app.models`, `myapp/internal/handler`)
- `EXTERNAL` -- third-party package not part of the project tree (e.g., `express`, `github.com/other/lib`)
- `STDLIB` -- standard library import (e.g., `node:path`, `fmt`, `java.util.List`)

Cross-language import resolution uses a multi-strategy resolver: exact path match, extension completion, index-file resolution, and stem-based fuzzy match. Resolved internal imports have `target_file_path` populated; external and stdlib imports leave it NULL.

### Exports

File-level export declarations.

```sql
CREATE TABLE exports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  export_kind TEXT NOT NULL,
  exported_name TEXT NOT NULL,
  local_name TEXT,
  symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  source_module TEXT,
  line INT NOT NULL,
  "column" INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Symbol References

AST-level references to other symbols, with optional resolution to a target symbol.

```sql
CREATE TABLE symbol_references (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  source_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  reference_kind TEXT NOT NULL,
  raw_text TEXT,
  target_name TEXT NOT NULL,
  qualified_target_hint TEXT,
  start_line INT NOT NULL,
  start_column INT NOT NULL,
  end_line INT NOT NULL,
  end_column INT NOT NULL,
  resolution_scope TEXT,
  resolved_target_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  confidence TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### JSX Usages

JSX component usage tracking, specific to JSX/TSX files.

```sql
CREATE TABLE jsx_usages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  source_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  component_name TEXT NOT NULL,
  is_intrinsic BOOLEAN NOT NULL DEFAULT FALSE,
  is_fragment BOOLEAN NOT NULL DEFAULT FALSE,
  line INT NOT NULL,
  "column" INT NOT NULL,
  resolved_target_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  confidence TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Network Calls

Detected HTTP/fetch call sites extracted from source code.

```sql
CREATE TABLE network_calls (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  source_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  client_kind TEXT NOT NULL,
  method TEXT NOT NULL,
  url_literal TEXT,
  url_template TEXT,
  is_relative BOOLEAN NOT NULL DEFAULT FALSE,
  start_line INT NOT NULL,
  start_column INT NOT NULL,
  confidence TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## Commit Tables

### Commits

Git commit metadata stored per project for history tracking and incremental indexing.

```sql
CREATE TABLE commits (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_hash TEXT NOT NULL,
  author_name TEXT NOT NULL,
  author_email TEXT NOT NULL,
  author_date TIMESTAMPTZ NOT NULL,
  committer_name TEXT NOT NULL,
  committer_email TEXT NOT NULL,
  committer_date TIMESTAMPTZ NOT NULL,
  message TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, commit_hash)
);
```

### Commit Parents

Supports merge commits with ordered parent references.

```sql
CREATE TABLE commit_parents (
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_id UUID NOT NULL,
  parent_commit_id UUID NOT NULL,
  ordinal INT NOT NULL DEFAULT 0,
  PRIMARY KEY (project_id, commit_id, parent_commit_id)
);
```

### Commit File Diffs

Per-file diffs between a commit and its parent, used for incremental indexing and change history.

```sql
CREATE TABLE commit_file_diffs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_id UUID NOT NULL,
  parent_commit_id UUID,
  old_file_path TEXT,
  new_file_path TEXT,
  change_type TEXT NOT NULL,       -- added, modified, deleted, renamed, copied
  patch TEXT,
  additions INT NOT NULL DEFAULT 0,
  deletions INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

An idempotent unique index on `(project_id, commit_id, old_file_path, new_file_path)` prevents duplicate diff rows.

## API Keys

API keys are used by MCP agents and programmatic clients for project-scoped access. Two types exist:

- **Project keys** (`key_type = 'project'`): scoped to exactly one project via a direct FK. Managed in the project context by project admins/owners.
- **Personal keys** (`key_type = 'personal'`): scoped to all projects the creator is a member of. Access is dynamic -- derived at query time from `project_members`. Managed in the user context.

```sql
CREATE TABLE api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key_type TEXT NOT NULL DEFAULT 'project',
  key_prefix TEXT NOT NULL,
  key_hash TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'read' CHECK (role IN ('read', 'write')),
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  expires_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_api_key_type_project CHECK (
    (key_type = 'project' AND project_id IS NOT NULL) OR
    (key_type = 'personal' AND project_id IS NULL)
  )
);
```

Key format distinguishes types visually: project keys use prefix `mj_proj_`, personal keys use `mj_pers_`. The `key_prefix` column stores the first characters for display.

Unified access check view (`api_key_project_access`) handles both types. For project keys, access is a direct FK match. For personal keys, access is derived from `project_members` with effective role = MIN(key role, mapped membership role). Membership-to-API role mapping: `owner->write`, `admin->write`, `member->read`.

A helper function `role_rank(r TEXT)` maps role names to integer ranks for MIN-role computation. Unknown roles map to rank 0 and are excluded (fail-closed).

## Query Log

```sql
CREATE TABLE query_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  api_key_id UUID REFERENCES api_keys(id) ON DELETE SET NULL,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  tool_name TEXT NOT NULL,
  query_text TEXT,
  result_count INT,
  latency_ms INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## Advisory Lock Pattern

Workers use PostgreSQL advisory locks to serialize concurrent indexing jobs for the same project. This prevents snapshot corruption from overlapping full-index and incremental-index runs.

```sql
-- Try to acquire (non-blocking)
SELECT pg_try_advisory_lock(key1::int, key2::int)::boolean AS acquired;

-- Release when done
SELECT pg_advisory_unlock(key1::int, key2::int)::boolean AS released;
```

Lock keys are derived from the project UUID. The lock is acquired on a pinned database connection before claiming a job and released on completion or failure. Session-level advisory locks auto-release on disconnect, preventing leaked locks from contaminating the connection pool.

## Relationships

- `projects` owns snapshots, jobs, files, symbols, chunks, dependencies, exports, symbol_references, jsx_usages, network_calls, commits, commit_file_diffs, and query_log.
- `users` own identity records and are linked to projects through `project_members`.
- `user_platform_roles` assigns platform-level roles (e.g., `platform_admin`) separately from project roles.
- `api_keys` come in two types: project keys have a direct FK to one project; personal keys derive access dynamically from `project_members`. The `api_key_project_access` view unifies both for access checks.
- `index_snapshots` version every indexed artifact and enable safe model switches. Each snapshot links to a `commits` row.
- `code_chunks` store raw text referenced by Qdrant payload `raw_text_ref` field.
- `file_contents` provides content-addressable deduplication for raw file text.
- `ssh_keys` store reusable Git authentication credentials.
- `project_ssh_key_assignments` maps projects to one active SSH key while allowing one key to be shared by many projects.
- `embedding_provider_configs` and `llm_provider_configs` store pluggable provider settings with two-tier resolution (global vs project-owned).
- `embedding_versions` link each snapshot to the exact embedding configuration used.

## Migration and Access

- Use `golang-migrate` for SQL migrations
- Use `sqlc` for type-safe query code generation
- Keep DDL explicit; avoid runtime schema creation

## Retention Policy (Initial)

- Keep full `query_log` for 30 days
- Roll up daily aggregates for long-term analytics
- Keep superseded snapshots for configurable grace window (for rollback)
- Inactive SSH keys retained for audit trail; private key material cleared after grace period
