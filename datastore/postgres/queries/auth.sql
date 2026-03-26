-- name: CountUsers :one
SELECT COUNT(*)::bigint AS total
FROM users;

-- name: CreateUser :one
INSERT INTO users (
  username,
  email,
  display_name,
  is_active
) VALUES ($1, $2, $3, TRUE)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
SELECT *
FROM users
WHERE username = $1
  AND is_active = TRUE
LIMIT 1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1
  AND is_active = TRUE
LIMIT 1;

-- name: UpdateUserProfile :one
UPDATE users
SET
  display_name = COALESCE($2, display_name),
  avatar_url = CASE WHEN sqlc.narg(avatar_url)::text IS NOT NULL THEN NULLIF(sqlc.narg(avatar_url)::text, '') ELSE avatar_url END,
  email = COALESCE(sqlc.narg(email), email),
  updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListUsers :many
SELECT *
FROM users
WHERE is_active = TRUE
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: DeactivateUser :exec
UPDATE users
SET
  is_active = FALSE,
  updated_at = NOW()
WHERE id = $1;

-- name: CreateProjectMember :one
INSERT INTO project_members (
  project_id,
  user_id,
  role,
  invited_by
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetProjectMember :one
SELECT *
FROM project_members
WHERE project_id = $1
  AND user_id = $2;

-- name: GetProjectMemberForUpdate :one
-- Fetches a project member row while acquiring a FOR UPDATE lock.
-- Use inside a transaction to prevent TOCTOU races on role checks.
SELECT *
FROM project_members
WHERE project_id = $1
  AND user_id = $2
FOR UPDATE;

-- name: ListProjectMembers :many
SELECT pm.*, u.username, u.display_name, u.avatar_url
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE pm.project_id = $1
  AND u.is_active = TRUE
ORDER BY pm.created_at ASC;

-- name: UpdateProjectMemberRole :one
UPDATE project_members
SET
  role = $3,
  updated_at = NOW()
WHERE project_id = $1
  AND user_id = $2
RETURNING *;

-- name: DeleteProjectMember :exec
DELETE FROM project_members
WHERE project_id = $1
  AND user_id = $2;

-- name: CountProjectOwners :one
SELECT COUNT(*)::bigint AS total
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE pm.project_id = $1
  AND pm.role = 'owner'
  AND u.is_active = TRUE;

-- name: CountProjectOwnersForUpdate :one
-- Counts project owners while acquiring row-level locks on the matching
-- project_members rows. Use inside a transaction to serialize concurrent
-- owner demotion / removal and prevent the last-owner invariant from
-- being violated by race conditions.
-- NOTE: FOR UPDATE cannot be used directly with aggregates, so we lock
-- the rows in a subquery and count the locked result set.
SELECT COUNT(*)::bigint AS total
FROM (
  SELECT pm.id
  FROM project_members pm
  JOIN users u ON u.id = pm.user_id
  WHERE pm.project_id = $1
    AND pm.role = 'owner'
    AND u.is_active = TRUE
  FOR UPDATE OF pm
) locked;

-- name: ListUserProjects :many
SELECT p.*, pm.role
FROM projects p
JOIN project_members pm ON pm.project_id = p.id
WHERE pm.user_id = $1
ORDER BY p.created_at DESC;

-- name: ListUserProjectIDs :many
SELECT pm.project_id
FROM project_members pm
WHERE pm.user_id = $1;

-- name: ListUserProjectsWithHealth :many
SELECT
  p.*, pm.role,
  snap.id                      AS index_snapshot_id,
  COALESCE(snap.git_commit, '') AS index_git_commit,
  COALESCE(snap.branch, '')    AS index_branch,
  snap.activated_at            AS index_activated_at,
  active_job.id                AS active_job_id,
  COALESCE(active_job.status, '') AS active_job_status,
  COALESCE(active_job.worker_id, '') AS active_job_worker_id,
  active_job.started_at        AS active_job_started_at,
  failed_job.id                AS failed_job_id,
  failed_job.finished_at       AS failed_job_finished_at,
  COALESCE(failed_job.job_type, '') AS failed_job_type
FROM projects p
JOIN project_members pm ON pm.project_id = p.id
LEFT JOIN LATERAL (
  SELECT s.id, s.git_commit, s.branch, s.activated_at
  FROM index_snapshots s
  WHERE s.project_id = p.id AND s.is_active = TRUE
  ORDER BY s.activated_at DESC NULLS LAST, s.id DESC LIMIT 1
) snap ON TRUE
LEFT JOIN LATERAL (
  SELECT j.id, j.status, j.worker_id, j.started_at FROM indexing_jobs j
  WHERE j.project_id = p.id AND j.status IN ('queued', 'running')
  ORDER BY j.created_at DESC, j.id DESC LIMIT 1
) active_job ON TRUE
LEFT JOIN LATERAL (
  SELECT j.id, j.finished_at, j.job_type FROM indexing_jobs j
  WHERE j.project_id = p.id AND j.status = 'failed'
    AND j.finished_at >= NOW() - INTERVAL '24 hours'
    AND NOT EXISTS (
      SELECT 1 FROM indexing_jobs j2
      WHERE j2.project_id = p.id
        AND j2.status = 'completed'
        AND j2.finished_at > j.finished_at
    )
  ORDER BY j.finished_at DESC, j.id DESC LIMIT 1
) failed_job ON TRUE
WHERE pm.user_id = $1
ORDER BY p.created_at DESC;

-- name: ListAllProjectsWithHealth :many
SELECT
  p.*, 'owner'::text AS role,
  snap.id                      AS index_snapshot_id,
  COALESCE(snap.git_commit, '') AS index_git_commit,
  COALESCE(snap.branch, '')    AS index_branch,
  snap.activated_at            AS index_activated_at,
  active_job.id                AS active_job_id,
  COALESCE(active_job.status, '') AS active_job_status,
  COALESCE(active_job.worker_id, '') AS active_job_worker_id,
  active_job.started_at        AS active_job_started_at,
  failed_job.id                AS failed_job_id,
  failed_job.finished_at       AS failed_job_finished_at,
  COALESCE(failed_job.job_type, '') AS failed_job_type
FROM projects p
LEFT JOIN LATERAL (
  SELECT s.id, s.git_commit, s.branch, s.activated_at
  FROM index_snapshots s
  WHERE s.project_id = p.id AND s.is_active = TRUE
  ORDER BY s.activated_at DESC NULLS LAST, s.id DESC LIMIT 1
) snap ON TRUE
LEFT JOIN LATERAL (
  SELECT j.id, j.status, j.worker_id, j.started_at FROM indexing_jobs j
  WHERE j.project_id = p.id AND j.status IN ('queued', 'running')
  ORDER BY j.created_at DESC, j.id DESC LIMIT 1
) active_job ON TRUE
LEFT JOIN LATERAL (
  SELECT j.id, j.finished_at, j.job_type FROM indexing_jobs j
  WHERE j.project_id = p.id AND j.status = 'failed'
    AND j.finished_at >= NOW() - INTERVAL '24 hours'
    AND NOT EXISTS (
      SELECT 1 FROM indexing_jobs j2
      WHERE j2.project_id = p.id
        AND j2.status = 'completed'
        AND j2.finished_at > j.finished_at
    )
  ORDER BY j.finished_at DESC, j.id DESC LIMIT 1
) failed_job ON TRUE
ORDER BY p.created_at DESC;
