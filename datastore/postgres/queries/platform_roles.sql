-- name: HasPlatformRole :one
SELECT EXISTS (
  SELECT 1 FROM user_platform_roles
  WHERE user_id = $1 AND role = $2
) AS has_role;

-- name: GetUserPlatformRoles :many
SELECT role
FROM user_platform_roles
WHERE user_id = $1;

-- name: ListActivePlatformRoleAssignments :many
-- Returns role assignments for active users only. Deactivated users are
-- excluded because their roles are effectively dormant (they cannot log in).
SELECT upr.id, upr.user_id, upr.role, upr.granted_by, upr.created_at,
       u.username, u.display_name, u.avatar_url
FROM user_platform_roles upr
JOIN users u ON u.id = upr.user_id
WHERE u.is_active = TRUE
ORDER BY upr.created_at ASC;

-- name: GrantPlatformRole :one
-- Only grants when the target user is active, preventing TOCTOU races
-- where a concurrent deactivation could allow granting to an inactive user.
INSERT INTO user_platform_roles (user_id, role, granted_by)
SELECT $1, $2, $3
FROM users WHERE id = $1 AND is_active = TRUE
ON CONFLICT (user_id, role) DO NOTHING
RETURNING *;

-- name: RevokePlatformRole :execrows
DELETE FROM user_platform_roles
WHERE user_id = $1 AND role = $2;

-- name: CountActivePlatformAdmins :one
SELECT COUNT(*)::bigint AS total
FROM user_platform_roles upr
JOIN users u ON u.id = upr.user_id
WHERE upr.role = 'platform_admin'
  AND u.is_active = TRUE;

-- name: CountActivePlatformAdminsForUpdate :one
-- Locking variant: acquires FOR UPDATE on both user_platform_roles AND users rows
-- to serialize concurrent attempts to deactivate or revoke the last platform admin.
-- Locking users rows too is critical: the deactivation flow sets users.is_active = FALSE,
-- so without locking users a concurrent deactivation could bypass the role-only lock.
-- FOR UPDATE cannot be used with aggregate functions directly, so we lock
-- the matching rows in a subquery first, then count.
SELECT COUNT(*)::bigint AS total
FROM (
  SELECT upr.id
  FROM user_platform_roles upr
  JOIN users u ON u.id = upr.user_id
  WHERE upr.role = 'platform_admin'
    AND u.is_active = TRUE
  FOR UPDATE OF upr, u
) locked;

-- name: UpsertPlatformRoleByUsername :execresult
INSERT INTO user_platform_roles (user_id, role)
SELECT u.id, 'platform_admin'
FROM users u
WHERE u.username = $1
  AND u.is_active = TRUE
ON CONFLICT (user_id, role) DO NOTHING;
