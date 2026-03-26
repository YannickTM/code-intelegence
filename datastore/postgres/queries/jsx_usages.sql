-- name: InsertJsxUsage :one
INSERT INTO jsx_usages (
  project_id, index_snapshot_id, file_id, source_symbol_id,
  component_name, is_intrinsic, is_fragment, line, "column",
  resolved_target_symbol_id, confidence
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListJsxUsagesByFileID :many
SELECT * FROM jsx_usages WHERE file_id = $1 ORDER BY line;
