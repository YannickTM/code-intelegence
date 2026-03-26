# 14 — Stuck-Job Reaper Core & Worker Startup Sweep

## Status
Done

## Goal
Provide the shared reaper logic that detects orphaned `running` jobs (whose worker has crashed) and transitions them to `failed`, cleans up partial snapshots, and publishes SSE events. Integrate into the worker as a one-shot startup sweep so a restarted worker instantly cleans up after its own crash.

The `backend-api` consumes the same reaper core via a separate ticket (`backend-api/17-lazy-job-reaper`).

## Depends On
02-queue-dispatch, 03-job-lifecycle, 13-events-safety

## Problem

When a worker container is killed (SIGKILL, OOM, `docker stop`, node eviction) during indexing:

1. **Job stuck forever** — status remains `running`, `finished_at` is NULL. The backoffice shows "indexing in progress" indefinitely.
2. **Orphaned snapshot** — `index_snapshots` row with `status = 'building'`, `is_active = FALSE`, never activated. Its associated files, symbols, chunks, and dependencies consume space but are unreachable.
3. **Orphaned vectors** — Qdrant points written during the partial run reference a non-active snapshot.
4. **No SSE event** — the backoffice never receives `job:failed`, so the UI never updates.

The graceful shutdown path (`SIGTERM` → `Close()` → `registry.SetDraining()`) handles clean shutdowns, but `SIGKILL` and container force-stops bypass it entirely.

### Existing Infrastructure

| Component | What it provides | Gap |
|---|---|---|
| Worker heartbeat (`registry.go`) | Redis key `worker:status:{id}` with 30s TTL, 10s refresh. Payload includes `current_job_id`. | **No consumer** checks for expired keys. |
| Job state machine (`indexing.sql`) | Guarded transitions: `WHERE status = 'running'`. | No timeout column. No scanner. |
| Event publisher (`notify/publisher.go`) | `PublishJobFailed()` sends SSE via Redis pub/sub. | Only called from workflow handlers. |
| Advisory locks | Session-scoped, auto-release on connection death. | Safe — not the problem. |
| Snapshot cascade | `DELETE FROM index_snapshots WHERE id = X` cascades to files, symbols, chunks, deps. | No process triggers the delete. |

## Scope

### 1. Schema Addition: `worker_id` on `indexing_jobs`

Add a `worker_id TEXT` column to the `indexing_jobs` table. Populated when the job is claimed, so the reaper can correlate a running job with a specific worker's heartbeat.

**Migration (`000002_add_worker_id.up.sql`):**
```sql
ALTER TABLE indexing_jobs ADD COLUMN worker_id TEXT;
```

**Updated claim query:**
```sql
-- name: ClaimQueuedIndexingJob :execrows
UPDATE indexing_jobs
SET status = 'running',
    started_at = NOW(),
    worker_id = @worker_id
WHERE id = @id
  AND status = 'queued';
```

### 2. New SQL Queries

```sql
-- name: ListStaleRunningJobs :many
SELECT id, project_id, worker_id, started_at
FROM indexing_jobs
WHERE status = 'running'
  AND started_at < NOW() - @stale_threshold::interval;

-- name: FailStaleJob :execrows
UPDATE indexing_jobs
SET status = 'failed',
    error_details = @error_details,
    finished_at = NOW()
WHERE id = @id
  AND status = 'running';

-- name: ListOrphanedBuildingSnapshots :many
SELECT s.id, s.project_id
FROM index_snapshots s
LEFT JOIN indexing_jobs j
  ON j.index_snapshot_id = s.id
  AND j.status = 'running'
WHERE s.status = 'building'
  AND s.is_active = FALSE
  AND j.id IS NULL;

-- name: DeleteOrphanedSnapshot :exec
DELETE FROM index_snapshots WHERE id = @id AND status = 'building' AND is_active = FALSE;
```

### 3. Reaper Core (`internal/reaper/`)

A `Reaper` struct with a `RunOnce(ctx) (int, error)` method that performs a single scan cycle. Returns the number of jobs reaped. This is the shared logic consumed by both the worker startup sweep and the API lazy reaper.

**Dependencies (injected):**
- `db.Querier` — for `ListStaleRunningJobs`, `FailStaleJob`, `ListOrphanedBuildingSnapshots`, `DeleteOrphanedSnapshot`
- `redis.Client` — for checking `worker:status:{id}` keys
- `notify.EventPublisher` (optional, nil-safe) — for `job:failed` SSE events

**`RunOnce` algorithm:**

