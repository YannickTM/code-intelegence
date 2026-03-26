CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- Users (Phase 1: username-based identity, no passwords, local network trust)
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_users_username ON users(username);
CREATE UNIQUE INDEX idx_users_email ON users(email);

CREATE INDEX idx_users_active
  ON users(is_active)
  WHERE is_active = TRUE;

-- Platform-level roles (separate from project-scoped roles).
-- Supports future platform roles (e.g. platform_viewer).
CREATE TABLE user_platform_roles (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       TEXT NOT NULL CHECK (role IN ('platform_admin')),
  granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, role)
);

CREATE INDEX idx_upr_user ON user_platform_roles(user_id);
CREATE INDEX idx_upr_role ON user_platform_roles(role);

-- Projects
CREATE TABLE projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  repo_url TEXT NOT NULL,
  default_branch TEXT NOT NULL DEFAULT 'main',
  status TEXT NOT NULL DEFAULT 'active',
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Project Members (project-scoped RBAC)
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

CREATE INDEX idx_pm_project ON project_members(project_id);
CREATE INDEX idx_pm_user ON project_members(user_id);
CREATE INDEX idx_pm_project_role ON project_members(project_id, role);

-- SSH Keys (Git authentication)
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

CREATE UNIQUE INDEX idx_project_active_ssh_key
  ON project_ssh_key_assignments(project_id)
  WHERE is_active = TRUE;

CREATE INDEX idx_active_project_assignments_by_key
  ON project_ssh_key_assignments(ssh_key_id)
  WHERE is_active = TRUE;

-- Embedding provider configurations
-- project_id NULL = platform global config, non-NULL = project-owned custom config.
CREATE TABLE embedding_provider_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'ollama',
  endpoint_url TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INT NOT NULL CHECK (dimensions > 0 AND dimensions <= 65536),
  max_tokens INT NOT NULL DEFAULT 8000 CHECK (max_tokens >= 1 AND max_tokens <= 131072),
  settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  credentials_encrypted BYTEA,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  is_available_to_projects BOOLEAN NOT NULL DEFAULT FALSE,
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_embedding_provider_configs_global_flags
    CHECK (
      project_id IS NULL
      OR (is_default = FALSE AND is_available_to_projects = FALSE)
    )
);

-- Only one default GLOBAL config (project_id IS NULL).
CREATE UNIQUE INDEX idx_single_default_global_embedding_provider_config
  ON embedding_provider_configs(is_default)
  WHERE is_default = TRUE AND project_id IS NULL;

CREATE UNIQUE INDEX idx_single_active_project_embedding_provider_config
  ON embedding_provider_configs(project_id, is_active)
  WHERE is_active = TRUE AND project_id IS NOT NULL;

CREATE INDEX idx_embedding_provider_configs_project_id
  ON embedding_provider_configs(project_id);

-- LLM provider configurations
-- project_id NULL = platform global config, non-NULL = project-owned custom config.
CREATE TABLE llm_provider_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'ollama',
  endpoint_url TEXT NOT NULL,
  -- NULL model allowed; LLM providers may choose a model dynamically at runtime.
  model TEXT,
  settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  credentials_encrypted BYTEA,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  is_available_to_projects BOOLEAN NOT NULL DEFAULT FALSE,
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_llm_provider_configs_global_flags
    CHECK (
      project_id IS NULL
      OR (is_default = FALSE AND is_available_to_projects = FALSE)
    )
);

CREATE UNIQUE INDEX idx_single_default_global_llm_provider_config
  ON llm_provider_configs(is_default)
  WHERE is_default = TRUE AND project_id IS NULL;

CREATE UNIQUE INDEX idx_single_active_project_llm_provider_config
  ON llm_provider_configs(project_id, is_active)
  WHERE is_active = TRUE AND project_id IS NOT NULL;

CREATE INDEX idx_llm_provider_configs_project_id
  ON llm_provider_configs(project_id);

