-- name: GetActiveSnapshotForProject :one
SELECT *
FROM index_snapshots
WHERE project_id = $1 AND is_active = TRUE
ORDER BY activated_at DESC NULLS LAST, id DESC
LIMIT 1;

-- name: ListSnapshotFiles :many
SELECT *
FROM files
WHERE index_snapshot_id = $1
ORDER BY file_path;

-- name: GetFileWithContent :one
SELECT
  f.id,
  f.file_path,
  f.language,
  f.file_hash,
  fc.size_bytes,
  fc.line_count,
  fc.content,
  fc.content_hash,
  f.file_facts,
  f.issues,
  f.parser_meta,
  f.extractor_statuses
FROM files f
LEFT JOIN file_contents fc ON fc.id = f.file_content_id
WHERE f.index_snapshot_id = $1 AND f.file_path = $2;

-- name: GetActiveSnapshotForBranch :one
SELECT *
FROM index_snapshots
WHERE project_id = $1 AND branch = $2 AND is_active = TRUE
LIMIT 1;

-- name: GetFileBySnapshotAndPath :one
SELECT id FROM files WHERE index_snapshot_id = $1 AND file_path = $2;

-- name: GetFileWithContentAndAST :one
SELECT
  f.id,
  f.file_path,
  f.language,
  f.file_hash,
  fc.size_bytes,
  fc.line_count,
  fc.content,
  fc.tree_sitter_ast,
  fc.content_hash,
  f.file_facts,
  f.issues,
  f.parser_meta,
  f.extractor_statuses
FROM files f
LEFT JOIN file_contents fc ON fc.id = f.file_content_id
WHERE f.index_snapshot_id = $1 AND f.file_path = $2;