```
1. Query ListStaleRunningJobs(stale_threshold)
2. For each stale job:
   a. If job.worker_id is set:
      - GET worker:status:{worker_id} from Redis
      - If key exists AND payload.current_job_id == job.id → worker alive, skip
      - If key exists AND payload.current_job_id != job.id → worker moved on, orphaned
      - If key is gone (TTL expired) → worker dead, orphaned
   b. If job.worker_id is NULL (legacy rows):
      - SCAN worker:status:* for any with current_job_id == job.id
      - If none found → orphaned
   c. For orphaned jobs:
      - FailStaleJob(job.id, error_details: worker_crash category)
      - Publish job:failed SSE event
      - Log at WARN level
3. Query ListOrphanedBuildingSnapshots
4. For each orphaned snapshot:
   - DeleteOrphanedSnapshot(snapshot.id) → CASCADE to artifacts
   - Log at INFO level
```

All operations are idempotent. `FailStaleJob` uses `WHERE status = 'running'` so concurrent callers are no-ops.

**`IsWorkerAlive(ctx, workerID, jobID) bool`** — a standalone helper method that checks whether a specific worker is still alive and working on a specific job. Exported so the API can call it for a single job without running the full scan (see `backend-api/17-lazy-job-reaper`).

**Error details format:**
```json
[{"category": "worker_crash", "message": "Worker process became unreachable during indexing. The job was automatically marked as failed.", "step": "reaper"}]
```

### 4. Qdrant Orphan Cleanup (follow-up)

When a building snapshot is deleted, its Qdrant vectors become orphaned. Deleting them by `index_snapshot_id` filter can be added as a follow-up to keep the initial scope focused.

### 5. Worker Startup Sweep (`app.go`)

On boot, before the queue consumer starts accepting jobs, run a single synchronous reap:

```go
// In App.Run(), before queue.Server.Start():
r := reaper.New(reaper.Config{
    StaleThreshold: cfg.Reaper.StaleThreshold,
}, db.Queries, redisClient, publisher)

if n, err := r.RunOnce(ctx); err != nil {
    slog.Warn("startup reap failed", "error", err)
} else if n > 0 {
    slog.Info("startup reap: cleaned up orphaned jobs", "count", n)
}
```

This provides instant recovery when a crashed worker restarts — the orphaned job and snapshot are cleaned up before accepting new work.

### 6. Pass `worker_id` During Claim

Update `repository.ClaimJob` to accept and pass the worker ID:

```go
func (r *JobRepository) ClaimJob(ctx context.Context, jobID pgtype.UUID, workerID string) error {
    rows, err := r.queries.ClaimQueuedIndexingJob(ctx, ClaimParams{
        ID:       jobID,
        WorkerID: workerID,
    })
    ...
}
```

The `consumer.go` `wrapHandler` already has access to the registry and calls `reg.SetBusy(task.JobID, ...)`. The worker ID is available via `reg.WorkerID()`.

## Key Files

| File/Package | Purpose |
|---|---|
| `backend-worker/internal/reaper/reaper.go` | `Reaper` struct, `RunOnce()`, `IsWorkerAlive()` |
| `backend-worker/internal/reaper/reaper_test.go` | Unit tests with mock DB + miniredis |
| `backend-worker/internal/app/app.go` | Wire startup sweep before queue start |
| `backend-worker/internal/repository/job.go` | Pass `worker_id` during `ClaimJob` |
| `backend-worker/internal/config/config.go` | `Reaper.StaleThreshold` config field |
| `datastore/postgres/queries/indexing.sql` | New queries + updated `ClaimQueuedIndexingJob` |
| `datastore/postgres/migrations/000002_add_worker_id.up.sql` | Add `worker_id` column |

## Acceptance Criteria
- [ ] `worker_id` column added to `indexing_jobs` and populated on claim
- [ ] `Reaper.RunOnce()` detects jobs stuck in `running` where the worker heartbeat has expired
- [ ] `Reaper.IsWorkerAlive()` exported for single-job checks by the API
- [ ] Orphaned jobs transitioned to `failed` with structured error details
- [ ] `job:failed` SSE event published so backoffice updates in real-time
- [ ] Orphaned `building` snapshots deleted (cascading to files/symbols/chunks/deps)
- [ ] All reaper operations are idempotent
- [ ] Worker runs a single `RunOnce` sweep on startup before accepting jobs
- [ ] Reaper logs at WARN (job failures) and INFO (snapshot cleanup)
- [ ] Existing tests pass; new unit tests cover: stale job detection, worker-alive skip, snapshot cleanup, idempotent re-runs
