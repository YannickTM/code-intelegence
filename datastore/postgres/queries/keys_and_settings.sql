-- ===================== Embedding Provider Configs =====================

-- name: GetEmbeddingProviderConfigByID :one
SELECT *
FROM embedding_provider_configs
WHERE id = $1
LIMIT 1;

-- name: GetDefaultGlobalEmbeddingProviderConfig :one
SELECT *
FROM embedding_provider_configs
WHERE project_id IS NULL
  AND is_active = TRUE
  AND is_default = TRUE
LIMIT 1;

-- name: ListAvailableGlobalEmbeddingProviderConfigs :many
SELECT
  id,
  name,
  provider,
  endpoint_url,
  model,
  dimensions,
  max_tokens,
  settings_json,
  COALESCE(octet_length(credentials_encrypted), 0) > 0 AS has_credentials,
  is_active,
  is_default,
  is_available_to_projects,
  project_id,
  created_at,
  updated_at
FROM embedding_provider_configs
WHERE project_id IS NULL
  AND is_active = TRUE
  AND is_available_to_projects = TRUE
ORDER BY is_default DESC, created_at ASC, id ASC;

-- name: GetActiveProjectEmbeddingProviderConfig :one
SELECT *
FROM embedding_provider_configs
WHERE project_id = $1
  AND is_active = TRUE
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: DeactivateProjectEmbeddingProviderConfigs :exec
UPDATE embedding_provider_configs
SET
  is_active = FALSE,
  credentials_encrypted = NULL,
  updated_at = NOW()
WHERE project_id = $1
  AND is_active = TRUE;

-- name: CreateEmbeddingProviderConfig :one
INSERT INTO embedding_provider_configs (
  name,
  provider,
  endpoint_url,
  model,
  dimensions,
  max_tokens,
  settings_json,
  credentials_encrypted,
  is_active,
  is_default,
  is_available_to_projects,
  project_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, TRUE, FALSE, FALSE, $9
)
RETURNING *;

-- name: CreateEmbeddingDefaultProviderConfigIfMissing :exec
INSERT INTO embedding_provider_configs (
  name,
  provider,
  endpoint_url,
  model,
  dimensions,
  max_tokens,
  settings_json,
  is_active,
  is_default,
  is_available_to_projects,
  project_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, TRUE, TRUE, TRUE, NULL
)
ON CONFLICT (is_default)
  WHERE is_default = TRUE AND project_id IS NULL
DO NOTHING;

