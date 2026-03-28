# 00 — Backend Worker: Component Overview

## Status
Done

## Goal
Define the architectural overview of `backend-worker`, the asynchronous workflow execution engine for the MYJUNGLE Code Intelligence Platform. This document describes the service's role, technology stack, internal package layout, and the ticket progression that built it.

## Depends On
—

## Scope

### Role

`backend-worker` is a single Go binary that:

- Dequeues jobs from Redis/asynq
- Loads project-scoped execution context from PostgreSQL
- Runs workflow handlers (full-index, incremental-index, commits, describe-full, describe-incremental, describe-file)
- Parses source files in-process using go-tree-sitter (28 languages)
- Generates embeddings via Ollama
- Generates LLM-powered file descriptions via Ollama
- Persists indexed artifacts and file descriptions to PostgreSQL and vectors to Qdrant
- Publishes lifecycle events over Redis pub/sub

The worker does not expose public HTTP endpoints. Its operational state is surfaced through PostgreSQL job state, Redis pub/sub events, and the Redis worker registry.

### Embedded Parser

The parser uses `smacker/go-tree-sitter`, which provides Go bindings to the tree-sitter C runtime. Each supported language has a compiled grammar loaded via the language registry. The engine processes files through a pipeline: normalize content, detect language, parse with tree-sitter, run extractors by tier, compute file facts, and emit metadata.

Parser pool: `sitter.Parser` is not goroutine-safe. The pool (`internal/parser/pool.go`) maintains a bounded set of pre-allocated parsers behind a buffered channel. Callers acquire a parser, set the language, parse, and return it. Pool size defaults to `runtime.NumCPU()`.

### Language Support (28 languages, 3 tiers)

**Tier 1 -- Full extraction** (symbols + imports + exports + references + chunks + diagnostics):
JavaScript, TypeScript, JSX, TSX, Python, Go, Rust, Java, Kotlin, C, C++, C#, Swift, Ruby, PHP

**Tier 2 -- Partial extraction** (symbols + imports + chunks + diagnostics):
Bash, SQL, GraphQL, Dockerfile, HCL (Terraform/OpenTofu)

**Tier 3 -- Structural only** (chunks + diagnostics, minimal symbols):
HTML, CSS, SCSS, JSON, YAML, TOML, Markdown, XML

### Technology Stack

| Layer | Technology |
|---|---|
| Language | Go 1.22+ |
| Queue | Redis / asynq |
| Database | PostgreSQL (pgx/v5, sqlc) |
| Parser | go-tree-sitter (CGO, 28 grammars) |
| Embeddings | Ollama (jina-embeddings-v2-base-en) |
| LLM | Ollama (configurable model) |
| Vectors | Qdrant |
| Events | Redis pub/sub |

### Service Layout

```
backend-worker/
  cmd/worker/main.go                  # Entrypoint
  internal/
    app/                              # Application bootstrap and wiring
    artifact/                         # PostgreSQL artifact writer
    config/                           # Environment-driven configuration
    description/                      # File description writer and schema
    embedding/                        # Ollama embedding client
    execution/                        # Execution context loader
    gitclient/                        # Git operations (clone, fetch, diff, log)
    indexing/                         # Storage pipeline + import resolution
    llmclient/                        # Provider-agnostic LLM completion client
    logger/                           # Structured logging (slog)
    notify/                           # Redis pub/sub event publisher
    parser/
      domain.go                       # Domain types
      pool.go                         # Bounded tree-sitter parser pool
      grammars.go                     # Grammar loading
      normalize.go                    # Content normalization
      engine/engine.go                # Pipeline orchestration per file
      extractors/                     # All extractors (symbols, imports, etc.)
      registry/                       # Language detection and config (28 langs)
    queue/                            # Asynq consumer and task definitions
    registry/                         # Worker heartbeat registry
    repository/                       # Job repository (claim, complete, fail, lock)
    sshenv/                           # SSH environment setup
    sshkey/                           # SSH key decryption
    storage/                          # Storage utilities
    vectorstore/                      # Qdrant client
    workflow/
      handler.go                      # Handler interface
      task.go                         # WorkflowTask type
      fullindex/handler.go            # Full-index workflow
      incremental/handler.go          # Incremental-index workflow
      describe/handler.go             # File description workflow (full, incremental, single-file)
      commits/indexer.go              # Git commit history indexer
    workspace/                        # Workspace preparation
  Dockerfile
  go.mod / go.sum
```

