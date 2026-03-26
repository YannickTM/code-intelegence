-- name: GetSymbolByID :one
-- Fetch a single symbol by ID with joined file metadata.
SELECT s.id, s.project_id, s.index_snapshot_id, s.file_id,
       s.name, s.qualified_name, s.kind, s.signature,
       s.start_line, s.end_line, s.doc_text, s.symbol_hash,
       s.parent_symbol_id,
       s.flags, s.modifiers, s.return_type, s.parameter_types,
       s.created_at,
       f.file_path, f.language
FROM symbols s
JOIN files f ON f.id = s.file_id
WHERE s.id = $1
  AND s.project_id = $2;

-- name: ListSymbolChildren :many
-- List child symbols (e.g. methods of a class) ordered by source position.
SELECT s.id, s.project_id, s.index_snapshot_id, s.file_id,
       s.name, s.qualified_name, s.kind, s.signature,
       s.start_line, s.end_line, s.doc_text, s.symbol_hash,
       s.parent_symbol_id, s.created_at,
       f.file_path
FROM symbols s
JOIN files f ON f.id = s.file_id
WHERE s.parent_symbol_id = $1
  AND s.project_id = $2
ORDER BY s.start_line, s.id;
