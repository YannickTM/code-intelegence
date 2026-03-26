-- name: CreateProject :one
INSERT INTO projects (
  name,
  repo_url,
  default_branch,
  status,
  created_by
) VALUES (
  $1, $2, COALESCE(sqlc.narg(default_branch), 'main'), COALESCE(sqlc.narg(status), 'active'), $3
)
RETURNING *;

-- name: GetProject :one
SELECT *
FROM projects
WHERE id = $1;

-- name: ListProjectsByMember :many
SELECT p.*
FROM projects p
JOIN project_members pm ON pm.project_id = p.id
WHERE pm.user_id = $1
ORDER BY p.created_at DESC, p.id DESC
LIMIT $2 OFFSET $3;

-- name: CountProjectsByMember :one
SELECT count(*)
FROM projects p
JOIN project_members pm ON pm.project_id = p.id
WHERE pm.user_id = $1;

-- name: UpdateProject :one
UPDATE projects
SET
  name = COALESCE(sqlc.narg(name), name),
  repo_url = COALESCE(sqlc.narg(repo_url), repo_url),
  default_branch = COALESCE(sqlc.narg(default_branch), default_branch),
  status = COALESCE(sqlc.narg(status), status),
  updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteProject :exec
DELETE FROM projects
WHERE id = $1;

-- name: GetProjectSSHKeyAssignment :one
SELECT
  psa.id,
  psa.project_id,
  psa.ssh_key_id,
  psa.is_active,
  psa.assigned_at,
  psa.unassigned_at
FROM project_ssh_key_assignments psa
WHERE psa.project_id = $1
  AND psa.is_active = TRUE;

-- name: GetProjectSSHKeyWithDetails :one
SELECT
  sk.id,
  sk.name,
  sk.fingerprint,
  sk.public_key,
  sk.key_type,
  sk.created_at
FROM project_ssh_key_assignments psa
JOIN ssh_keys sk ON sk.id = psa.ssh_key_id
WHERE psa.project_id = $1
  AND psa.is_active = TRUE;

-- name: LockActiveProjectSSHKeyAssignment :one
SELECT psa.id
FROM project_ssh_key_assignments psa
WHERE psa.project_id = $1
  AND psa.is_active = TRUE
FOR UPDATE;

-- name: DeactivateProjectSSHKeyAssignments :exec
UPDATE project_ssh_key_assignments
SET
  is_active = FALSE,
  unassigned_at = NOW()
WHERE project_id = $1
  AND is_active = TRUE;

-- name: AssignProjectSSHKey :one
INSERT INTO project_ssh_key_assignments (
  project_id,
  ssh_key_id,
  is_active
) VALUES ($1, $2, TRUE)
RETURNING *;

-- name: GetProjectWithHealth :one
SELECT
  p.*,
  snap.id                           AS index_snapshot_id,
  COALESCE(snap.git_commit, '')     AS index_git_commit,
  COALESCE(snap.branch, '')         AS index_branch,
  snap.activated_at                 AS index_activated_at,
  active_job.id                     AS active_job_id,
  COALESCE(active_job.status, '')   AS active_job_status,
  COALESCE(active_job.worker_id, '') AS active_job_worker_id,
  active_job.started_at             AS active_job_started_at,
  failed_job.id                     AS failed_job_id,
  failed_job.finished_at            AS failed_job_finished_at,
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
WHERE p.id = $1;

-- name: GetProjectSSHKeyForExecution :one
SELECT
  sk.id,
  sk.name,
  sk.public_key,
  sk.private_key_encrypted,
  sk.key_type,
  sk.fingerprint,
  p.repo_url,
  p.default_branch
FROM projects p
JOIN project_ssh_key_assignments psa ON psa.project_id = p.id
JOIN ssh_keys sk ON sk.id = psa.ssh_key_id
WHERE p.id = $1
  AND psa.is_active = TRUE
  AND sk.is_active = TRUE;