### Ticket Progression

| # | Title | Depends On |
|---|---|---|
| 01 | Worker Bootstrap, Config, Logging & Deployment | -- |
| 02 | Queue Consumer & Workflow Dispatcher | 01 |
| 03 | Job Repository & Execution Context | 01, 02 |
| 04 | Git Workspace & Repository Cache | 01, 03 |
| 05 | Embedded Tree-Sitter Parser Engine | -- |
| 06 | Symbol & Import Extraction + Cross-Language Resolution | 05 |
| 07 | Export, Reference, JSX & Network Extractors | 05, 06 |
| 08 | Chunking, Diagnostics & File Metadata | 05, 06 |
| 09 | Indexing Pipeline & Vector Persistence | 03, 04, 05 |
| 10 | Full-Index Workflow | 02, 03, 04, 05, 09 |
| 11 | Incremental-Index Workflow | 04, 09, 10 |
| 12 | Commit Indexing Workflow | 10, 11 |
| 13 | Event Publishing & Multi-Worker Safety | 01, 02, 10 |
| 14 | Stuck-Job Reaper Core & Worker Startup Sweep | 02, 03, 13 |
| 15 | LLM Client Package | -- |
| 16 | Description Schema, DB Migration & Storage Layer | -- |
| 17 | Description Workflow Handler (Full & Single-File) | 15, 16 |
| 18 | Incremental Description Workflow | 17 |

### Cross-Service Contracts

**Queue message** (`contracts/queue/workflow-task.v1.schema.json`):
The queue payload is a durable job reference with minimal routing hints (`job_id`, `workflow`, `enqueued_at`, optional `project_id`). The worker loads all execution details from PostgreSQL by `job_id`.

**SSE events** (`contracts/events/sse-event.v1.schema.json`):
The worker publishes lifecycle events (`job:started`, `job:progress`, `job:completed`, `job:failed`, `snapshot:activated`) to the `myjungle:events` Redis channel. The API subscribes and fans out to SSE clients.

**Worker registry** (`contracts/redis/worker-status.v1.schema.json`):
Each worker publishes ephemeral status (`starting`, `idle`, `busy`, `draining`) to `worker:status:{worker_id}` with 30s TTL, refreshed every 10s.

## Key Files

| File/Package | Purpose |
|---|---|
| `cmd/worker/main.go` | Process entrypoint |
| `internal/config/config.go` | Environment-based configuration |
| `internal/app/app.go` | Bootstrap and dependency wiring |
| `internal/parser/domain.go` | Domain types for parser output |
| `internal/parser/registry/languages.go` | 28 language configurations |
| `internal/parser/engine/engine.go` | Parser engine pipeline |
| `internal/workflow/fullindex/handler.go` | Full-index workflow |
| `internal/workflow/incremental/handler.go` | Incremental-index workflow |
| `internal/workflow/describe/handler.go` | File description workflow |
| `internal/llmclient/client.go` | LLM completion interface |
| `internal/description/writer.go` | File description persistence |
| `internal/indexing/pipeline.go` | Storage pipeline |
| `internal/indexing/resolve.go` | Import resolution |

## Acceptance Criteria
- [x] Service compiles as a single Go binary with CGO enabled
- [x] Supports 28 languages across 3 extraction tiers
- [x] Executes full-index and incremental-index workflows end-to-end
- [x] Persists artifacts to PostgreSQL and vectors to Qdrant
- [x] Publishes lifecycle events over Redis pub/sub
- [x] Scales horizontally via Redis/asynq queue competition
- [x] All `go test ./...` pass
