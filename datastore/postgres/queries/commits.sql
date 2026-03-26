-- =============================================
-- File contents
-- =============================================

-- name: UpsertFileContent :one
INSERT INTO file_contents (
  project_id,
  content_hash,
  content,
  size_bytes,
  line_count
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (project_id, content_hash) DO UPDATE
  SET project_id = file_contents.project_id
RETURNING *;

-- name: GetFileContentByHash :one
SELECT *
FROM file_contents
WHERE project_id = $1 AND content_hash = $2;

-- name: GetFileContentByID :one
SELECT *
FROM file_contents
WHERE project_id = $1 AND id = $2;

-- name: SetFileContentAST :execrows
UPDATE file_contents
SET tree_sitter_ast = $3
WHERE project_id = $1 AND id = $2;

-- name: UpdateFileContentRef :execrows
UPDATE files
SET file_content_id = $3
WHERE project_id = $1 AND id = $2;

-- name: DeleteProjectFileContents :exec
DELETE FROM file_contents
WHERE project_id = $1;

-- =============================================
-- Commits
-- =============================================

-- name: InsertCommit :one
INSERT INTO commits (
  project_id,
  commit_hash,
  author_name,
  author_email,
  author_date,
  committer_name,
  committer_email,
  committer_date,
  message
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (project_id, commit_hash) DO UPDATE
  SET project_id = commits.project_id
RETURNING *;

-- name: GetCommitByHash :one
SELECT *
FROM commits
WHERE project_id = $1 AND commit_hash = $2;

-- name: GetCommitByID :one
SELECT *
FROM commits
WHERE project_id = $1 AND id = $2;

-- name: ListProjectCommits :many
SELECT *
FROM commits
WHERE project_id = $1
ORDER BY committer_date DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountProjectCommits :one
SELECT COUNT(*)::bigint AS total
FROM commits
WHERE project_id = $1;

-- name: ListCommitsBetween :many
SELECT *
FROM commits
WHERE project_id = $1
  AND committer_date >= $2
  AND committer_date <= $3
ORDER BY committer_date DESC, id DESC;

-- name: DeleteProjectCommits :exec
DELETE FROM commits
WHERE project_id = $1;

-- =============================================
-- Commit parents
-- =============================================

-- name: InsertCommitParent :exec
INSERT INTO commit_parents (project_id, commit_id, parent_commit_id, ordinal)
VALUES ($1, $2, $3, $4)
ON CONFLICT (project_id, commit_id, parent_commit_id) DO NOTHING;

-- name: GetCommitParents :many
SELECT cp.*, c.commit_hash AS parent_hash
FROM commit_parents cp
JOIN commits c ON c.id = cp.parent_commit_id
WHERE cp.project_id = $1 AND cp.commit_id = $2
ORDER BY cp.ordinal;

-- name: GetCommitChildren :many
SELECT cp.*, c.commit_hash AS child_hash
FROM commit_parents cp
JOIN commits c ON c.id = cp.commit_id
WHERE cp.project_id = $1 AND cp.parent_commit_id = $2
ORDER BY cp.ordinal, c.commit_hash;

-- =============================================
-- Commit file diffs
-- =============================================

-- name: InsertCommitFileDiff :one
INSERT INTO commit_file_diffs (
  project_id,
  commit_id,
  parent_commit_id,
  old_file_path,
  new_file_path,
  change_type,
  patch,
  additions,
  deletions
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (project_id, commit_id, COALESCE(old_file_path, ''), COALESCE(new_file_path, ''))
  DO UPDATE SET project_id = commit_file_diffs.project_id
RETURNING *;

-- name: ListCommitFileDiffs :many
SELECT *
FROM commit_file_diffs
WHERE project_id = $1 AND commit_id = $2
ORDER BY COALESCE(new_file_path, old_file_path)
LIMIT $3 OFFSET $4;

-- name: ListCommitFileDiffsMeta :many
SELECT id, project_id, commit_id, parent_commit_id,
       old_file_path, new_file_path, change_type,
       additions, deletions, created_at
FROM commit_file_diffs
WHERE project_id = $1 AND commit_id = $2
ORDER BY COALESCE(new_file_path, old_file_path)
LIMIT $3 OFFSET $4;

-- name: CountCommitFileDiffs :one
SELECT COUNT(*)::bigint AS total
FROM commit_file_diffs
WHERE project_id = $1 AND commit_id = $2;

-- name: GetCommitFileDiff :one
SELECT *
FROM commit_file_diffs
WHERE project_id = $1 AND commit_id = $2 AND id = $3;

-- name: GetCommitDiffStats :one
SELECT
  COUNT(*)::bigint AS files_changed,
  COALESCE(SUM(additions), 0)::bigint AS total_additions,
  COALESCE(SUM(deletions), 0)::bigint AS total_deletions
FROM commit_file_diffs
WHERE project_id = $1 AND commit_id = $2;

-- name: ListFileDiffsByPath :many
SELECT cfd.*
FROM commit_file_diffs cfd
JOIN commits c ON c.id = cfd.commit_id
WHERE cfd.project_id = sqlc.arg(project_id)
  AND (cfd.new_file_path = sqlc.arg(file_path) OR cfd.old_file_path = sqlc.arg(file_path))
ORDER BY c.committer_date DESC, cfd.id DESC
LIMIT sqlc.arg(query_limit) OFFSET sqlc.arg(query_offset);

-- name: ListFileDiffsByPathWithCommit :many
SELECT
  cfd.id         AS diff_id,
  c.commit_hash,
  c.author_name,
  c.committer_date,
  c.message,
  cfd.change_type,
  cfd.additions,
  cfd.deletions
FROM commit_file_diffs cfd
JOIN commits c ON c.id = cfd.commit_id
WHERE cfd.project_id = sqlc.arg(project_id)
  AND (sqlc.arg(file_path) = cfd.new_file_path OR sqlc.arg(file_path) = cfd.old_file_path)
ORDER BY c.committer_date DESC, cfd.id DESC
LIMIT sqlc.arg(query_limit) OFFSET sqlc.arg(query_offset);

-- name: CountFileDiffsByPath :one
SELECT count(*)::bigint AS total
FROM commit_file_diffs cfd
WHERE cfd.project_id = sqlc.arg(project_id)
  AND (sqlc.arg(file_path) = cfd.new_file_path OR sqlc.arg(file_path) = cfd.old_file_path);

-- =============================================
-- Snapshot ↔ Commit link
-- =============================================

-- name: UpdateSnapshotCommitID :execrows
UPDATE index_snapshots
SET commit_id = $3
WHERE project_id = $1 AND id = $2;
