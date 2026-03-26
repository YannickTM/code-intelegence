-- name: InsertExport :one
INSERT INTO exports (
  project_id, index_snapshot_id, file_id, export_kind, exported_name,
  local_name, symbol_id, source_module, line, "column"
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListExportsByFileID :many
SELECT * FROM exports WHERE file_id = $1 ORDER BY line;
