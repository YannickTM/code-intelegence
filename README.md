# MYJUNGLE Code Intelligence Platform

MYJUNGLE is a persistent code intelligence platform for AI coding agents.
It indexes repositories once, keeps them fresh incrementally, and exposes read-oriented MCP tools for high-quality code context retrieval.

## Why

AI coding agents re-analyze code from scratch every session — no persistent memory of project structure, conventions, or symbol relationships carries over. MYJUNGLE provides a persistent, pre-indexed knowledge layer so agents query structured code intelligence instead of re-reading everything.

## Phase 1 Scope

- Multi-project support from day one
- One repository per project
- One active branch per project
- Read-only MCP tools (no code write/edit tools)
- Git authentication through reusable SSH key pairs
- Embeddings via self-hosted Ollama
- Backoffice with OIDC auth via better-auth

## What Gets Indexed

Each repository is parsed into embeddable code units:

- Functions and methods (individual chunks)
- Classes and interfaces (declaration chunks, separate from method bodies)
- Module-level code (imports, constants, top-level statements)
- Configuration files (route definitions, middleware setup)
- Test cases (split by test block or full-file)

Every indexed unit carries metadata: file path, language, symbol name and kind, line range, semantic role, and content hash for incremental change detection.

## Component Stack

- Backend API: Go
- Backend Worker: Go
- MCP Server: TypeScript + `@modelcontextprotocol/sdk`
- Backoffice: Next.js + TypeScript + tRPC + Tailwind CSS
- Relational DB: PostgreSQL
- Vector DB: Qdrant
- Queue + Pub/Sub: Redis + asynq
- Embeddings: external Ollama (`/api/embed`)

## Runtime Topology

- Agents call MCP tools through the MCP server (stdio or SSE mode)
- MCP server calls `backend-api` HTTP `/v1` endpoints
- `backend-api` validates auth, serves queries, and enqueues jobs
- `backend-worker` executes indexing pipeline tasks from Redis/asynq
- `backend-worker` runs the embedded tree-sitter parser engine for parse output
- `backend-worker` calls external Ollama for embeddings
- Worker stores metadata/raw code in PostgreSQL and vectors in Qdrant
- Worker emits events via Redis pub/sub
- Backoffice receives updates from `backend-api` SSE bridge

## Project Layout

- `ARCHIVE/concept/`: original architecture concept docs (archived — implementation has diverged)
- `backend-api/`: API service workspace (`Dockerfile`, service docs)
- `backend-worker/`: worker service workspace (`Dockerfile`, service docs)
- `ARCHIVE/sidecar-tsjs/`: archived TS/JS parser workspace from the previous design
- `backoffice/`: Next.js backoffice UI (T3 stack, tRPC, better-auth)
- `mcpserver/README.md`: MCP server contract and tool mappings
- `datastore/POSTGRES.md`: relational schema and data ownership
- `datastore/QDRANT.md`: vector model, payload schema, query patterns
- `datastore/REDIS.md`: queue and pub/sub runtime model
- `docker-compose.yaml`: Docker baseline stack for local/dev and early staging
- `DOCKER.md`: Docker runbook and operational notes
- `ARCHITECTURE.md`: consolidated system architecture and ADR summary

## Quick Start

1. Start infra services only:

```bash
docker compose up -d
```

2. Verify infra health:

```bash
docker compose ps
```

3. Start full stack (builds app images from local Dockerfiles):

```bash
docker compose --profile app up -d
```

See `DOCKER.md` for env overrides, health checks, and troubleshooting.

## Default Ports

- Backoffice UI: `3000`
- Backend API/SSE: `8080`
- MCP HTTP/SSE: `4444`
- PostgreSQL: `5432`
- Qdrant: `6333`
- Redis: `6379`
- Ollama: external via `OLLAMA_URL` (example `http://host.docker.internal:11434`)

## Security Baseline

- API keys are hashed and scoped to projects via join table
- SSH private keys are encrypted at rest in PostgreSQL
- Internal services communicate on a dedicated Docker network
- MCP server does not directly access PostgreSQL/Qdrant/Redis/Ollama

## Status

Core services are implemented and operational: backend-api (HTTP + SSE), backend-worker (full and incremental indexing pipelines), MCP server (6 tools), and backoffice (project management, search, symbol browser, commit history, provider settings). The parser subsystem supports three-tier language extraction. Incremental indexing with artifact and vector copy-forward is functional.
