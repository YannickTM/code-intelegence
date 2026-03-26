# Architecture

## Goal

Provide persistent, multi-project code intelligence to AI agents through stable MCP tools, while keeping retrieval quality high and operational complexity controlled.

## Problem Statement

AI coding agents face a cold-start problem on every interaction: they must re-read, re-parse, and re-understand code each session. No persistent memory of project structure, conventions, or relationships carries over. Semantic questions like "find the auth handler" fall back to manual grep-style searching, and no shared understanding exists across agent sessions or team members.

This platform solves the problem by ingesting repositories once (clone, parse, chunk, embed), storing knowledge in both vector (semantic) and relational (structured) databases, keeping the index fresh incrementally, and serving contextual code intelligence to agents via standardized MCP tools.

## Core Principles

- Persist understanding; avoid cold-start analysis per agent session
- Separate semantic retrieval from structured metadata/state
- Keep MCP surface stable while internals evolve
- Run on Docker as deployment baseline
- Enforce project-scoped access everywhere

## High-Level System

```text
AI Agent
  -> MCP Server (TypeScript, stdio/SSE)
    -> Backend API (Go, /v1)
      -> PostgreSQL (metadata, auth, raw code chunks)
      -> Qdrant (query-time retrieval)
      -> Redis (task enqueue + SSE event bridge)

Backoffice (Next.js) -> Backend API (tRPC + SSE stream)
Backend Worker (Go) -> Redis dequeue -> Embedded Parser Engine (tree-sitter)
Backend Worker (Go) -> External Ollama (/api/embed)
Backend Worker (Go) -> PostgreSQL + Qdrant upserts + Redis pub/sub events
```

## Runtime Flows

### Project Setup

1. Create SSH key in backoffice key library
2. Create project and assign one SSH key
3. Add public key to Git provider deploy keys
4. Backend can clone/pull with assigned key

### Indexing

1. Backoffice or scheduler triggers indexing
2. Backend API creates `indexing_jobs` row and enqueues task
3. Worker clones/pulls and computes full/incremental scope
4. Worker parses changed files with the embedded parser engine
5. Worker embeds chunks with Ollama
6. Worker writes vectors to Qdrant and metadata/chunks to PostgreSQL
7. Worker publishes progress events via Redis pub/sub
8. Backend SSE bridge streams status to backoffice

### Query

1. Agent calls MCP tool with `project_id`
2. MCP server validates input and forwards bearer token
3. Backend checks API key scope for target project
4. Backend embeds query text using the project's resolved embedding provider
5. Backend runs vector similarity search in Qdrant against the active snapshot
6. Backend enriches results with PostgreSQL metadata (symbols, file context, related symbols)
7. Backend deduplicates, ranks by combined relevance, and trims to token budget
8. Backend returns compact structured payload with file paths, line numbers, and freshness metadata

### Incremental Refresh

1. Trigger incremental job
2. Resolve active snapshot for project+branch; verify embedding version compatibility
3. Compute `git diff --name-status` between snapshot base commit and HEAD
4. Changed files: re-parse via the embedded parser engine, re-chunk, re-embed, upsert new vectors and artifacts
5. Unchanged files: copy-forward all artifacts (files, symbols, chunks, dependencies, exports, references) from old snapshot to new with remapped IDs
6. Copy-forward vectors: batch-read old vectors from Qdrant, re-upsert with new chunk IDs and updated snapshot metadata
7. Delete vectors for removed files/symbols
8. Activate new snapshot atomically (deactivate old, activate new)

Full re-index triggers: embedding model change, chunk strategy change, no active snapshot, diff computation failure, manual trigger from backoffice.

## Service Boundaries

- MCP server only talks to backend HTTP
- Backoffice only talks to backend HTTP + SSE
- `backend-api` owns HTTP auth, orchestration, and query assembly
- `backend-worker` owns async job execution pipeline and the embedded parser engine
- Redis used only by backend API/worker for queue + pub/sub
- Ollama is accessed by `backend-worker` (batch embedding during indexing) and `backend-api` (single query embedding during search)

## Chunking Strategy

The platform uses a hybrid chunking approach with a three-tier language system:

- **Tier 1** (full extraction — JS, TS, TSX, JSX): symbols, imports, exports, references, JSX usages, network calls, individual function/class/module-context chunks
- **Tier 2** (partial — Go, Python, Rust, Java, etc.): symbols, imports, chunks, diagnostics
- **Tier 3** (structural — YAML, JSON, Markdown, etc.): single module-context chunk per file

Chunk types: `function`, `class`, `module_context`, `config`, `test`. Each chunk carries a content hash (SHA-256) for change detection, a semantic role (`implementation`, `api_surface`, `config`, `test`, `ui_component`, `hook`), owner symbol info, and estimated token count. Config and test files receive special handling with dedicated chunk types.

## Data Ownership

### PostgreSQL

- Projects, project groups
- SSH key library + project assignments
- API keys + project access mapping
- Embedding config + embedding versions
- Snapshots, jobs, files, symbols, dependencies
- Raw chunk text (`code_chunks`)
- Query logs and analytics base

### Qdrant

- Per-project and per-embedding-version vector collections
- Metadata filters for language/symbol/path/snapshot
- Pointer from vector payload to raw chunk in PostgreSQL

### Redis

- asynq tasks for indexing and cleanup
- pub/sub stream for real-time job and snapshot events

## Deployment Baseline

Compose services:

- `postgres`
- `qdrant`
- `redis`
- `backend-api`
- `backend-worker`
- `mcp-server`
- `backoffice`

External dependency:

- `ollama` (host-local or remote endpoint via `OLLAMA_URL`)

## Capacity Planning

Vector count depends on language tier and symbol density:

- ~50–200 vectors per Tier 1 source file, 1 vector per Tier 3 file
- Small project (1k files): ~100k vectors
- Medium project (10k files): ~1M vectors
- Large monorepo (100k files): ~10M vectors (may need Qdrant cluster mode)

Vector dimensions depend on the configured embedding model (e.g., 768 for default Ollama Jina model).

## API and Tool Contracts

- Backend versioned under `/v1`
- MCP tools in phase 1:
  - `search_code`
  - `get_symbol_info`
  - `get_dependencies`
  - `get_project_structure`
  - `get_file_context`
  - `get_conventions`
- Tool contract changes are treated as versioned compatibility changes

## Security and Access

- API key checks enforced by backend per request and per project
- Backoffice auth via OIDC (better-auth)
- SSH private keys encrypted at rest in PostgreSQL
- Secrets injected via environment variables
- Internal network exposure minimized through Docker networking

## Non-Goals in Phase 1

- MCP write/edit tools
- Cross-project global semantic search
- Enterprise RBAC/SSO
- Languages beyond TypeScript/JavaScript

## ADR Snapshot

- ADR-001: MCP and backend remain separate services
- ADR-002: Docker Compose is baseline deployment model
- ADR-003: Dual-store architecture (Qdrant + PostgreSQL)
- ADR-004: Embed version isolation per collection
- ADR-006: Incremental indexing default, full reindex explicit
- ADR-013: Redis + asynq for queues, Redis pub/sub for events
- ADR-014: SSE for backoffice real-time updates
- ADR-015: Reusable Ed25519 SSH key pairs managed in platform
- ADR-016: Ollama as embedding provider
- ADR-018: Backoffice auth via OIDC (better-auth)
