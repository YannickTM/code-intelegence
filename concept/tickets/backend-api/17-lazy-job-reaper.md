# 17 — Lazy Job Reaper (Read-Path Crash Detection)

## Status
Done

## Goal
When the backoffice (or any API client) queries job status and encounters a `running` job, check whether the worker is still alive. If the worker's heartbeat has expired, transition the job to `failed` on the spot — before returning the response. The UI never shows a stale "running" state for a dead worker.

This is a lazy/on-demand approach: no background goroutine, no periodic ticker. The reaper only runs when someone looks.

## Depends On
- `backend-worker/14-job-reaper` (schema: `worker_id` column, shared reaper core: `IsWorkerAlive()`, `FailStaleJob` query, orphaned snapshot cleanup)

## Problem

After a worker crash, the `indexing_jobs` row stays in `running` status. Every API endpoint that returns job status will report the job as active. The backoffice shows "indexing in progress" forever — until either:
- The worker restarts (startup sweep from ticket 14), or
- Someone manually fixes the database

If the worker never restarts (scaled to zero, infra outage, deleted container), nothing ever corrects the state. The lazy reaper fills this gap: **the act of checking the status is what triggers the correction**.

## Scope

### 1. Identify Read Paths That Surface Job Status

Three API response patterns include `active_job_status`:

| Endpoint | Query | Where active job appears |
|---|---|---|
| `GET /v1/projects/{id}` | `GetProjectWithIndexStatus` | `active_job_id`, `active_job_status` fields |
| `GET /v1/projects` | `ListProjectsForUser` | Same fields per project |
| `GET /v1/projects/{id}/jobs` | `ListProjectJobs` | Each job row has `status` |
| `GET /v1/dashboard` | `GetDashboardForUser` | `active_job_id`, `active_job_status` per project |

All of these surface a `running` status to the backoffice without verifying whether the worker is still alive.

### 2. Reaper Check Helper (`internal/reaper/` or `internal/jobhealth/`)

A lightweight helper that the API handlers call when they encounter a `running` job. This wraps the shared `reaper.IsWorkerAlive()` from `backend-worker/14-job-reaper` and the `FailStaleJob` query.

```go
// CheckAndReapIfDead checks whether a running job's worker is still alive.
// If the worker is dead, it transitions the job to failed, cleans up the
// orphaned snapshot, publishes a job:failed SSE event, and returns the
// updated status. If the worker is alive (or the job is not running),
// it returns the original status unchanged.
//
// This is a best-effort, fire-and-forget operation: errors are logged
// but do not fail the parent request.
func (c *JobHealthChecker) CheckAndReapIfDead(ctx context.Context, jobID, workerID, currentStatus string) string
```

**Behavior:**
- If `currentStatus != "running"` → return as-is (fast path, no Redis call)
- If job has been running < stale threshold → return as-is (avoid hitting Redis for fresh jobs)
- `GET worker:status:{workerID}` from Redis
- If worker alive and working on this job → return `"running"`
- If worker dead or moved on:
  1. `FailStaleJob(jobID, error_details)` — transition to `failed`
  2. Clean up orphaned `building` snapshot if any
  3. Publish `job:failed` SSE event
  4. Return `"failed"` (so the current response already reflects the correction)
- On any error (Redis down, DB error) → log warning, return original status (graceful degradation)

### 3. Integration Into Handlers

**Option A: Inline in response serialization (minimal diff)**

In the handler functions that build the response map, after reading `active_job_status` from the DB row:

```go
// In projectWithIndexToMap() or equivalent:
if row.ActiveJobID.Valid && row.ActiveJobStatus == "running" {
    corrected := h.jobHealth.CheckAndReapIfDead(ctx,
        dbconv.PgUUIDToString(row.ActiveJobID),
        row.ActiveJobWorkerID,  // new column from ticket 14
        row.ActiveJobStatus,
    )
    m["active_job_status"] = corrected
    if corrected == "failed" {
        m["active_job_id"] = nil  // no longer active
    }
}
```

**Option B: Middleware/decorator on the DB query layer**

Wrap the queries that return `active_job_status` so the check is transparent to handlers. More complex, but handlers don't need to know about the reaper.

**Recommended: Option A** — explicit, easy to audit, minimal blast radius.

### 4. `ListProjectJobs` Enhancement

For the job list endpoint, each returned job has its own status. The check runs for any job with `status = 'running'`:

```go
for i, j := range jobs {
    item := indexingJobToMap(j)
    if j.Status == "running" {
        item["status"] = h.jobHealth.CheckAndReapIfDead(ctx, ...)
    }
    items[i] = item
}
```

In practice there's at most one `running` job per project (enforced by dedup), so this adds at most one Redis GET per request.

### 5. SQL Query Update

The `GetProjectWithIndexStatus` and related queries need to also return `worker_id` from the active job subquery so the API can pass it to the health checker:

```sql
-- Add to the active_job lateral join:
active_job.worker_id AS active_job_worker_id,
```

### 6. Stale Threshold

The check uses the same `REAPER_STALE_THRESHOLD` (default: 5 minutes) as the worker-side reaper. Jobs running less than this are assumed healthy — no Redis lookup needed. This avoids unnecessary Redis traffic for jobs that just started.

### 7. Redis Dependency

The API already has a Redis connection for SSE pub/sub. The `JobHealthChecker` reuses this connection for the heartbeat key lookup. No new infrastructure.

## Key Files

| File/Package | Purpose |
|---|---|
| `backend-api/internal/jobhealth/checker.go` | `JobHealthChecker` struct, `CheckAndReapIfDead()` |
| `backend-api/internal/jobhealth/checker_test.go` | Unit tests with miniredis |
| `backend-api/internal/handler/project.go` | Integrate check in `projectWithIndexToMap`, `HandleListJobs` |
| `backend-api/internal/handler/user.go` | Integrate check in dashboard project maps |
| `datastore/postgres/queries/projects.sql` | Add `active_job_worker_id` to project detail queries |
| `datastore/postgres/queries/auth.sql` | Add `active_job_worker_id` to dashboard queries |

## Non-Goals
- No background goroutine or periodic ticker in the API
- No orphaned snapshot GC in the API (handled by worker startup sweep in ticket 14; the lazy reaper cleans snapshots for the specific job it reaps)
- No Qdrant vector cleanup (follow-up)

## Acceptance Criteria
- [x] `running` jobs are checked against the worker heartbeat on every read path that surfaces job status
- [x] Dead-worker jobs are transitioned to `failed` inline, before the API response is sent
- [x] The corrected status is returned in the response (client sees `failed`, not stale `running`)
- [x] `job:failed` SSE event published when a job is reaped
- [x] Orphaned `building` snapshot cleaned up for the reaped job
- [x] Jobs running less than `REAPER_STALE_THRESHOLD` are not checked (avoid unnecessary Redis calls)
- [x] Redis or DB errors during the check do not fail the parent API request (graceful degradation)
- [x] At most one Redis GET per request (only one active job per project)
- [x] `active_job_worker_id` added to project detail and dashboard SQL queries
- [x] Existing API tests pass; new unit tests cover: alive worker skipped, dead worker reaped, Redis-down fallback, non-running jobs fast-pathed
