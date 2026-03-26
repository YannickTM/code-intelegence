# 02 — Queue Consumer & Workflow Dispatcher

## Status
Done

## Goal
Add the Redis/asynq consumer and generic workflow dispatcher that decodes queued messages into typed Go structs and routes them to the correct workflow handler. Includes the worker registry for heartbeat-based status publishing.

## Depends On
01-foundation

## Scope

### Asynq Server Integration

The worker runs an `asynq.Server` with `Server.Start(mux)` (non-blocking) instead of `Server.Run(mux)` (blocking). The existing signal-aware `App.Run()` loop handles shutdown via `Server.Shutdown()` in `App.Close()`.

Server configuration:
- Queue name from `QUEUE_NAME` (default: `default`)
- Concurrency from `WORKER_CONCURRENCY` (default: 4)
- Graceful shutdown timeout from `QUEUE_SHUTDOWN_TIMEOUT` (default: 30s)

The API publisher uses the workflow name as the asynq task type (e.g., `asynq.NewTask("full-index", data)`). The worker's `asynq.ServeMux` registers handlers using the same workflow name strings as pattern keys.

### Workflow Message Contract

Matches `contracts/queue/workflow-task.v1.schema.json`:

```go
type WorkflowTask struct {
    JobID       string  // required
    Workflow    string  // required
    EnqueuedAt  string  // required
    ProjectID   string  // optional routing hint
    TraceID     string  // optional observability hint
    RequestedBy string  // optional observability hint
}
```

Contract rules:
- The queue message is a durable job reference, not copied workflow input
- No SSH secrets or embedding configs in Redis
- Worker loads all execution details from PostgreSQL by `job_id`

### Dispatcher (`internal/workflow/`)

The dispatcher defines a `Handler` interface and routes tasks by workflow name:

```go
type Handler interface {
    Handle(ctx context.Context, task WorkflowTask) error
}
```

A wrapper function on the `asynq.ServeMux` handles each incoming task by:
1. Decoding `asynq.Task.Payload()` into `WorkflowTask`
2. Validating required fields (`job_id`, `workflow`, `enqueued_at`)
3. Logging context (job_id, workflow, project_id, retry count)
4. Delegating to the registered `Handler`

Registered handlers: `full-index`, `incremental-index`.

Unknown workflow names are rejected by asynq ServeMux itself. The dispatcher stays transport-agnostic once the task is decoded.

### Worker Registry (`internal/registry/`)

Each live worker publishes an ephemeral runtime status entry to Redis using a direct `go-redis/v9` client (asynq does not expose its internal Redis connection).

- Key pattern: `worker:status:{worker_id}`
- TTL: 30s, refreshed by heartbeat goroutine every 10s
- Worker ID: `WORKER_ID` env var, fallback to `os.Hostname()`
- Status lifecycle: `starting` -> `idle` -> `busy` -> `draining`
- Payload matches `contracts/redis/worker-status.v1.schema.json`

The registry is advisory only. PostgreSQL remains authoritative for job ownership and completion.

### Per-Project Concurrency

Phase 1 uses the `default` queue with basic asynq concurrency. Per-project fairness improvements are deferred to ticket 13-events-safety.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/queue/consumer.go` | Payload decoding, ServeMux setup, handler wrapper |
| `internal/workflow/handler.go` | Handler interface definition |
| `internal/workflow/task.go` | WorkflowTask type |
| `internal/registry/registry.go` | Worker heartbeat, status transitions, Redis SET/EXPIRE |
| `internal/app/app.go` | Wiring: Server.Start + registry heartbeat |

## Acceptance Criteria
- [x] Worker connects to Redis/asynq and starts a consumer loop via `Server.Start(mux)`
- [x] Queued workflow messages decode into `WorkflowTask` matching all contract fields
- [x] Payload decoding validates required fields (`job_id`, `workflow`, `enqueued_at`)
- [x] Dispatcher routes `full-index` and `incremental-index` to registered handlers
- [x] Unknown workflow names fail deterministically (rejected by asynq ServeMux)
- [x] Queue messages do not include secrets or large config blobs
- [x] Each live worker publishes a TTL-bound status entry in Redis
- [x] Worker status transitions cover `starting`, `idle`, `busy`, and `draining`
- [x] Worker ID is deterministic per process (hostname-based with env var override)
- [x] Logs include task identity for debugging (job_id, workflow, project_id, retry count)
- [x] Shutdown stops the consumer cleanly via `Server.Shutdown()`
