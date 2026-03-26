<details>
<summary>Relevant source files</summary>

The following files were used as context for generating this wiki page:

- [backend-worker/cmd/worker/main.go](https://github.com/YannickTM/code-intelegence/blob/main/backend-worker/cmd/worker/main.go)
- [backend-worker/internal/workflow/dispatcher.go](https://github.com/YannickTM/code-intelegence/blob/main/backend-worker/internal/workflow/dispatcher.go)
- [backend-worker/internal/execution/context.go](https://github.com/YannickTM/code-intelegence/blob/main/backend-worker/internal/execution/context.go)
- [concept/worker/00-Introduction-overview.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/worker/00-Introduction-overview.md)
- [concept/worker/01-Components.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/worker/01-Components.md)
- [concept/worker/03-Communication-between-components-container.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/worker/03-Communication-between-components-container.md)
- [concept/tickets/backend-worker/00-overview.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/tickets/backend-worker/00-overview.md)
- [concept/tickets/backend-worker/10-full-index.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/tickets/backend-worker/10-full-index.md)
- [concept/tickets/backend-worker/01-foundation.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/tickets/backend-worker/01-foundation.md)
</details>

# Backend Worker Processes

## Introduction

The **Backend Worker** is the asynchronous execution engine of the MYJUNGLE platform, responsible for processing heavy-duty workflows such as repository indexing, code analysis, and artifact generation. It operates as a horizontally scalable, stateless runtime (excluding local workspaces) that consumes tasks from a Redis/asynq queue and persists results into PostgreSQL and Qdrant. Sources: [backend-worker/README.md](), [concept/worker/00-Introduction-overview.md]()

The system follows a "v7" architecture which prioritizes a minimal, durable execution model. In this model, the `backend-api` creates job records in PostgreSQL and enqueues small task messages in Redis. The worker then dequeues these tasks, resolves the full project-scoped execution context from the database, and executes the designated workflow handler. Sources: [concept/worker/00-Introduction-overview.md](), [concept/tickets/backend-worker/00-overview.md]()

## Architecture and Component Roles

The worker ecosystem is composed of several logical components that manage the transition from a queued task to a completed code intelligence product.

### Core Worker Components

| Component | Responsibility |
|---|---|
| **Queue Consumer** | Pulls tasks from Redis/asynq; ensures idempotent and retry-safe execution. |
| **Workflow Dispatcher** | Routes incoming job IDs to specific handlers (e.g., `full-index`). |
| **Execution Context Loader** | Fetches project metadata, SSH keys, and embedding configurations from PostgreSQL. |
| **Workflow Handlers** | Specific logic for operations like `full-index` or `incremental-index`. |
| **Worker Registry** | Publishes ephemeral heartbeats and runtime status (idle, busy, etc.) to Redis. |

Sources: [concept/worker/01-Components.md](), [concept/tickets/backend-worker/00-overview.md]()

### High-Level Data Flow

The following diagram illustrates the lifecycle of a worker process from task dequeue to final persistence.

```mermaid
flowchart TD
    subgraph "Queue Layer"
        Redis[(Redis/asynq)]
    end

    subgraph "Worker Runtime"
        Consumer[Queue Consumer]
        Loader[Context Loader]
        Dispatcher{Workflow Dispatcher}
        Handler[Workflow Handler]
        Parser[Embedded Parser Engine]
    end

    subgraph "Durable Storage"
        PG[(PostgreSQL)]
        QD[(Qdrant)]
    end

    subgraph "Dependencies"
        Git[Git Remote/SSH]
        Ollama[Ollama Embeddings]
    end

    Redis -- Dequeue Task --> Consumer
    Consumer -- job_id --> Loader
    Loader -- Load Context --> PG
    Loader -- Dispatch --> Dispatcher
    Dispatcher -- Execute --> Handler
    
    Handler -- Clone/Fetch --> Git
    Handler -- Parse Files --> Parser
    Handler -- Generate Vectors --> Ollama
    Handler -- Save Artifacts --> PG
    Handler -- Upsert Vectors --> QD
```
Sources: [concept/01-system-overview.md](), [concept/worker/03-Communication-between-components-container.md]()

## Workflow Dispatching and Context Loading

A critical design principle of the worker is that the queue message remains minimal. It typically contains only a `job_id` and the `workflow` type. This prevents stale secrets or large configuration blobs from living in the queue. Sources: [concept/worker/03-Communication-between-components-container.md](), [concept/worker/00-Introduction-overview.md]()

### Execution Context
Upon receiving a `job_id`, the worker resolves an `execution.Context` which includes:
- **Project Metadata**: Repository URL and default branch.
- **SSH Credentials**: Decrypted private keys for Git access.
- **Embedding Settings**: Provider URL, model name, and dimensions.
- **LLM Configuration**: (Future) Settings for analysis or description workflows.

Sources: [backend-worker/internal/execution/context.go](), [concept/tickets/backend-worker/10-full-index.md]()

### Dispatcher Logic
The `Workflow Dispatcher` acts as a router. It supports multiple workflow types, mapping them to specific Go handlers.

```go
// Example of supported workflow types
const (
    WorkflowFullIndex        = "full-index"
    WorkflowIncrementalIndex = "incremental-index"
    WorkflowDescribeFiles    = "describe-files"
)
```
Sources: [backend-worker/internal/workflow/dispatcher.go](), [concept/tickets/backend-worker/09-pipeline.md]()

## Primary Workflow: Full Indexing

The `full-index` workflow is the primary pipeline in Phase 1. It transforms a source repository into a queryable knowledge base.

### Step-by-Step Execution
1. **Job Claim**: The worker marks the job as `running` in PostgreSQL and records the `started_at` timestamp.
2. **Workspace Preparation**: Clones or fetches the repository into a project-scoped cache and creates an isolated Git worktree for the specific job.
3. **Parsing**: Parses supported files directly inside `backend-worker` using the embedded parser engine to extract symbols, imports, and code chunks.
4. **Embedding**: Batches extracted chunks and sends them to the embedding provider (default: Ollama).
5. **Persistence**: Writes files, symbols, chunks, and dependencies to PostgreSQL; upserts vectors to Qdrant.
6. **Snapshot Activation**: Atomically updates the project's active snapshot reference.
7. **Cleanup**: Deletes job-specific temporary files and worktrees while preserving the project-level Git cache.

Sources: [concept/tickets/backend-worker/10-full-index.md](), [concept/tickets/backend-worker/04-workspace.md](), [concept/worker/01-Components.md]()

### Communication Sequence
The interaction between the worker and external dependencies during indexing is shown below:

```mermaid
sequenceDiagram
    participant W as backend-worker
    participant G as Git Remote
    participant P as worker-parser-engine
    participant E as Embedding Provider
    participant DB as PostgreSQL/Qdrant

    W->>G: clone/fetch (SSH)
    G-->>W: repository data
    W->>P: ParseFiles(batch)
    P-->>W: symbols, chunks, imports
    W->>E: POST /api/embed
    E-->>W: vector arrays
    W->>DB: write artifacts & vectors
    W->>DB: update job status (completed)
```
Sources: [concept/worker/03-Communication-between-components-container.md](), [concept/tickets/backend-worker/10-full-index.md]()

## Operational Observability

Workers maintain a registry in Redis to allow the `backend-api` and backoffice to monitor system health.

### Worker Status Lifecycle
Each worker instance publishes a heartbeat every 10 seconds with a 30-second TTL. The status follows a specific state machine:
- `starting`: Initializing dependencies.
- `idle`: Ready to consume tasks.
- `busy`: Currently processing a job (includes `current_job_id` and `current_project_id`).
- `draining`: Shutting down gracefully; finishing current work but not accepting new tasks.

Sources: [backend-worker/README.md](), [concept/tickets/backend-worker/01-foundation.md](), [concept/tickets/backoffice/14-platform-admin.md]()

### Configuration Parameters
Workers are configured via environment variables to ensure compatibility with containerized environments.

| Variable | Description | Default/Example |
|---|---|---|
| `POSTGRES_DSN` | Connection string for the relational store. | `postgres://app:app@postgres:5432/...` |
| `REDIS_URL` | Endpoint for asynq and the worker registry. | `redis://redis:6379/0` |
| `REPO_CACHE_DIR` | Root directory for local Git clones. | `/var/lib/myjungle/repos` |
| `PARSER_POOL_SIZE` | Size of the in-process parser pool; `0` uses `runtime.NumCPU()`. | `4` |
| `PARSER_TIMEOUT_PER_FILE` | Per-file timeout for parser execution. | `30s` |
| `PARSER_MAX_FILE_SIZE` | Maximum file size accepted by the parser engine. | `10485760` |
| `SSH_KEY_ENCRYPTION_SECRET` | Secret used to decrypt project SSH keys. | (Required) |

Sources: [concept/tickets/backend-worker/01-foundation.md](), [backend-worker/README.md]()

## Summary

The Backend Worker processes are designed for high durability and project isolation. By separating the trigger (API) from execution (Worker) and utilizing a persistent job store in PostgreSQL, the system ensures that long-running indexing tasks can be retried, monitored, and scaled across multiple instances without losing state or compromising repository security. Sources: [concept/worker/00-Introduction-overview.md](), [concept/tickets/backend-worker/13-events-safety.md]()
