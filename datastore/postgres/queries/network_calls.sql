-- name: InsertNetworkCall :one
INSERT INTO network_calls (
  project_id, index_snapshot_id, file_id, source_symbol_id,
  client_kind, method, url_literal, url_template, is_relative,
  start_line, start_column, confidence
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ListNetworkCallsByFileID :many
SELECT * FROM network_calls WHERE file_id = $1 ORDER BY start_line;
