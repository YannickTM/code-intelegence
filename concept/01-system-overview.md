# System Overview v8

## Goal

Build a persistent, multi-project code intelligence platform that indexes repositories once, keeps them fresh incrementally, and exposes stable read-oriented APIs for AI coding agents and operator tooling.

The platform supports 28 languages out of the box, with parsing embedded directly in the worker process. MCP tools provide structured code context to AI agents without requiring per-session re-indexing.

## Component stack

| Component | Technology |
|---|---|
| Backend API | Go |
| Backend Worker | Go + embedded go-tree-sitter parser |
| MCP Server | TypeScript |
| Backoffice | Next.js + TypeScript + tRPC + Tailwind + better-auth |
| Relational Store | PostgreSQL |
| Vector Store | Qdrant |
| Queue + transient events | Redis + asynq |
| Embedding / LLM provider | Pluggable; phase 1 default: Ollama |
| Auth (interactive) | GitHub OAuth via better-auth |
| Auth (programmatic) | API keys |
| Runtime | Docker / Docker Compose |

## High-level topology

```
                          +-----------+
                          | AI Agent  |
                          +-----+-----+
                                |
                                v
                         +------+-------+
                         | mcp-server   |
                         +------+-------+
                                |
         +----------+           v            +-----------+
         |Backoffice+-----> backend-api <----+  SSE/Poll |
         +----------+      +----+----+       +-----------+
                           |    |    |
              +------------+    |    +------------+
              v                 v                 v
         PostgreSQL           Redis            Qdrant
              ^                 |                 ^
              |                 v                 |
              |          +-----------+            |
              +----------+  backend  +------------+
              |          |  worker   |            |
              |          +--+--+--+--+            |
              |             |  |  |               |
              |             |  |  |               |
              |             v  v  v               |
              |    +--------+--+--+--------+      |
              |    | embedded   | Ollama   |      |
              |    | parser     | (embed)  |      |
              |    | (go-tree-  +----------+      |
              |    |  sitter)              |      |
              |    +----------+------------+      |
              |               |                   |
              |               v                   |
              |          Git remote               |
              |          (SSH clone)               |
              +-----------------------------------+
```

Data flow summary:

- AI Agent calls MCP Server, which proxies to Backend API
- Backoffice calls Backend API directly (GitHub OAuth session)
- Backend API reads from PostgreSQL and Qdrant; enqueues work to Redis
- Backend Worker dequeues from Redis, clones repos via SSH, parses files with the embedded go-tree-sitter parser, generates embeddings via Ollama, and writes results to PostgreSQL and Qdrant

## Service boundaries

| Service | Owns | Talks to |
|---|---|---|
| `backend-api` | Project lifecycle, job creation, read APIs, auth | PostgreSQL, Qdrant, Redis |
| `backend-worker` | Workflow execution, parsing, embedding, snapshot activation | PostgreSQL, Qdrant, Redis, Git remotes, Ollama |
| `mcp-server` | Agent-facing MCP tool surface | `backend-api` only |
| `backoffice` | Operator UI for projects, jobs, keys, settings | `backend-api` only (+ SSE for live updates) |

Key constraint: `mcp-server` and `backoffice` never talk directly to PostgreSQL, Qdrant, or Redis. All data access goes through `backend-api`.

Parser logic (tree-sitter grammars, extractors) is internal to `backend-worker`. There is no separate parser service.

## Primary flows

### 1. Project setup

1. Operator authenticates via GitHub OAuth in the backoffice.
2. Operator creates or assigns an SSH key for repository access.
3. Operator creates a project with repository URL and branch defaults.
4. The project now has enough metadata for workflow execution.

### 2. Full indexing

