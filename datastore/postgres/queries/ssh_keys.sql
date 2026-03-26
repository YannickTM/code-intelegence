-- name: CreateSSHKey :one
INSERT INTO ssh_keys (
  name,
  public_key,
  private_key_encrypted,
  key_type,
  fingerprint,
  created_by
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSSHKey :one
SELECT *
FROM ssh_keys
WHERE id = $1
  AND created_by = $2;

-- name: ListSSHKeys :many
SELECT *
FROM ssh_keys
WHERE created_by = $1
ORDER BY created_at DESC;

-- name: RetireSSHKey :one
UPDATE ssh_keys
SET
  is_active = FALSE,
  rotated_at = NOW()
WHERE id = $1
  AND created_by = $2
RETURNING *;

-- name: ListProjectsBySSHKey :many
SELECT p.*
FROM projects p
JOIN project_ssh_key_assignments psa
  ON psa.project_id = p.id
JOIN ssh_keys sk
  ON sk.id = psa.ssh_key_id
WHERE psa.ssh_key_id = $1
  AND psa.is_active = TRUE
  AND sk.created_by = $2
ORDER BY p.created_at DESC;

-- name: CountActiveAssignmentsByKey :one
SELECT COUNT(*)
FROM project_ssh_key_assignments
WHERE ssh_key_id = $1
  AND is_active = TRUE;

-- name: RetireSSHKeyIfNoAssignments :one
UPDATE ssh_keys
SET
  is_active = FALSE,
  rotated_at = NOW()
WHERE ssh_keys.id = $1
  AND ssh_keys.created_by = $2
  AND NOT EXISTS (
    SELECT 1 FROM project_ssh_key_assignments
    WHERE project_ssh_key_assignments.ssh_key_id = $1
      AND project_ssh_key_assignments.is_active = TRUE
  )
RETURNING *;
