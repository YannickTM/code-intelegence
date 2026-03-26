# Redis

## Responsibility

Redis provides three runtime capabilities:

- Background job queueing via `asynq`
- Worker heartbeat and runtime registry
- Real-time event fanout via Redis pub/sub

It answers: "What should run next, which workers appear alive right now, and what changed right now?"

## Why Redis in This Architecture

- Low-latency queue operations
- Cheap ephemeral worker liveness registry
- Mature retry/timeout semantics through `asynq`
- Lightweight pub/sub for live backoffice updates
- Operationally simple for phase 1 scale

## Queue Model (asynq)

### Producer

`backend-api` enqueues tasks when a workflow is requested and records job rows in PostgreSQL.

### Consumer

`backend-worker` consumes tasks and executes the matching workflow handler.

### Queue message contract

Queue payloads should validate against:

- `contracts/queue/workflow-task.v1.schema.json`

Core envelope fields:

- `job_id`
- `workflow`
- `enqueued_at`

Optional hints:

- `project_id`
- `trace_id`
- `requested_by`

Supported workflow names in `v1`:

- `full-index`
- `incremental-index`
- `code-analysis`
- `rag-file`
- `rag-repo`
- `agent-run`

Contract rules:

- keep queue payloads small
- treat the queue message as a durable job reference, not as copied workflow input
- do not copy secrets, SSH material, or provider credentials into Redis
- load project execution context and workflow-specific request details from PostgreSQL by `job_id`
- if a future workflow needs extra input, persist it durably before enqueue and reference it through the job row, not through Redis payload expansion

Example payload shapes:

```json
{
  "job_id": "uuid",
  "workflow": "full-index",
  "enqueued_at": "2026-03-08T11:00:00Z",
  "project_id": "uuid"
}
```

```json
{
  "job_id": "uuid",
  "workflow": "agent-run",
  "enqueued_at": "2026-03-08T11:10:00Z",
  "trace_id": "req_opaque_id"
}
```

### Fair Scheduling

Use per-project queue names and weighted round-robin to avoid one project starving others.

### Baseline Policies

Implemented indexing baseline:

- `full-index`: 3 retries, 30m timeout
- `incremental-index`: 3 retries, 10m timeout

Future workflows should set handler-specific retry and timeout budgets based on their cost profile.

Uniqueness baseline:

- 1h per project + workflow identity

## Worker Registry Model

Workers should register their current runtime status in Redis while they are alive.

This registry is for:

- operational visibility
- lightweight routing or capacity hints
- graceful drain signaling

This registry is not for:

- durable job ownership
- authoritative job state
- deciding whether a job actually succeeded or failed

PostgreSQL remains the source of truth for job execution state.

### Worker status contract

Worker status payloads should validate against:

- `contracts/redis/worker-status.v1.schema.json`

Recommended fields:

- `worker_id`
- `status`
- `started_at`
- `last_heartbeat_at`
- `supported_workflows`
- optional `current_job_id`
- optional `current_project_id`
- optional `hostname`
- optional `version`

Recommended status values:

- `starting`
- `idle`
- `busy`
- `draining`
- `stopped`

### Key model

Recommended Redis key convention:

- `workers:{worker_id}`

Store a JSON value matching the worker-status contract and refresh TTL on each heartbeat.

Optional later helpers:

- `workers:active`
- workflow or queue-specific secondary indexes if listing needs become hot

### Heartbeat baseline

Recommended baseline:

- heartbeat interval: 10 seconds
- TTL expiry: 30 to 45 seconds

Expected behavior:

- worker writes `starting` on boot
- worker transitions to `idle` when ready for work
- worker writes `busy` while executing a job
- worker writes `draining` before shutdown or when stopping new task intake
- key expires automatically if the worker crashes and stops heartbeating

### Advisory-only rule

If worker registry status and PostgreSQL job state disagree:

- trust PostgreSQL for job truth
- treat Redis worker status as stale or advisory telemetry

## Event Model (pub/sub)

Redis pub/sub carries transient runtime events.

Typical events:

- `job:queued`
- `job:started`
- `job:progress`
- `job:completed`
- `job:failed`
- `snapshot:activated`

`backend-api` bridges these events to SSE clients (`/v1/events/stream`) for backoffice live updates.

## Suggested Key and Channel Conventions

### asynq queues

- `queue:workflow:project:{project_id}`
- optional later refinement: `queue:workflow:{workflow}:project:{project_id}`

### pub/sub channels

- `events:jobs`
- `events:snapshots`
- optional scoped channel: `events:project:{project_id}`

### payload shape (event)

```json
{
  "event": "job:progress",
  "project_id": "uuid",
  "job_id": "uuid",
  "snapshot_id": "uuid",
  "timestamp": "2026-03-02T00:00:00Z",
  "data": {
    "files_processed": 120,
    "chunks_upserted": 480
  }
}
```

## Persistence and Durability

- Queue durability is managed by Redis persistence plus asynq task storage
- Worker registry entries are ephemeral and should expire via TTL
- Pub/sub events are ephemeral by design (not a durable log)
- Historical/audit state remains in PostgreSQL (`indexing_jobs`, `index_snapshots`)

## Docker Baseline

- Image: `redis:7-alpine`
- Port: `6379`
- Persistence: AOF enabled (`--appendonly yes`)
- Health check: `redis-cli ping`

Compose defaults:

```text
command: redis-server --save 60 1 --appendonly yes
volume: redis_data:/data
```

## Operational Notes

- Monitor memory usage and eviction policy
- Keep pub/sub payloads compact
- Use structured JSON events so SSE consumers can apply deterministic updates
- Avoid storing long-lived business data in Redis; PostgreSQL is source of truth

## Phase 1 Non-Goals

- Multi-region Redis replication
- Kafka-like durable event replay
- Advanced stream processing with Redis Streams