1. Backoffice or API client calls `POST /v1/projects/{id}/index`.
2. `backend-api` validates the request and creates a durable job row in PostgreSQL.
3. `backend-api` enqueues a small workflow message to Redis/asynq.
4. `backend-worker` dequeues the job and acquires a PostgreSQL advisory lock for the project.
5. Worker loads the project execution context (repo URL, SSH key, embedding config) from PostgreSQL.
6. Worker clones or fetches the repository via SSH.
7. Worker walks the file tree, selecting files by language tier.
8. For each file, the embedded go-tree-sitter parser produces a syntax tree.
9. Extractors run against the syntax tree: symbols, imports, exports, references, JSX usages, network calls, chunks, diagnostics, file metadata.
10. Import paths are resolved to indexed files using the multi-strategy resolver (exact, extension, index-file, stem-based).
11. Text chunks are sent to the embedding provider (Ollama) for vector generation.
12. Artifacts are written to PostgreSQL; vectors are written to Qdrant.
13. Worker activates the new snapshot and marks the job completed.
14. Advisory lock is released.

### 3. Incremental refresh

1. Same entry path as full indexing.
2. Worker computes a diff against the last active snapshot (changed, added, deleted files).
3. Only affected files are re-parsed, re-embedded, and updated in the stores.
4. Snapshot is activated atomically.

### 4. Commits workflow

1. Triggered after indexing or on schedule.
2. Worker walks the git log for the project branch.
3. Commit metadata (hash, author, message, changed files) is persisted to PostgreSQL.
4. Supports downstream queries for file history and change frequency.

### 5. Query flow

1. AI agent calls an MCP tool (e.g., `search_code`, `get_symbol_info`).
2. `mcp-server` translates the tool call to a `backend-api` HTTP request.
3. `backend-api` reads from PostgreSQL (structured data) and Qdrant (vector similarity) as needed.
4. Results are returned through the MCP protocol to the agent.

Backoffice queries follow the same path but use tRPC/REST endpoints with GitHub OAuth sessions.

## Docker Compose services

The default `docker-compose.yml` defines 7 services:

| Service | Image | Port |
|---|---|---|
| `postgres` | PostgreSQL 16 | 5432 |
| `qdrant` | Qdrant latest | 6333/6334 |
| `redis` | Redis 7 | 6379 |
| `backend-api` | Custom Go build | 8080 |
| `backend-worker` | Custom Go build | (no public port) |
| `mcp-server` | Custom TS build | 3100 |
| `backoffice` | Next.js build | 3000 |

Ollama runs externally or as an optional compose profile. The worker connects to it via the configured embedding provider URL.

## Phase 1 scope

Delivered:

- Multi-project support with project-scoped isolation
- Project CRUD, SSH key management
- `full-index`, `incremental-index`, and `commits` workflows
- 28-language parsing via embedded go-tree-sitter (13 Tier 1, 15 Tier 2, Tier 3 fallback)
- Extractors: symbols, imports, exports, references, JSX usages, network calls, chunks, diagnostics, file metadata/facts
- Cross-language import resolution (exact, extension, index-file, stem-based)
- Embeddings via pluggable provider (Ollama default)
- PostgreSQL + Qdrant-backed read APIs
- 6 MCP tools: `search_code`, `get_symbol_info`, `get_dependencies`, `get_project_structure`, `get_file_context`, `get_conventions`
- GitHub OAuth for backoffice (better-auth)
- API keys for MCP and programmatic access
- Advisory locks for per-project serialized indexing
- Basic job monitoring and project health

Out of scope for phase 1:

- Workflow coordinators and chain engines
- Maintenance pipelines
- Global cross-project search
- Direct backend code-mutation APIs
- Enterprise auth complexity beyond GitHub OAuth

## Data ownership

| Store | Owns |
|---|---|
| PostgreSQL | Projects, SSH keys, jobs, snapshots, indexed artifacts (symbols, imports, exports, references, file metadata, commits), user sessions, API keys |
| Qdrant | Embedding vectors for code chunks, keyed by snapshot and file |
| Redis | Asynq task queue, transient SSE event fanout, ephemeral job progress |

PostgreSQL is the authoritative store. If Redis or Qdrant data is lost, it can be rebuilt from PostgreSQL state plus a re-index. Redis data is ephemeral by design.

## Security baseline

- GitHub OAuth for interactive sessions (backoffice)
- API keys for programmatic access (MCP server, external clients)
- SSH keys encrypted at rest in PostgreSQL
- Project-scoped isolation prevents cross-project data leakage
- `mcp-server` authenticates via API key; never accesses stores directly
- No secrets in queue payloads; workers load credentials at execution time
- Advisory locks prevent concurrent mutation of the same project's index
