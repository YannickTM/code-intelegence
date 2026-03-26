# 13 — Event Publishing & Multi-Worker Safety

## Status
Done

## Goal
Implement Redis pub/sub event publishing for real-time job lifecycle notifications and the multi-worker safety mechanisms that allow horizontal scaling without data corruption: advisory locks, idempotent operations, and the worker heartbeat registry.

## Depends On
01-foundation, 02-queue-dispatch

## Scope

### Event Publisher (`internal/notify/publisher.go`)

The `EventPublisher` publishes structured lifecycle events to the `myjungle:events` Redis pub/sub channel. The API's SSE subscriber consumes these events and broadcasts them to connected browser clients.

**Event types:**

| Event | Published when | Data fields |
|---|---|---|
| `job:started` | Worker claims a job and sets status to `running` | `status`, `job_type` |
| `job:progress` | Worker completes a file batch (throttled by caller) | `status`, `job_type`, `files_processed`, `files_total`, `chunks_upserted` |
| `job:completed` | Job finishes successfully | `status`, `job_type`, `files_processed`, `chunks_upserted`, `vectors_deleted` |
| `job:failed` | Job fails terminally | `status`, `job_type`, `error_message` |
| `snapshot:activated` | Index snapshot becomes active | `active_commit` |

**Message contract** (matches `contracts/events/sse-event.v1.schema.json`):

```go
type SSEEvent struct {
    Event      string         `json:"event"`
    ProjectID  string         `json:"project_id"`
    JobID      string         `json:"job_id,omitempty"`
    SnapshotID string         `json:"snapshot_id,omitempty"`
    Timestamp  string         `json:"timestamp"`
    Data       map[string]any `json:"data,omitempty"`
}
```

**Fire-and-forget semantics:** Event publishing is advisory and must never block or fail the job workflow. Each convenience method (`PublishJobStarted`, `PublishJobProgress`, `PublishJobCompleted`, `PublishJobFailed`, `PublishSnapshotActivated`) logs a warning on publish failure but does not return the error.

**Nil-safe publisher:** All methods check `p == nil || p.rdb == nil` and return nil, so callers do not need nil guards. This follows the same pattern as other nil-safe clients in the codebase.

**Progress throttling:** The publisher itself does not enforce throttling. Callers (workflow handlers) are responsible for limiting `job:progress` events to at most once per 2 seconds.

### Advisory Locks (`internal/repository/`)

PostgreSQL session-level advisory locks prevent concurrent indexing of the same project:

- Uses `pg_try_advisory_lock(key1, key2)` with two int32 keys derived from the project UUID to avoid hash collisions
- Non-blocking: returns `ErrProjectLocked` immediately if the lock is already held by another session
- Session-scoped: lock lives on a dedicated pinned database connection acquired from the pool
- The unlock function releases the lock via `pg_advisory_unlock(key1, key2)` and returns the connection to the pool
- If unlock fails, the connection is destroyed (not returned) to prevent lock leaks

Both the full-index and incremental-index handlers acquire the lock before claiming the job. The lock is released via `defer projectUnlock(ctx)`.

### Idempotent Operations

Multiple safety mechanisms ensure correctness under concurrent execution:

- **Job claim:** `ClaimJob` uses `WHERE status = 'queued'` so only one worker can transition a job to `running`; subsequent attempts get `ErrJobNotQueued` and silently skip
- **Terminal transitions:** `SetJobCompleted` and `SetJobFailed` use `WHERE status = 'running'` guards so duplicate completions are no-ops
- **Artifact upserts:** `UpsertFileContent` uses `ON CONFLICT (project_id, content_hash) DO UPDATE` for idempotent content deduplication; `InsertCommit` uses `ON CONFLICT (project_id, commit_hash) DO UPDATE`
- **Qdrant collection creation:** 409 Conflict is handled gracefully when multiple workers create the same collection concurrently
- **Embedding version:** `GetEmbeddingVersionByLabel` lookup before insert ensures no duplicate rows

### Worker Registry (`internal/registry/`)

Each live worker publishes an ephemeral runtime status entry to Redis using `go-redis/v9`:

- Key pattern: `worker:status:{worker_id}` (SET with TTL)
- Key TTL: 30 seconds, refreshed by heartbeat goroutine every 10 seconds
- Worker ID: `WORKER_ID` environment variable with fallback to `os.Hostname()`
- Status lifecycle: `starting` -> `idle` -> `busy` -> `draining`
- Payload matches `contracts/redis/worker-status.v1.schema.json`: includes `worker_id`, `status`, `started_at`, `last_heartbeat_at`, `hostname`, `supported_workflows`, `current_job_id`, `current_project_id`

The registry is advisory only. PostgreSQL remains authoritative for job ownership and completion. If Redis is unavailable, the heartbeat logs a warning and continues -- job execution is unaffected.

### Multi-Worker Queue Behavior

- Workers compete for jobs via asynq's Redis-backed queue (atomic dequeue)
- Concurrency per worker is configurable via `WORKER_CONCURRENCY` (default: 4)
- Queue name configurable via `QUEUE_NAME` (default: `default`)
- Per-project fairness is enforced by advisory locks: only one worker can index a given project at a time
- No shared mutable state between workers beyond Redis (queue, events, registry) and PostgreSQL (jobs, artifacts)

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/notify/publisher.go` | `EventPublisher`, `SSEEvent`, convenience methods, nil-safe design |
| `internal/notify/publisher_test.go` | Unit tests with miniredis |
| `internal/repository/job.go` | `TryProjectLock`, `ClaimJob`, guarded terminal transitions |
| `internal/registry/registry.go` | Worker heartbeat, status transitions, Redis SET/EXPIRE |
| `contracts/events/sse-event.v1.schema.json` | Event message contract |
| `contracts/redis/worker-status.v1.schema.json` | Worker status payload contract |

## Acceptance Criteria
- [x] `EventPublisher` publishes events to `myjungle:events` Redis pub/sub channel
- [x] Published messages conform to `sse-event.v1.schema.json` (event, project_id, timestamp required)
- [x] Five event types implemented: `job:started`, `job:progress`, `job:completed`, `job:failed`, `snapshot:activated`
- [x] Convenience helpers construct correct payloads and swallow publish errors (fire-and-forget)
- [x] Publisher is nil-safe (all methods no-op on nil receiver)
- [x] Channel name matches API subscriber constant (`myjungle:events`)
- [x] Advisory lock per project_id prevents concurrent indexing of same project
- [x] Advisory lock is session-scoped via pinned database connection
- [x] Lock not acquired returns `ErrProjectLocked` immediately (non-blocking)
- [x] Job claim uses `WHERE status = 'queued'` for atomic single-winner transition
- [x] Terminal job transitions guarded by `WHERE status = 'running'`
- [x] Qdrant collection creation handles 409 Conflict for concurrent creates
- [x] Worker registry publishes ephemeral status with 30s TTL, 10s heartbeat
- [x] Worker status lifecycle covers `starting`, `idle`, `busy`, `draining`
- [x] Redis unavailability does not block or fail job execution
