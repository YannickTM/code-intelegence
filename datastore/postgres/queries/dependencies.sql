-- name: ListDependenciesFromFile :many
-- Forward: what does this file import?
SELECT d.id, d.source_file_path, d.target_file_path,
       d.import_name, d.import_type, d.package_name, d.package_version
FROM dependencies d
WHERE d.project_id = $1
  AND d.index_snapshot_id = $2
  AND d.source_file_path = $3
ORDER BY d.import_name;

-- name: ListDependenciesToFile :many
-- Reverse: what imports this file?
SELECT d.id, d.source_file_path, d.target_file_path,
       d.import_name, d.import_type, d.package_name, d.package_version
FROM dependencies d
WHERE d.project_id = $1
  AND d.index_snapshot_id = $2
  AND d.target_file_path = $3
ORDER BY d.source_file_path, d.import_name;

-- name: ListExternalDependencies :many
-- External packages: target_file_path IS NULL (unresolvable within project).
SELECT d.package_name, d.package_version,
       COUNT(*)::bigint AS import_count,
       COUNT(DISTINCT d.source_file_path)::bigint AS file_count
FROM dependencies d
WHERE d.project_id = $1
  AND d.index_snapshot_id = $2
  AND d.target_file_path IS NULL
  AND d.package_name IS NOT NULL
GROUP BY d.package_name, d.package_version
ORDER BY import_count DESC;

-- name: ListFileDependencyCounts :many
-- Per-file aggregate: how many imports and how many importers.
SELECT sub.file_path,
       SUM(sub.imports_count)::bigint AS imports_count,
       SUM(sub.imported_by_count)::bigint AS imported_by_count
FROM (
  SELECT d1.source_file_path AS file_path,
         COUNT(*)::bigint AS imports_count,
         0::bigint AS imported_by_count
  FROM dependencies d1
  WHERE d1.project_id = $1 AND d1.index_snapshot_id = $2
  GROUP BY d1.source_file_path

  UNION ALL

  SELECT d2.target_file_path AS file_path,
         0::bigint AS imports_count,
         COUNT(*)::bigint AS imported_by_count
  FROM dependencies d2
  WHERE d2.project_id = $1 AND d2.index_snapshot_id = $2
    AND d2.target_file_path IS NOT NULL
  GROUP BY d2.target_file_path
) sub
GROUP BY sub.file_path
ORDER BY sub.file_path
LIMIT $3 OFFSET $4;

-- name: CountFilesWithDependencies :one
SELECT COUNT(DISTINCT file_path)::bigint AS total
FROM (
  SELECT d1.source_file_path AS file_path FROM dependencies d1
  WHERE d1.project_id = $1 AND d1.index_snapshot_id = $2
  UNION
  SELECT d2.target_file_path AS file_path FROM dependencies d2
  WHERE d2.project_id = $1 AND d2.index_snapshot_id = $2
    AND d2.target_file_path IS NOT NULL
) sub;

-- name: ListDependenciesFromFiles :many
-- Batch forward lookup for BFS: all imports from a set of source files.
SELECT d.id, d.source_file_path, d.target_file_path,
       d.import_name, d.import_type, d.package_name, d.package_version
FROM dependencies d
WHERE d.project_id = $1
  AND d.index_snapshot_id = $2
  AND d.source_file_path = ANY($3::text[])
ORDER BY d.source_file_path, d.import_name;

-- name: ListDependenciesToFiles :many
-- Batch reverse lookup for BFS: all files importing any of the given files.
SELECT d.id, d.source_file_path, d.target_file_path,
       d.import_name, d.import_type, d.package_name, d.package_version
FROM dependencies d
WHERE d.project_id = $1
  AND d.index_snapshot_id = $2
  AND d.target_file_path = ANY($3::text[])
ORDER BY d.target_file_path, d.source_file_path;
