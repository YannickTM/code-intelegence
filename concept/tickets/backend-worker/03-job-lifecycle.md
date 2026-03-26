# 03 — Job Repository & Execution Context

## Status
Done

## Goal
Teach the worker to load the full execution context it needs from PostgreSQL and to own durable job/snapshot state transitions. After this ticket the worker can answer: what project, which repo, which SSH key, which embedding config, and which job row to update.

## Depends On
01-foundation, 02-queue-dispatch

## Scope

### Execution Context Model (`internal/execution/`)

The execution context struct holds all data a workflow handler needs:

- Project ID, repo URL, default branch
- SSH key metadata + decrypted private key bytes
- Embedding provider config (provider, endpoint URL, model, dimensions)
- Job ID, job type

All fields are derived from the job row and its foreign keys. The queue message is intentionally minimal (`job_id`, `workflow`, `enqueued_at`); everything else is loaded from PostgreSQL at runtime.

### Database Reads

The worker uses existing `sqlc` queries where possible and adds new ones:

**New queries added:**
- `GetIndexingJob` -- load job by id
- `GetProjectSSHKeyForExecution` -- project-scoped SSH lookup joining `projects`, `project_ssh_key_assignments`, and `ssh_keys` tables to return the encrypted private key, repo URL, and default branch

**Existing queries reused:**
- `GetProject`, `GetEmbeddingProviderConfigByID`, `CreateIndexSnapshot`, `SetIndexingJobRunning`, `SetIndexingJobProgress`, `SetIndexingJobCompleted`, `SetIndexingJobFailed`, `CreateEmbeddingVersion`

### SSH Key Decryption (`internal/sshkey/`)

Worker-local decryption helper compatible with `backend-api` encryption:
- AES-256-GCM
- Key derived via SHA-256 from `SSH_KEY_ENCRYPTION_SECRET`
- Ciphertext format: `nonce || sealed(ciphertext + tag)`

This is a worker-local copy, not an import from `backend-api/internal/sshkey/`.

### Job Repository (`internal/repository/`)

Wraps sqlc queries in worker-friendly methods:

- `LoadExecutionContext(ctx, jobID)` -- loads job, project, SSH key, embedding config
- `ClaimJob(ctx, jobID)` -- atomic `queued` -> `running` transition
- `CreateSnapshot(ctx, params)` -- wraps `CreateIndexSnapshot`
- `SetJobRunning(ctx, jobID)` / `SetJobProgress(...)` / `SetJobCompleted(...)` / `SetJobFailed(...)`
- `TryProjectLock(ctx, projectID)` -- PostgreSQL advisory lock

State machine enforcement: `ClaimIndexingJobRunning` uses `WHERE status = 'queued'` so only one worker can claim. Terminal transitions (`SetIndexingJobCompletedFromRunning`, `SetIndexingJobFailedFromRunning`) use `WHERE status = 'running'`.

### Failure Behavior

The worker fails early before touching Git, Qdrant, or the parser if:
- Job does not exist or is not in `queued` status
- Project does not exist
- Project has no active SSH key assignment
- SSH key is inactive or undecryptable
- Embedding config referenced by the job does not exist

These surface as structured job failures with categorized error details.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/execution/` | Execution context types and loader |
| `internal/repository/` | Job/snapshot repository wrapper |
| `internal/sshkey/` | AES-256-GCM SSH key decryption |
| `datastore/postgres/queries/indexing.sql` | `GetIndexingJob`, `ClaimIndexingJobRunning`, guarded terminal updates |
| `datastore/postgres/queries/projects.sql` | `GetProjectSSHKeyForExecution` |

## Acceptance Criteria
- [x] Worker loads a job row from PostgreSQL by `job_id`
- [x] Worker loads project metadata (repo URL, default branch) from the job's `project_id`
- [x] Worker loads the active SSH key assignment and encrypted private key for a project
- [x] Worker decrypts the stored SSH private key with `SSH_KEY_ENCRYPTION_SECRET`
- [x] Worker loads the embedding config pinned on the job row via `embedding_provider_config_id`
- [x] Repository wrapper exposes worker-friendly methods instead of raw `sqlc` calls
- [x] Atomic claim: `ClaimIndexingJobRunning` uses `WHERE status = 'queued'`
- [x] Terminal transitions guarded: completion/failure only from `running` status
- [x] Missing job, project, SSH, or embedding state surfaces as structured execution errors
- [x] `sqlc generate` runs cleanly after adding new queries
