# 09 — Job Creation, Dedup & Queue

## Status
Done

## Goal
Implemented durable index job creation in PostgreSQL with provider config pinning at creation time, deduplication against running/queued jobs, paginated job listing, and Redis/asynq queue integration. Enqueue failures are handled by marking the job as failed rather than leaving it stuck in a permanent `queued` state.

## Depends On
- Ticket 05 (Project CRUD)
- Ticket 08 (Provider Settings -- embedding and LLM resolution)

## Scope

### Job Creation Flow

`POST /v1/projects/{projectID}/index` (admin+ auth) performs the following:

1. Parse optional request body for `job_type` (`"full"` or `"incremental"`, default: `"full"`)
2. Validate the project has an active SSH key assignment (409 if not)
3. Resolve the effective embedding config via `embedding.Service.GetResolvedConfig` (409 if none)
4. Resolve the effective LLM config via `llm.Service.GetResolvedConfig` (NULL if none -- LLM is optional)
5. Check for an existing active job with `FindActiveIndexingJobForProjectAndType` (dedup)
6. If found: return the existing job with 202 Accepted (no re-enqueue)
7. If not found: create a new `indexing_jobs` row with `status = 'queued'`, pinning the resolved `embedding_provider_config_id` and `llm_provider_config_id`
8. Enqueue to Redis/asynq via `Publisher.EnqueueIndexJob`
9. If enqueue fails: mark job as `failed` with structured error in `error_details` and return 500
10. Return the created job with 202 Accepted

### Provider Config Pinning

The resolved embedding and LLM provider config IDs are stored on the job row at creation time. The worker loads these at execution time rather than re-resolving, ensuring the job uses the configuration that was active when it was requested. `ON DELETE SET NULL` on the FK references ensures job rows survive if a config is later deleted.

### Deduplication

Phase 1 dedupe key: `(project_id, job_type)`. "Active" means `status IN ('queued', 'running')`. At most one active full job and one active incremental job per project. Completed or failed jobs do not block new job creation.

### Queue Integration (`internal/queue/`)

The `Publisher` wraps an `asynq.Client` connected via `REDIS_URL`. `EnqueueIndexJob` builds a task payload conforming to `contracts/queue/workflow-task.v1.schema.json`:

```json
{
  "job_id": "uuid",
  "workflow": "full-index",
  "enqueued_at": "2026-03-09T10:00:00Z",
  "project_id": "uuid",
  "requested_by": "user:uuid"
}
```

Job type mapping: `"full"` -> `"full-index"`, `"incremental"` -> `"incremental-index"`. The workflow name is used as the asynq task type. No config blobs, secrets, or branch overrides are included in the queue message.

Per-workflow timeout and retry configuration is set via `PublisherConfig` so asynq enforces them. The publisher is nil-safe: calling methods on a nil publisher is a no-op, supporting graceful degradation when Redis is unavailable at startup.

### Job Listing

`GET /v1/projects/{projectID}/jobs` (member+ auth) returns paginated job history with `limit` (default 20, max 100) and `offset` query parameters. Jobs are ordered by `created_at DESC`.

### Error Handling

| Condition | Status | Error |
|-----------|--------|-------|
| Invalid `job_type` | 400 | Validation error |
| No active SSH key | 409 | `"project has no active SSH key assignment"` |
| No embedding config | 409 | `"project has no embedding provider configured"` |
| Enqueue failure | 500 | Job marked failed with `"stage": "enqueue"` in `error_details` |

## Key Files

| File | Purpose |
|------|---------|
| `backend-api/internal/queue/publisher.go` | asynq publisher with `EnqueueIndexJob`, `MapJobTypeToWorkflow`, `PublisherConfig` |
| `backend-api/internal/handler/project.go` | `HandleIndex` (job creation + enqueue), `HandleListJobs` |
| `datastore/postgres/queries/indexing.sql` | `CreateIndexingJob`, `FindActiveIndexingJobForProjectAndType`, `ListProjectJobs` |
| `datastore/postgres/migrations/000001_init.up.sql` | `indexing_jobs` table with `embedding_provider_config_id` and `llm_provider_config_id` columns |

## Acceptance Criteria
- [x] `POST /v1/projects/{id}/index` creates a durable `indexing_jobs` row in PostgreSQL
- [x] Resolved `embedding_provider_config_id` pinned on the job at creation time
- [x] Resolved `llm_provider_config_id` pinned on the job (NULL if none configured)
- [x] Projects without a resolved embedding config rejected with 409
- [x] Projects without an active SSH key rejected with 409
- [x] Repeated equivalent requests return the existing active job (dedup)
- [x] Dedup checks only consider active jobs (queued, running)
- [x] Completed/failed jobs do not block new job creation
- [x] Different projects and different job types do not dedupe against each other
- [x] Newly created jobs enqueued to Redis/asynq
- [x] Dedup-hit responses do not re-enqueue
- [x] Queue message conforms to workflow-task.v1 contract schema
- [x] No secrets, config blobs, or branch overrides in the queue message
- [x] Enqueue failure marks job as `failed` with structured error details
- [x] Publisher initialized at app startup and shut down gracefully
- [x] `GET /v1/projects/{id}/jobs` returns paginated job history ordered by `created_at DESC`
- [x] Per-workflow timeout and retry config set on asynq tasks