-- name: CreateEmbeddingVersion :one
INSERT INTO embedding_versions (
  embedding_provider_config_id,
  provider,
  model,
  dimensions,
  version_label
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetEmbeddingVersionByLabel :one
SELECT * FROM embedding_versions WHERE version_label = $1;

-- ===================== LLM Provider Configs =====================

-- name: GetLLMProviderConfigByID :one
SELECT *
FROM llm_provider_configs
WHERE id = $1
LIMIT 1;

-- name: GetDefaultGlobalLLMProviderConfig :one
SELECT *
FROM llm_provider_configs
WHERE project_id IS NULL
  AND is_active = TRUE
  AND is_default = TRUE
LIMIT 1;

-- name: ListAvailableGlobalLLMProviderConfigs :many
SELECT
  id,
  name,
  provider,
  endpoint_url,
  model,
  settings_json,
  COALESCE(octet_length(credentials_encrypted), 0) > 0 AS has_credentials,
  is_active,
  is_default,
  is_available_to_projects,
  project_id,
  created_at,
  updated_at
FROM llm_provider_configs
WHERE project_id IS NULL
  AND is_active = TRUE
  AND is_available_to_projects = TRUE
ORDER BY is_default DESC, created_at ASC, id ASC;

-- name: GetActiveProjectLLMProviderConfig :one
SELECT *
FROM llm_provider_configs
WHERE project_id = $1
  AND is_active = TRUE
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: DeactivateProjectLLMProviderConfigs :exec
UPDATE llm_provider_configs
SET
  is_active = FALSE,
  credentials_encrypted = NULL,
  updated_at = NOW()
WHERE project_id = $1
  AND is_active = TRUE;

-- name: CreateLLMProviderConfig :one
INSERT INTO llm_provider_configs (
  name,
  provider,
  endpoint_url,
  model,
  settings_json,
  credentials_encrypted,
  is_active,
  is_default,
  is_available_to_projects,
  project_id
) VALUES (
  $1, $2, $3, $4, $5, $6, TRUE, FALSE, FALSE, $7
)
RETURNING *;

-- name: CreateLLMDefaultProviderConfigIfMissing :exec
INSERT INTO llm_provider_configs (
  name,
  provider,
  endpoint_url,
  model,
  settings_json,
  is_active,
  is_default,
  is_available_to_projects,
  project_id
) VALUES (
  $1, $2, $3, $4, $5, TRUE, TRUE, TRUE, NULL
)
ON CONFLICT (is_default)
  WHERE is_default = TRUE AND project_id IS NULL
DO NOTHING;

-- ===================== Project Provider Selections =====================

-- name: GetProjectProviderSelections :one
SELECT
  selected_embedding_global_config_id,
  selected_llm_global_config_id
FROM projects
WHERE id = $1;

-- name: SetSelectedEmbeddingGlobalConfig :exec
UPDATE projects
SET
  selected_embedding_global_config_id = $2,
  updated_at = NOW()
WHERE id = $1;

-- name: ClearSelectedEmbeddingGlobalConfig :exec
UPDATE projects
SET
  selected_embedding_global_config_id = NULL,
  updated_at = NOW()
WHERE id = $1;

-- name: SetSelectedLLMGlobalConfig :exec
UPDATE projects
SET
  selected_llm_global_config_id = $2,
  updated_at = NOW()
WHERE id = $1;

-- name: ClearSelectedLLMGlobalConfig :exec
UPDATE projects
SET
  selected_llm_global_config_id = NULL,
  updated_at = NOW()
WHERE id = $1;

-- ===================== Global Provider Config Management =====================

-- name: ListGlobalEmbeddingProviderConfigs :many
SELECT *
FROM embedding_provider_configs
WHERE project_id IS NULL
ORDER BY is_default DESC, is_active DESC, created_at DESC;

-- name: DeactivateGlobalEmbeddingProviderConfigs :exec
-- DEPRECATED: Use DeactivateDefaultGlobalEmbeddingProviderConfig instead.
-- This deactivates ALL global rows, which breaks multi-provider support.
UPDATE embedding_provider_configs
SET is_active = FALSE, is_default = FALSE, updated_at = NOW()
WHERE project_id IS NULL AND (is_active = TRUE OR is_default = TRUE);

-- name: DeactivateDefaultGlobalEmbeddingProviderConfig :exec
-- Deactivates only the current default global config, leaving non-default configs untouched.
UPDATE embedding_provider_configs
SET is_active = FALSE, is_default = FALSE, updated_at = NOW()
WHERE project_id IS NULL AND is_default = TRUE AND is_active = TRUE;

-- name: CreateGlobalEmbeddingProviderConfig :one
INSERT INTO embedding_provider_configs (
  name, provider, endpoint_url, model, dimensions, max_tokens,
  settings_json, credentials_encrypted,
  is_active, is_default, is_available_to_projects, project_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8,
  TRUE, TRUE, $9, NULL
)
RETURNING *;

-- name: GetGlobalEmbeddingProviderConfigByID :one
SELECT *
FROM embedding_provider_configs
WHERE id = $1 AND project_id IS NULL
LIMIT 1;

-- name: CreateGlobalEmbeddingProviderConfigNonDefault :one
-- Creates a new non-default global config. is_default is always FALSE.
INSERT INTO embedding_provider_configs (
  name, provider, endpoint_url, model, dimensions, max_tokens,
  settings_json, credentials_encrypted,
  is_active, is_default, is_available_to_projects, project_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8,
  TRUE, FALSE, $9, NULL
)
RETURNING *;

-- name: DeactivateGlobalEmbeddingProviderConfigByID :execrows
-- Soft-deletes a single global config by ID.
-- The is_default = FALSE guard prevents deleting a config that was concurrently promoted.
UPDATE embedding_provider_configs
SET is_active = FALSE, is_default = FALSE, credentials_encrypted = NULL, updated_at = NOW()
WHERE id = $1 AND project_id IS NULL AND is_active = TRUE AND is_default = FALSE;

-- name: DemoteGlobalEmbeddingProviderConfigDefault :exec
-- Clears is_default on the current default. Step 1 of the promote transaction.
UPDATE embedding_provider_configs
SET is_default = FALSE, updated_at = NOW()
WHERE project_id IS NULL AND is_default = TRUE;

-- name: PromoteGlobalEmbeddingProviderConfigToDefault :execrows
-- Sets is_default=TRUE on the target config. Step 2 of the promote transaction.
UPDATE embedding_provider_configs
SET is_default = TRUE, updated_at = NOW()
WHERE id = $1 AND project_id IS NULL AND is_active = TRUE;

-- name: CountProjectsUsingEmbeddingConfig :one
SELECT COUNT(*)::bigint AS total
FROM projects
WHERE selected_embedding_global_config_id = $1;

-- name: UpdateGlobalEmbeddingProviderConfig :one
-- In-place partial update of a global config. Only provided (non-NULL) fields are changed.
UPDATE embedding_provider_configs
SET
  name = COALESCE(sqlc.narg(name), name),
  provider = COALESCE(sqlc.narg(provider), provider),
  endpoint_url = COALESCE(sqlc.narg(endpoint_url), endpoint_url),
  model = COALESCE(sqlc.narg(model), model),
  dimensions = COALESCE(sqlc.narg(dimensions), dimensions),
  max_tokens = COALESCE(sqlc.narg(max_tokens), max_tokens),
  settings_json = COALESCE(sqlc.narg(settings_json)::jsonb, settings_json),
  credentials_encrypted = CASE
    WHEN sqlc.arg(clear_credentials)::boolean THEN NULL
    WHEN sqlc.narg(credentials_encrypted)::bytea IS NOT NULL THEN sqlc.narg(credentials_encrypted)::bytea
    ELSE credentials_encrypted
  END,
  is_available_to_projects = COALESCE(sqlc.narg(is_available_to_projects), is_available_to_projects),
  updated_at = NOW()
WHERE id = sqlc.arg(id) AND project_id IS NULL AND is_active = TRUE
RETURNING *;

-- name: ListGlobalLLMProviderConfigs :many
SELECT *
FROM llm_provider_configs
WHERE project_id IS NULL
ORDER BY is_default DESC, is_active DESC, created_at DESC;

-- name: DeactivateGlobalLLMProviderConfigs :exec
-- DEPRECATED: Use DeactivateDefaultGlobalLLMProviderConfig instead.
-- This deactivates ALL global rows, which breaks multi-provider support.
UPDATE llm_provider_configs
SET is_active = FALSE, is_default = FALSE, updated_at = NOW()
WHERE project_id IS NULL AND (is_active = TRUE OR is_default = TRUE);

-- name: DeactivateDefaultGlobalLLMProviderConfig :exec
-- Deactivates only the current default global config, leaving non-default configs untouched.
UPDATE llm_provider_configs
SET is_active = FALSE, is_default = FALSE, updated_at = NOW()
WHERE project_id IS NULL AND is_default = TRUE AND is_active = TRUE;

-- name: CreateGlobalLLMProviderConfig :one
INSERT INTO llm_provider_configs (
  name, provider, endpoint_url, model,
  settings_json, credentials_encrypted,
  is_active, is_default, is_available_to_projects, project_id
) VALUES (
  $1, $2, $3, $4, $5, $6,
  TRUE, TRUE, $7, NULL
)
RETURNING *;

-- name: GetGlobalLLMProviderConfigByID :one
SELECT *
FROM llm_provider_configs
WHERE id = $1 AND project_id IS NULL
LIMIT 1;

-- name: CreateGlobalLLMProviderConfigNonDefault :one
-- Creates a new non-default global LLM config. is_default is always FALSE.
INSERT INTO llm_provider_configs (
  name, provider, endpoint_url, model,
  settings_json, credentials_encrypted,
  is_active, is_default, is_available_to_projects, project_id
) VALUES (
  $1, $2, $3, $4, $5, $6,
  TRUE, FALSE, $7, NULL
)
RETURNING *;

-- name: DeactivateGlobalLLMProviderConfigByID :execrows
-- Soft-deletes a single global LLM config by ID.
-- The is_default = FALSE guard prevents deleting a config that was concurrently promoted.
UPDATE llm_provider_configs
SET is_active = FALSE, is_default = FALSE, credentials_encrypted = NULL, updated_at = NOW()
WHERE id = $1 AND project_id IS NULL AND is_active = TRUE AND is_default = FALSE;

-- name: DemoteGlobalLLMProviderConfigDefault :exec
-- Clears is_default on the current default LLM config. Step 1 of the promote transaction.
UPDATE llm_provider_configs
SET is_default = FALSE, updated_at = NOW()
WHERE project_id IS NULL AND is_default = TRUE;

-- name: PromoteGlobalLLMProviderConfigToDefault :execrows
-- Sets is_default=TRUE on the target LLM config. Step 2 of the promote transaction.
UPDATE llm_provider_configs
SET is_default = TRUE, updated_at = NOW()
WHERE id = $1 AND project_id IS NULL AND is_active = TRUE;

-- name: CountProjectsUsingLLMConfig :one
SELECT COUNT(*)::bigint AS total
FROM projects
WHERE selected_llm_global_config_id = $1;

-- name: UpdateGlobalLLMProviderConfig :one
-- In-place partial update of a global LLM config. Only provided (non-NULL) fields are changed.
UPDATE llm_provider_configs
SET
  name = COALESCE(sqlc.narg(name), name),
  provider = COALESCE(sqlc.narg(provider), provider),
  endpoint_url = COALESCE(sqlc.narg(endpoint_url), endpoint_url),
  model = COALESCE(sqlc.narg(model), model),
  settings_json = COALESCE(sqlc.narg(settings_json)::jsonb, settings_json),
  credentials_encrypted = CASE
    WHEN sqlc.arg(clear_credentials)::boolean THEN NULL
    WHEN sqlc.narg(credentials_encrypted)::bytea IS NOT NULL THEN sqlc.narg(credentials_encrypted)::bytea
    ELSE credentials_encrypted
  END,
  is_available_to_projects = COALESCE(sqlc.narg(is_available_to_projects), is_available_to_projects),
  updated_at = NOW()
WHERE id = sqlc.arg(id) AND project_id IS NULL AND is_active = TRUE
RETURNING *;
