-- name: AdminListUsers :many
SELECT
  u.id, u.username, u.email, u.display_name, u.avatar_url,
  u.is_active, u.created_at, u.updated_at,
  (SELECT COUNT(*) FROM project_members pm WHERE pm.user_id = u.id)::bigint AS project_count,
  COALESCE(
    (SELECT array_agg(upr.role ORDER BY upr.role)
     FROM user_platform_roles upr WHERE upr.user_id = u.id),
    ARRAY[]::text[]
  ) AS platform_roles
FROM users u
WHERE (@search::text = '' OR u.username ILIKE @search OR u.display_name ILIKE @search)
  AND (sqlc.narg('is_active')::boolean IS NULL OR u.is_active = sqlc.narg('is_active')::boolean)
ORDER BY
  CASE WHEN @sort::text = 'username'   THEN u.username END ASC,
  CASE WHEN @sort::text = 'created_at' THEN u.created_at END DESC,
  u.created_at DESC,
  u.id ASC
LIMIT @row_limit OFFSET @row_offset;

-- name: AdminCountUsers :one
SELECT COUNT(*)::bigint AS total
FROM users u
WHERE (@search::text = '' OR u.username ILIKE @search OR u.display_name ILIKE @search)
  AND (sqlc.narg('is_active')::boolean IS NULL OR u.is_active = sqlc.narg('is_active')::boolean);

-- name: AdminGetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: AdminGetUserMemberships :many
SELECT pm.project_id, p.name AS project_name, pm.role, pm.created_at AS joined_at
FROM project_members pm
JOIN projects p ON p.id = pm.project_id
WHERE pm.user_id = $1
ORDER BY pm.created_at ASC;

-- name: AdminUpdateUser :one
UPDATE users
SET
  display_name = CASE WHEN sqlc.narg(display_name)::text IS NOT NULL
                      THEN sqlc.narg(display_name)::text
                      ELSE display_name END,
  avatar_url   = CASE WHEN sqlc.narg(avatar_url)::text IS NOT NULL
                      THEN NULLIF(sqlc.narg(avatar_url)::text, '')
                      ELSE avatar_url END,
  updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ActivateUser :exec
UPDATE users
SET is_active = TRUE, updated_at = NOW()
WHERE id = $1;
