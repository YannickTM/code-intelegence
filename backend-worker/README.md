# Backend Worker (Go)

## Role

`backend-worker` is the asynchronous execution engine for indexing and cleanup jobs.
It consumes queue tasks, runs parse/embed/store pipeline stages, and publishes progress events.

## Responsibilities

- Dequeue jobs from Redis/asynq with fair per-project scheduling
- Resolve project SSH key assignment and perform git clone/pull
- Run the embedded tree-sitter parser engine for parse output
- Generate embeddings through external Ollama (`/api/embed`)
- Upsert vectors to Qdrant
- Persist snapshots/files/symbols/chunks/dependencies in PostgreSQL
- Publish `job:*` and `snapshot:activated` events via Redis pub/sub

## Execution Model

1. Receive task payload from Redis queue
2. Mark `indexing_jobs` row as running
3. Run full/incremental pipeline
4. Publish progress events
5. Mark job completed or failed

Retry and timeout baseline:

- Retries: 3 for indexing jobs, 1 for cleanup jobs
- Timeout: 30m full, 10m incremental
- Uniqueness window: 1h for duplicate-suppression per project and job type

## Pipeline Stages

The indexing pipeline transforms raw source code into indexed knowledge through eight stages:

1. **Workspace preparation** — Clone or fetch the repository using the project's assigned SSH key. Create a worktree at the target commit. Enumerate source files with language detection.
2. **File classification** — Split files into parser-supported (Tier 1/2 languages) and raw files (Tier 3/unsupported). Build parser inputs with language IDs.
3. **Parse** — Process parser-supported files in batches with the embedded parser engine. Extract symbols, imports, chunks, exports, references, JSX usages, network calls, file facts, and diagnostics. Build synthetic module-context results for raw files.
4. **Chunk** — Hybrid strategy produces `function`, `class`, `module_context`, `config`, and `test` chunks. Each chunk carries a content hash, semantic role, owner info, and estimated token count.
5. **Import resolution** — Resolve extensionless import targets to actual file paths using stem-based lookup across the project file set. Classify imports as internal, external, or stdlib.
6. **Embed** — Batch-embed all chunk content using the project's resolved embedding provider (default: Ollama). Respects `EMBED_BATCH_SIZE`.
7. **Store** — Within a database transaction: persist snapshot, files, symbols, chunks, dependencies, exports, references, JSX usages, and network calls to PostgreSQL. Upsert vectors to Qdrant. Activate new snapshot atomically (deactivate old, activate new).
8. **Events** — Publish terminal job events via Redis pub/sub. Mark job completed or failed.

### Chunking Strategy

Three-tier language system governs chunking depth:

- **Tier 1** (full extraction — JS, TS, TSX, JSX): individual function/class/module-context chunks with export/reference/JSX tracking
- **Tier 2** (partial — Go, Python, Rust, Java, etc.): similar chunks but without export/reference tracking
- **Tier 3** (structural — YAML, JSON, Markdown, etc.): single module-context chunk per file

Config files are detected by patterns and chunked as a single CONFIG chunk. Test files are split by test block patterns (`describe`/`it`/`test`) or as a single TEST chunk. Per-file vector count: ~50–200 for Tier 1 files, 1 for Tier 3 files.

### Incremental Indexing

Incremental jobs avoid re-processing unchanged files:

1. Resolve active snapshot for project+branch; verify embedding version compatibility
2. Compute `git diff --name-status` between snapshot base commit and HEAD
3. **Changed files**: re-parse, re-chunk, re-embed, write new artifacts
4. **Unchanged files**: copy-forward all artifacts (files, symbols, chunks, dependencies, exports, references, JSX usages, network calls) from old snapshot to new with remapped IDs
5. **Vector copy-forward**: batch-read old vectors from Qdrant, re-upsert with new chunk IDs and updated snapshot metadata
6. Activate new snapshot atomically

Fallback to full re-index when: no active snapshot exists, embedding version mismatch, or diff computation fails. Commit history indexing runs as a non-fatal background step during both full and incremental indexing.

## Runtime Dependencies

- PostgreSQL
- Qdrant
- Redis
- Embedding provider (default: Ollama, configurable per project)

## Environment

```env
POSTGRES_DSN=postgres://app:app@postgres:5432/codeintel?sslmode=disable
QDRANT_URL=http://qdrant:6333
REDIS_URL=redis://redis:6379/0
PARSER_POOL_SIZE=4
PARSER_TIMEOUT_PER_FILE=30s
PARSER_MAX_FILE_SIZE=10485760
OLLAMA_URL=http://host.docker.internal:11434
OLLAMA_MODEL=jina/jina-embeddings-v2-base-en
REPO_CACHE_DIR=/var/lib/myjungle/repos
SSH_KEY_ENCRYPTION_SECRET=replace-this-secret
```

## Docker Build Contract

This service builds a single worker binary from `./cmd/worker` and runs it as a non-root container.
See `backend-worker/Dockerfile`.

## Worker Registry

Each running worker publishes a TTL-bound status entry in Redis for observability.

- Key pattern: `worker:status:{worker_id}`
- Heartbeat interval: 10 seconds
- Key TTL: 30 seconds (auto-expires if worker stops publishing)
- Schema: `contracts/redis/worker-status.v1.schema.json`

Status lifecycle:

1. `starting` — on boot, before queue consumer is ready
2. `idle` — ready and waiting for tasks
3. `busy` — executing a job (includes `current_job_id` and `current_project_id`)
4. `draining` — graceful shutdown in progress

Worker ID is resolved from the `WORKER_ID` environment variable, falling back to `os.Hostname()`.

## Local Development

### Start the stack

```bash
# 1. Start infrastructure services (postgres, redis, qdrant)
docker compose up -d postgres redis qdrant

# 2. Build and start the app profile (backend-api + backend-worker)
docker compose --profile app up --build
```

### Enqueue a test job

Use `backend-api` to create a project and trigger an index job. The worker picks it up automatically from the Redis queue.

### Observe worker status

```bash
# Check worker heartbeat in Redis
docker compose exec redis redis-cli GET "worker:status:myjungle-backend-worker"

# Follow worker logs
docker compose logs -f backend-worker
```

### Verify runtime tools

```bash
# Confirm git and SSH are available inside the container
docker compose exec backend-worker git --version
docker compose exec backend-worker ssh -V
```

## Notes

- Worker does not expose public HTTP endpoints in phase 1.
- Operational status is surfaced through PostgreSQL job state, Redis pub/sub events, and the worker registry.
- Stopping the worker leaves queued jobs waiting in Redis for restart.
- Workers are designed to be identical and replaceable for later horizontal scaling.
