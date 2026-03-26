-- name: CreateAPIKey :one
INSERT INTO api_keys (
  key_type,
  key_prefix,
  key_hash,
  name,
  role,
  project_id,
  created_by,
  expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListProjectKeys :many
SELECT *
FROM api_keys
WHERE project_id = $1
  AND key_type = 'project'
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY created_at DESC;

-- name: ListPersonalKeys :many
SELECT *
FROM api_keys
WHERE created_by = $1
  AND key_type = 'personal'
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY created_at DESC;

-- name: GetAPIKeyByID :one
SELECT *
FROM api_keys
WHERE id = $1;

-- name: SoftDeleteAPIKey :execrows
UPDATE api_keys
SET is_active = FALSE
WHERE id = $1
  AND is_active = TRUE;

-- name: CheckAPIKeyProjectAccess :one
SELECT effective_role
FROM api_key_project_access
WHERE key_hash = $1
  AND project_id = $2
LIMIT 1;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = NOW()
WHERE key_hash = $1
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: GetAPIKeyByHash :one
SELECT *
FROM api_keys
WHERE key_hash = $1
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: CountProjectKeys :one
SELECT COUNT(*) AS count
FROM api_keys
WHERE project_id = $1
  AND key_type = 'project'
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: CountPersonalKeys :one
SELECT COUNT(*) AS count
FROM api_keys
WHERE created_by = $1
  AND key_type = 'personal'
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: LockProjectRow :one
-- Acquires a row-level lock on a project; use inside a transaction.
SELECT id FROM projects WHERE id = $1 FOR UPDATE;

-- name: LockUserRow :one
-- Acquires a row-level lock on a user; use inside a transaction.
SELECT id FROM users WHERE id = $1 FOR UPDATE;
