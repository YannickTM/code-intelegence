-- name: InsertSymbolReference :one
INSERT INTO symbol_references (
  project_id, index_snapshot_id, file_id, source_symbol_id,
  reference_kind, raw_text, target_name, qualified_target_hint,
  start_line, start_column, end_line, end_column,
  resolution_scope, resolved_target_symbol_id, confidence
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: ListSymbolReferencesByFileID :many
SELECT * FROM symbol_references WHERE file_id = $1 ORDER BY start_line;