ALTER TABLE projects
  ADD COLUMN selected_embedding_global_config_id UUID
    REFERENCES embedding_provider_configs(id) ON DELETE SET NULL,
  ADD COLUMN selected_llm_global_config_id UUID
    REFERENCES llm_provider_configs(id) ON DELETE SET NULL;

CREATE INDEX idx_projects_selected_embedding_global_config_id
  ON projects(selected_embedding_global_config_id);

CREATE INDEX idx_projects_selected_llm_global_config_id
  ON projects(selected_llm_global_config_id);

CREATE OR REPLACE FUNCTION validate_projects_selected_global_configs()
RETURNS TRIGGER AS $$
DECLARE
  embedding_cfg embedding_provider_configs%ROWTYPE;
  llm_cfg llm_provider_configs%ROWTYPE;
BEGIN
  IF TG_TABLE_NAME = 'embedding_provider_configs' THEN
    PERFORM 1
    FROM projects
    WHERE selected_embedding_global_config_id = OLD.id
    FOR UPDATE;

    IF NEW.project_id IS NOT NULL AND EXISTS (
      SELECT 1
      FROM projects
      WHERE selected_embedding_global_config_id = OLD.id
    ) THEN
      RAISE EXCEPTION 'selected embedding provider config cannot become project-owned while referenced by a project';
    END IF;
    IF NEW.is_available_to_projects = FALSE AND EXISTS (
      SELECT 1
      FROM projects
      WHERE selected_embedding_global_config_id = OLD.id
    ) THEN
      RAISE EXCEPTION 'selected_embedding_global_config_id must reference a shareable global embedding provider config';
    END IF;
    RETURN NEW;
  END IF;

  IF TG_TABLE_NAME = 'llm_provider_configs' THEN
    PERFORM 1
    FROM projects
    WHERE selected_llm_global_config_id = OLD.id
    FOR UPDATE;

    IF NEW.project_id IS NOT NULL AND EXISTS (
      SELECT 1
      FROM projects
      WHERE selected_llm_global_config_id = OLD.id
    ) THEN
      RAISE EXCEPTION 'selected llm provider config cannot become project-owned while referenced by a project';
    END IF;
    IF NEW.is_available_to_projects = FALSE AND EXISTS (
      SELECT 1
      FROM projects
      WHERE selected_llm_global_config_id = OLD.id
    ) THEN
      RAISE EXCEPTION 'selected_llm_global_config_id must reference a shareable global llm provider config';
    END IF;
    RETURN NEW;
  END IF;

  IF NEW.selected_embedding_global_config_id IS NOT NULL THEN
    SELECT *
    INTO embedding_cfg
    FROM embedding_provider_configs
    WHERE id = NEW.selected_embedding_global_config_id
    FOR UPDATE;

    IF FOUND AND (embedding_cfg.project_id IS NOT NULL OR embedding_cfg.is_available_to_projects = FALSE) THEN
      RAISE EXCEPTION 'selected_embedding_global_config_id must reference a shareable global embedding provider config';
    END IF;
  END IF;

  IF NEW.selected_llm_global_config_id IS NOT NULL THEN
    SELECT *
    INTO llm_cfg
    FROM llm_provider_configs
    WHERE id = NEW.selected_llm_global_config_id
    FOR UPDATE;

    IF FOUND AND (llm_cfg.project_id IS NOT NULL OR llm_cfg.is_available_to_projects = FALSE) THEN
      RAISE EXCEPTION 'selected_llm_global_config_id must reference a shareable global llm provider config';
    END IF;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER validate_projects_selected_embedding_global_config_trg
BEFORE INSERT OR UPDATE OF selected_embedding_global_config_id
ON projects
FOR EACH ROW
EXECUTE FUNCTION validate_projects_selected_global_configs();

CREATE TRIGGER validate_projects_selected_llm_global_config_trg
BEFORE INSERT OR UPDATE OF selected_llm_global_config_id
ON projects
FOR EACH ROW
EXECUTE FUNCTION validate_projects_selected_global_configs();

