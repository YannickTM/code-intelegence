-- name: CreateIndexSnapshot :one
INSERT INTO index_snapshots (
  project_id,
  branch,
  embedding_version_id,
  git_commit,
  is_active,
  status
) VALUES ($1, $2, $3, $4, FALSE, 'building')
ON CONFLICT (project_id, branch, embedding_version_id, git_commit)
DO UPDATE SET status = 'building', is_active = FALSE, activated_at = NULL
RETURNING *;

-- name: DeactivateActiveSnapshot :exec
UPDATE index_snapshots
SET is_active = FALSE
WHERE project_id = $1
  AND branch = $2
  AND is_active = TRUE;

-- name: ActivateSnapshot :exec
UPDATE index_snapshots
SET
  is_active = TRUE,
  status = 'active',
  activated_at = NOW()
WHERE id = $1;

-- name: CreateIndexingJob :one
INSERT INTO indexing_jobs (
  project_id,
  index_snapshot_id,
  job_type,
  status,
  embedding_provider_config_id,
  llm_provider_config_id
) VALUES ($1, $2, $3, 'queued', $4, $5)
RETURNING *;

-- name: FindActiveIndexingJobForProjectAndType :one
SELECT *
FROM indexing_jobs
WHERE project_id = $1
  AND job_type = $2
  AND status IN ('queued', 'running')
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: SetIndexingJobRunning :exec
UPDATE indexing_jobs
SET
  status = 'running',
  started_at = NOW()
WHERE id = $1;

-- name: SetIndexingJobProgress :exec
UPDATE indexing_jobs
SET
  files_processed = $2,
  chunks_upserted = $3,
  vectors_deleted = $4
WHERE id = $1;

-- name: SetIndexingJobCompleted :exec
UPDATE indexing_jobs
SET
  status = 'completed',
  finished_at = NOW()
WHERE id = $1;

-- name: SetIndexingJobFailed :exec
UPDATE indexing_jobs
SET
  status = 'failed',
  error_details = $2,
  finished_at = NOW()
WHERE id = $1;

-- name: CountProjectJobs :one
SELECT COUNT(*)::bigint AS total
FROM indexing_jobs
WHERE project_id = $1;

-- name: ListProjectJobs :many
SELECT *
FROM indexing_jobs
WHERE project_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountActiveJobsForUser :one
SELECT COUNT(*)::bigint AS total
FROM indexing_jobs ij
JOIN project_members pm ON pm.project_id = ij.project_id
WHERE pm.user_id = $1
  AND ij.status IN ('queued', 'running');

-- name: CountFailedJobsForUser24h :one
SELECT COUNT(*)::bigint AS total
FROM indexing_jobs ij
JOIN project_members pm ON pm.project_id = ij.project_id
WHERE pm.user_id = $1
  AND ij.status = 'failed'
  AND ij.finished_at >= NOW() - INTERVAL '24 hours';

-- name: GetQueryStats24hForUser :one
SELECT
  COUNT(*)::bigint AS query_count,
  COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY ql.latency_ms), 0)::bigint AS p95_latency_ms
FROM query_log ql
JOIN project_members pm ON pm.project_id = ql.project_id
WHERE pm.user_id = $1
  AND ql.created_at >= NOW() - INTERVAL '24 hours';

-- name: GetIndexingJob :one
SELECT * FROM indexing_jobs WHERE id = $1;

-- name: ClaimQueuedIndexingJob :execrows
UPDATE indexing_jobs
SET
  status = 'running',
  started_at = NOW(),
  worker_id = @worker_id
WHERE id = @id
  AND status = 'queued';

-- name: GetIndexSnapshot :one
SELECT * FROM index_snapshots WHERE id = $1;

-- name: DeleteSnapshotArtifacts :exec
DELETE FROM files WHERE index_snapshot_id = $1;

-- name: InsertFile :one
INSERT INTO files (
  project_id, index_snapshot_id, file_path, language, file_hash,
  size_bytes, line_count, file_content_id,
  file_facts, parser_meta, extractor_statuses, issues
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: InsertSymbol :one
INSERT INTO symbols (
  project_id, index_snapshot_id, file_id, name, qualified_name,
  kind, signature, start_line, end_line, doc_text, symbol_hash,
  parent_symbol_id,
  flags, modifiers, return_type, parameter_types
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: InsertCodeChunk :one
INSERT INTO code_chunks (
  project_id, index_snapshot_id, file_id, symbol_id, chunk_type,
  chunk_hash, content, context_before, context_after, start_line,
  end_line, estimated_tokens,
  owner_qualified_name, owner_kind, is_exported_context, semantic_role
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: ListSymbolsByFileID :many
SELECT * FROM symbols WHERE file_id = $1 ORDER BY start_line;

-- name: ListChunksByFileID :many
SELECT * FROM code_chunks WHERE file_id = $1 ORDER BY start_line;

-- name: ListDependenciesBySnapshotAndFile :many
SELECT * FROM dependencies
WHERE index_snapshot_id = $1 AND source_file_path = $2;

-- name: InsertDependency :one
INSERT INTO dependencies (
  project_id, index_snapshot_id, source_symbol_id, source_file_path,
  target_file_path, import_name, import_type, package_name,
  package_version
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: SetIndexingJobCompletedFromRunning :execrows
UPDATE indexing_jobs
SET
  status = 'completed',
  finished_at = NOW()
WHERE id = $1
  AND status = 'running';

-- name: SetIndexingJobFailedFromRunning :execrows
UPDATE indexing_jobs
SET
  status = 'failed',
  error_details = $2,
  finished_at = NOW()
WHERE id = $1
  AND status = 'running';

-- name: TryAdvisoryLockForProject :one
-- Uses two int4 keys derived from the UUID to avoid hashtext collisions.
SELECT pg_try_advisory_lock($1::int, $2::int)::boolean AS acquired;

-- name: ReleaseAdvisoryLockForProject :one
SELECT pg_advisory_unlock($1::int, $2::int)::boolean AS released;

-- name: FailStaleJob :execrows
UPDATE indexing_jobs
SET
  status = 'failed',
  error_details = @error_details,
  finished_at = NOW()
WHERE id = @id
  AND status = 'running';

-- name: DeleteOrphanedBuildingSnapshot :exec
DELETE FROM index_snapshots
WHERE id = $1
  AND status = 'building'
  AND is_active = FALSE;

-- name: ListStaleRunningJobs :many
SELECT id, project_id, worker_id, started_at, job_type, index_snapshot_id
FROM indexing_jobs
WHERE status = 'running'
  AND started_at < NOW() - @stale_threshold::interval;

-- name: ListOrphanedBuildingSnapshots :many
SELECT s.id, s.project_id
FROM index_snapshots s
LEFT JOIN indexing_jobs j
  ON j.index_snapshot_id = s.id
  AND j.status IN ('queued', 'running')
WHERE s.status = 'building'
  AND s.is_active = FALSE
  AND j.id IS NULL;
