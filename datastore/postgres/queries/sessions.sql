-- name: CreateSession :one
INSERT INTO sessions (
  user_id,
  token_hash,
  ip_address,
  user_agent,
  expires_at
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT
  s.id, s.user_id, s.token_hash, s.ip_address, s.user_agent,
  s.expires_at, s.created_at,
  u.username, u.email, u.display_name, u.avatar_url, u.is_active,
  u.created_at AS user_created_at, u.updated_at AS user_updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.expires_at > NOW()
  AND u.is_active = TRUE;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE user_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= NOW();