CREATE TRIGGER validate_embedding_provider_configs_selected_global_reference_trg
BEFORE UPDATE OF project_id, is_available_to_projects
ON embedding_provider_configs
FOR EACH ROW
EXECUTE FUNCTION validate_projects_selected_global_configs();

CREATE TRIGGER validate_llm_provider_configs_selected_global_reference_trg
BEFORE UPDATE OF project_id, is_available_to_projects
ON llm_provider_configs
FOR EACH ROW
EXECUTE FUNCTION validate_projects_selected_global_configs();

-- Embedding versions
CREATE TABLE embedding_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  embedding_provider_config_id UUID NOT NULL REFERENCES embedding_provider_configs(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimensions INT NOT NULL CHECK (dimensions > 0 AND dimensions <= 65536),
  version_label TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index snapshots
CREATE TABLE index_snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  branch TEXT NOT NULL,
  embedding_version_id UUID NOT NULL REFERENCES embedding_versions(id),
  git_commit TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  activated_at TIMESTAMPTZ,
  UNIQUE(project_id, branch, embedding_version_id, git_commit)
);

-- Indexing jobs
CREATE TABLE indexing_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID REFERENCES index_snapshots(id) ON DELETE SET NULL,
  job_type TEXT NOT NULL,
  status TEXT NOT NULL,
  files_processed INT NOT NULL DEFAULT 0,
  chunks_upserted INT NOT NULL DEFAULT 0,
  vectors_deleted INT NOT NULL DEFAULT 0,
  error_details JSONB NOT NULL DEFAULT '[]'::jsonb,
  embedding_provider_config_id UUID REFERENCES embedding_provider_configs(id) ON DELETE SET NULL,
  llm_provider_config_id UUID REFERENCES llm_provider_configs(id) ON DELETE SET NULL,
  worker_id TEXT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_project_created
  ON indexing_jobs(project_id, created_at DESC);

-- Files
CREATE TABLE files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_path TEXT NOT NULL,
  language TEXT,
  file_hash TEXT NOT NULL,
  size_bytes BIGINT,
  line_count INT,
  file_facts JSONB,
  parser_meta JSONB,
  extractor_statuses JSONB,
  issues JSONB,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, index_snapshot_id, file_path)
);

-- Symbols
CREATE TABLE symbols (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  qualified_name TEXT,
  kind TEXT NOT NULL,
  signature TEXT,
  start_line INT,
  end_line INT,
  doc_text TEXT,
  symbol_hash TEXT NOT NULL,
  parent_symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  flags JSONB,
  modifiers TEXT[],
  return_type TEXT,
  parameter_types TEXT[],
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_symbols_lookup
  ON symbols(project_id, index_snapshot_id, name);

CREATE INDEX idx_symbols_file_id
  ON symbols(file_id);

-- Code chunks
CREATE TABLE code_chunks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  symbol_id UUID REFERENCES symbols(id) ON DELETE SET NULL,
  chunk_type TEXT NOT NULL,
  chunk_hash TEXT NOT NULL,
  content TEXT NOT NULL,
  context_before TEXT,
  context_after TEXT,
  start_line INT NOT NULL,
  end_line INT NOT NULL,
  estimated_tokens INT,
  owner_qualified_name TEXT,
  owner_kind TEXT,
  is_exported_context BOOLEAN NOT NULL DEFAULT FALSE,
  semantic_role TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chunks_snapshot
  ON code_chunks(project_id, index_snapshot_id);

CREATE INDEX idx_chunks_file
  ON code_chunks(file_id);

-- Dependencies
CREATE TABLE dependencies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  source_symbol_id UUID REFERENCES symbols(id) ON DELETE CASCADE,
  source_file_path TEXT NOT NULL,
  target_file_path TEXT,
  import_name TEXT NOT NULL,
  import_type TEXT NOT NULL,
  package_name TEXT,
  package_version TEXT
);

CREATE INDEX idx_deps_source
  ON dependencies(project_id, index_snapshot_id, source_file_path);

CREATE INDEX idx_deps_target
  ON dependencies(project_id, index_snapshot_id, target_file_path)
  WHERE target_file_path IS NOT NULL;

-- Exports: file-level export declarations.
CREATE TABLE exports (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id           UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  export_kind       TEXT NOT NULL,
  exported_name     TEXT NOT NULL,
  local_name        TEXT,
  symbol_id         UUID REFERENCES symbols(id) ON DELETE SET NULL,
  source_module     TEXT,
  line              INT NOT NULL,
  "column"          INT NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_exports_file ON exports(file_id);
CREATE INDEX idx_exports_snapshot ON exports(project_id, index_snapshot_id);
CREATE INDEX idx_exports_symbol ON exports(symbol_id) WHERE symbol_id IS NOT NULL;

-- Symbol references: AST references to other symbols.
-- Named symbol_references to avoid the SQL reserved word "references".
CREATE TABLE symbol_references (
  id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id                 UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id          UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id                    UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  source_symbol_id           UUID REFERENCES symbols(id) ON DELETE SET NULL,
  reference_kind             TEXT NOT NULL,
  raw_text                   TEXT,
  target_name                TEXT NOT NULL,
  qualified_target_hint      TEXT,
  start_line                 INT NOT NULL,
  start_column               INT NOT NULL,
  end_line                   INT NOT NULL,
  end_column                 INT NOT NULL,
  resolution_scope           TEXT,
  resolved_target_symbol_id  UUID REFERENCES symbols(id) ON DELETE SET NULL,
  confidence                 TEXT,
  created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_symbol_references_file ON symbol_references(file_id);
CREATE INDEX idx_symbol_references_snapshot ON symbol_references(project_id, index_snapshot_id);
CREATE INDEX idx_symbol_references_source ON symbol_references(source_symbol_id) WHERE source_symbol_id IS NOT NULL;
CREATE INDEX idx_symbol_references_target ON symbol_references(resolved_target_symbol_id) WHERE resolved_target_symbol_id IS NOT NULL;

-- JSX usages: JSX component usage tracking.
CREATE TABLE jsx_usages (
  id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id                 UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id          UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id                    UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  source_symbol_id           UUID REFERENCES symbols(id) ON DELETE SET NULL,
  component_name             TEXT NOT NULL,
  is_intrinsic               BOOLEAN NOT NULL DEFAULT FALSE,
  is_fragment                BOOLEAN NOT NULL DEFAULT FALSE,
  line                       INT NOT NULL,
  "column"                   INT NOT NULL,
  resolved_target_symbol_id  UUID REFERENCES symbols(id) ON DELETE SET NULL,
  confidence                 TEXT,
  created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jsx_usages_file ON jsx_usages(file_id);
CREATE INDEX idx_jsx_usages_snapshot ON jsx_usages(project_id, index_snapshot_id);

-- Network calls: detected HTTP/fetch calls.
CREATE TABLE network_calls (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id           UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  source_symbol_id  UUID REFERENCES symbols(id) ON DELETE SET NULL,
  client_kind       TEXT NOT NULL,
  method            TEXT NOT NULL,
  url_literal       TEXT,
  url_template      TEXT,
  is_relative       BOOLEAN NOT NULL DEFAULT FALSE,
  start_line        INT NOT NULL,
  start_column      INT NOT NULL,
  confidence        TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_network_calls_file ON network_calls(file_id);
CREATE INDEX idx_network_calls_snapshot ON network_calls(project_id, index_snapshot_id);

-- API keys (dual-type: project keys + personal keys)
CREATE TABLE api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key_type TEXT NOT NULL DEFAULT 'project',
  key_prefix TEXT NOT NULL,
  key_hash TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'read',
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  expires_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_api_key_role CHECK (role IN ('read', 'write')),
  CONSTRAINT chk_api_key_type_project CHECK (
    (key_type = 'project' AND project_id IS NOT NULL)
    OR
    (key_type = 'personal' AND project_id IS NULL)
  )
);

CREATE INDEX idx_api_keys_project
  ON api_keys(project_id)
  WHERE project_id IS NOT NULL;

CREATE INDEX idx_api_keys_created_by_personal
  ON api_keys(created_by)
  WHERE key_type = 'personal';

CREATE UNIQUE INDEX idx_api_keys_key_hash
  ON api_keys(key_hash);

-- Helper: map role text to integer rank for MIN-role computation.
CREATE FUNCTION role_rank(r TEXT) RETURNS INT IMMUTABLE LANGUAGE SQL AS $$
  SELECT CASE r
    WHEN 'write' THEN 2
    WHEN 'read'  THEN 1
    ELSE 0
  END;
$$;

-- Unified access view for both key types.
-- Returns one row per (key, project) with the effective_role.
-- Unknown roles map to rank 0 and are excluded (fail-closed).
CREATE VIEW api_key_project_access AS

-- Project keys: effective_role = key's own role.
SELECT
  ak.id          AS api_key_id,
  ak.key_prefix,
  ak.key_hash,
  ak.role        AS effective_role,
  ak.is_active,
  ak.project_id
FROM api_keys ak
WHERE ak.key_type = 'project'
  AND ak.is_active = TRUE
  AND (ak.expires_at IS NULL OR ak.expires_at > NOW())
  AND role_rank(ak.role) > 0

UNION ALL

-- Personal keys: effective_role = MIN(key_role, mapped_membership_role).
-- Membership mapping: owner→write(2), admin→write(2), member→read(1).
SELECT
  ak.id          AS api_key_id,
  ak.key_prefix,
  ak.key_hash,
  CASE LEAST(role_rank(ak.role), role_rank(
    CASE pm.role
      WHEN 'owner'  THEN 'write'
      WHEN 'admin'  THEN 'write'
      WHEN 'member' THEN 'read'
      ELSE ''
    END
  ))
    WHEN 2 THEN 'write'
    WHEN 1 THEN 'read'
  END             AS effective_role,
  ak.is_active,
  pm.project_id
FROM api_keys ak
JOIN project_members pm ON ak.created_by = pm.user_id
WHERE ak.key_type = 'personal'
  AND ak.is_active = TRUE
  AND (ak.expires_at IS NULL OR ak.expires_at > NOW())
  AND role_rank(ak.role) > 0
  AND role_rank(
    CASE pm.role
      WHEN 'owner'  THEN 'write'
      WHEN 'admin'  THEN 'write'
      WHEN 'member' THEN 'read'
      ELSE ''
    END
  ) > 0;

-- Query log
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

CREATE INDEX idx_query_log_project_time
  ON query_log(project_id, created_at DESC);

-- Sessions (server-side, opaque token, SHA-256 hashed)
CREATE TABLE sessions (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL UNIQUE,
  ip_address  TEXT,
  user_agent  TEXT,
  expires_at  TIMESTAMPTZ NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- File contents (content-addressable, deduplicated per project)
CREATE TABLE file_contents (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  content_hash    TEXT NOT NULL,
  content         TEXT NOT NULL,
  tree_sitter_ast JSONB,
  size_bytes      BIGINT NOT NULL,
  line_count      INT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, content_hash),
  UNIQUE(project_id, id)
);

-- Link existing files table to deduplicated content
ALTER TABLE files
  ADD COLUMN file_content_id UUID;

ALTER TABLE files
  ADD CONSTRAINT fk_files_file_content
    FOREIGN KEY (project_id, file_content_id)
    REFERENCES file_contents(project_id, id) ON DELETE SET NULL (file_content_id);

CREATE INDEX idx_files_content_id
  ON files(file_content_id);

CREATE INDEX idx_files_snapshot_path
  ON files(index_snapshot_id, file_path);

-- Git commits per project
CREATE TABLE commits (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_hash     TEXT NOT NULL,
  author_name     TEXT NOT NULL,
  author_email    TEXT NOT NULL,
  author_date     TIMESTAMPTZ NOT NULL,
  committer_name  TEXT NOT NULL,
  committer_email TEXT NOT NULL,
  committer_date  TIMESTAMPTZ NOT NULL,
  message         TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, commit_hash),
  UNIQUE(project_id, id)
);

CREATE INDEX idx_commits_project_date
  ON commits(project_id, committer_date DESC);

-- Commit parent relationships (supports merge commits)
CREATE TABLE commit_parents (
  project_id       UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_id        UUID NOT NULL,
  parent_commit_id UUID NOT NULL,
  ordinal          INT NOT NULL DEFAULT 0,
  PRIMARY KEY (project_id, commit_id, parent_commit_id),
  UNIQUE(project_id, commit_id, ordinal),
  CHECK (ordinal >= 0),
  CONSTRAINT fk_cp_commit
    FOREIGN KEY (project_id, commit_id)
    REFERENCES commits(project_id, id) ON DELETE CASCADE,
  CONSTRAINT fk_cp_parent
    FOREIGN KEY (project_id, parent_commit_id)
    REFERENCES commits(project_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_commit_parents_parent
  ON commit_parents(parent_commit_id);

-- Per-file diffs between a commit and its parent
CREATE TABLE commit_file_diffs (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id       UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_id        UUID NOT NULL,
  parent_commit_id UUID,
  old_file_path    TEXT,
  new_file_path    TEXT,
  change_type      TEXT NOT NULL,
  patch            TEXT,
  additions        INT NOT NULL DEFAULT 0,
  deletions        INT NOT NULL DEFAULT 0,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_diff_change_type
    CHECK (change_type IN ('added', 'modified', 'deleted', 'renamed', 'copied')),
  CONSTRAINT chk_diff_has_path
    CHECK (old_file_path IS NOT NULL OR new_file_path IS NOT NULL),
  CONSTRAINT chk_diff_nonneg
    CHECK (additions >= 0 AND deletions >= 0),
  CONSTRAINT fk_cfd_commit
    FOREIGN KEY (project_id, commit_id)
    REFERENCES commits(project_id, id) ON DELETE CASCADE,
  CONSTRAINT fk_cfd_parent
    FOREIGN KEY (project_id, parent_commit_id)
    REFERENCES commits(project_id, id) ON DELETE SET NULL (parent_commit_id)
);

CREATE INDEX idx_commit_file_diffs_commit
  ON commit_file_diffs(commit_id);

CREATE INDEX idx_commit_file_diffs_project_commit
  ON commit_file_diffs(project_id, commit_id);

CREATE INDEX idx_commit_file_diffs_file_path
  ON commit_file_diffs(project_id, new_file_path);

CREATE INDEX idx_commit_file_diffs_old_file_path
  ON commit_file_diffs(project_id, old_file_path);

CREATE UNIQUE INDEX idx_cfd_idempotent
  ON commit_file_diffs(project_id, commit_id,
    COALESCE(old_file_path, ''), COALESCE(new_file_path, ''));

-- Link index_snapshots to commit rows
ALTER TABLE index_snapshots
  ADD COLUMN commit_id UUID;

ALTER TABLE index_snapshots
  ADD CONSTRAINT fk_snapshots_commit
    FOREIGN KEY (project_id, commit_id)
    REFERENCES commits(project_id, id) ON DELETE SET NULL (commit_id);

CREATE INDEX idx_index_snapshots_commit_id
  ON index_snapshots(commit_id);

CREATE UNIQUE INDEX idx_index_snapshots_project_active
  ON index_snapshots(project_id, branch)
  WHERE is_active = TRUE;
