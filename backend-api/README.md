# backend-api

## Role

The backend is split into two processes:

- `backend-api`: HTTP API, auth, query assembly, job enqueue, SSE bridge
- `backend-worker`: async pipeline execution for indexing and cleanup

Together they provide project lifecycle, key management, indexing orchestration, and retrieval logic.

## Responsibilities

- Manage users, projects, memberships, SSH keys, API keys, provider settings
- Enqueue and track indexing jobs via Redis/asynq
- Execute full and incremental indexing pipelines
- Run the embedded parser engine inside `backend-worker` for symbol/chunk extraction
- Call the configured embedding provider for vector generation
- Persist metadata/chunks in PostgreSQL and vectors in Qdrant
- Publish real-time job events for SSE consumers
- Serve read-oriented `/v1` query endpoints for MCP/backoffice

## Recommended Layout

```text
cmd/
  api/
  worker/
internal/
  app/
  project/
  sshkey/
  auth/
  indexing/
  query/
  queue/
  worker/
  events/
  storage/
    postgres/
    qdrant/
    ollama/
    git/
```

## HTTP Surface

### Public

- `POST /v1/users`
- `POST /v1/auth/login`

### Authenticated User Routes

- `POST /v1/auth/logout`
- `GET /v1/users/me`
- `PATCH /v1/users/me`
- `GET /v1/users/me/projects`
- `GET /v1/users/lookup?q={query}`

- `GET /v1/projects`
- `POST /v1/projects`

### SSH Keys

- `POST /v1/ssh-keys`
- `GET /v1/ssh-keys`
- `GET /v1/ssh-keys/{id}`
- `GET /v1/ssh-keys/{id}/projects`
- `POST /v1/ssh-keys/{id}/retire`

### Personal API Keys

- `GET /v1/users/me/keys`
- `POST /v1/users/me/keys`
- `DELETE /v1/users/me/keys/{keyID}`

### Providers

- `GET /v1/settings/providers`

### Platform Management (Platform Admin)

These routes require the `platform_admin` role via `RequirePlatformAdmin` middleware.

User management:
- `GET /v1/platform-management/users`
- `GET /v1/platform-management/users/{userId}`
- `PATCH /v1/platform-management/users/{userId}`
- `POST /v1/platform-management/users/{userId}/deactivate`
- `POST /v1/platform-management/users/{userId}/activate`

Platform roles:
- `GET /v1/platform-management/platform-roles`
- `POST /v1/platform-management/platform-roles`
- `DELETE /v1/platform-management/platform-roles/{userId}/{role}`

Global embedding settings:
- `GET /v1/platform-management/settings/embedding`
- `PUT /v1/platform-management/settings/embedding`
- `POST /v1/platform-management/settings/embedding`
- `POST /v1/platform-management/settings/embedding/test`
- `PATCH /v1/platform-management/settings/embedding/{configId}`
- `DELETE /v1/platform-management/settings/embedding/{configId}`
- `POST /v1/platform-management/settings/embedding/{configId}/promote`
- `POST /v1/platform-management/settings/embedding/{configId}/test`

Global LLM settings:
- `GET /v1/platform-management/settings/llm`
- `PUT /v1/platform-management/settings/llm`
- `POST /v1/platform-management/settings/llm`
- `POST /v1/platform-management/settings/llm/test`
- `PATCH /v1/platform-management/settings/llm/{configId}`
- `DELETE /v1/platform-management/settings/llm/{configId}`
- `POST /v1/platform-management/settings/llm/{configId}/promote`
- `POST /v1/platform-management/settings/llm/{configId}/test`

Worker status:
- `GET /v1/platform-management/workers`

### Dashboard and Events

- `GET /v1/dashboard/summary`
- `GET /v1/events/stream`

### Project-Scoped Member Routes

These routes require project membership. Some read/data routes also accept project or personal API keys.

- `GET /v1/projects/{projectID}`
- `GET /v1/projects/{projectID}/ssh-key`
- `GET /v1/projects/{projectID}/members`
- `GET /v1/projects/{projectID}/logs/stream`
- `GET /v1/projects/{projectID}/jobs`
- `POST /v1/projects/{projectID}/query/search`
- `GET /v1/projects/{projectID}/symbols`
- `GET /v1/projects/{projectID}/symbols/{symbolID}`
- `GET /v1/projects/{projectID}/dependencies`
- `GET /v1/projects/{projectID}/dependencies/graph`
- `GET /v1/projects/{projectID}/structure`
- `GET /v1/projects/{projectID}/files/context`
- `GET /v1/projects/{projectID}/files/dependencies`
- `GET /v1/projects/{projectID}/files/exports`
- `GET /v1/projects/{projectID}/files/references`
- `GET /v1/projects/{projectID}/files/jsx-usages`
- `GET /v1/projects/{projectID}/files/network-calls`
- `GET /v1/projects/{projectID}/files/history`
- `GET /v1/projects/{projectID}/conventions`
- `GET /v1/projects/{projectID}/commits`
- `GET /v1/projects/{projectID}/commits/{commitHash}`
- `GET /v1/projects/{projectID}/commits/{commitHash}/diffs`

### Project Provider Settings

Embedding:
- `GET /v1/projects/{projectID}/settings/embedding/available`
- `GET /v1/projects/{projectID}/settings/embedding`
- `PUT /v1/projects/{projectID}/settings/embedding`
- `DELETE /v1/projects/{projectID}/settings/embedding`
- `POST /v1/projects/{projectID}/settings/embedding/test`
- `GET /v1/projects/{projectID}/settings/embedding/resolved`

LLM:
- `GET /v1/projects/{projectID}/settings/llm/available`
- `GET /v1/projects/{projectID}/settings/llm`
- `PUT /v1/projects/{projectID}/settings/llm`
- `DELETE /v1/projects/{projectID}/settings/llm`
- `POST /v1/projects/{projectID}/settings/llm/test`
- `GET /v1/projects/{projectID}/settings/llm/resolved`

### Project Management

User-session only:
- `PATCH /v1/projects/{projectID}`
- `DELETE /v1/projects/{projectID}` owner only
- `PUT /v1/projects/{projectID}/ssh-key`
- `DELETE /v1/projects/{projectID}/ssh-key`
- `POST /v1/projects/{projectID}/members`
- `PATCH /v1/projects/{projectID}/members/{userID}`
- `DELETE /v1/projects/{projectID}/members/{userID}`

Project admin or stronger:
- `POST /v1/projects/{projectID}/index`
- `GET /v1/projects/{projectID}/keys`
- `POST /v1/projects/{projectID}/keys`
- `DELETE /v1/projects/{projectID}/keys/{keyID}`

### Health and Metrics

- `GET /health/live`
- `GET /health/ready`
- `GET /metrics`

## Worker Pipeline

1. Load active project SSH key assignment
2. Clone/pull repository with SSH key
3. Compute full or incremental file set from git state
4. Parse files through the embedded parser engine
5. Batch embeddings with the configured provider
6. Upsert Qdrant vectors
7. Persist snapshot/files/symbols/chunks/dependencies in PostgreSQL
8. Activate snapshot and emit terminal events

## Query Layer

The backend-api serves three query modes for code retrieval:

**Semantic search** (`POST /v1/projects/{projectID}/query/search`):
1. Embed query text using the project's resolved embedding provider
2. Execute vector similarity search in Qdrant against the project's active snapshot collection
3. Enrich results with PostgreSQL metadata (symbol details, file context)
4. Return ranked results with code snippets, file paths, line ranges, and similarity scores

**Text search** (full-text, regex, or exact match across `code_chunks.content` with glob-based path filtering)

**Symbol search** (`GET /v1/projects/{projectID}/symbols`): structured search across the `symbols` table with kind, name, and qualified-name filters

Results from all query modes are assembled using multi-source merge: Qdrant vector results are combined with structured PostgreSQL metadata, deduplicated, and trimmed to stay within configurable token budgets for agent context windows.

## Queue Model

- Redis-backed asynq queues with per-project fairness
- Retries: 3 for indexing, 1 for cleanup
- Timeouts: 30m full, 10m incremental
- Uniqueness window to avoid duplicate active jobs

## Runtime Dependencies

- PostgreSQL
- Qdrant
- Redis
- Embedding/LLM provider (bootstrap default: Ollama). The provider system is configurable, and alternative providers can be selected per-project through provider configuration.

## Configuration

For standalone backend-api runs, copy [`.env.example`](./.env.example) to `.env` (or another local env file) and export it before starting the service.

Key settings:
- `POSTGRES_DSN`: required PostgreSQL connection string
- `REDIS_URL`: Redis/asynq connection URL
- `OLLAMA_URL`, `OLLAMA_MODEL`, `OLLAMA_DIMENSIONS`: default embedding provider settings
- `LLM_PROVIDER`, `LLM_URL`, `LLM_MODEL`: default LLM provider settings
- `SSH_KEY_ENCRYPTION_SECRET`: required, used for stored SSH private keys
- `PROVIDER_ENCRYPTION_SECRET`: required, used for encrypted provider credentials
- `REPO_CACHE_DIR`: local git checkout/cache directory
- `SESSION_TTL`, `SESSION_COOKIE_NAME`, `SESSION_SECURE_COOKIE`: session behavior
- `CORS_ALLOWED_ORIGINS`, `CORS_WILDCARD`: browser access policy

Minimal example:

```env
SERVER_PORT=8080
POSTGRES_DSN=postgres://app:app@postgres:5432/codeintel?sslmode=disable
REDIS_URL=redis://redis:6379/0
OLLAMA_URL=http://host.docker.internal:11434
OLLAMA_MODEL=jina/jina-embeddings-v2-base-en
OLLAMA_DIMENSIONS=768
EMBED_BATCH_SIZE=64
LLM_PROVIDER=ollama
LLM_URL=http://host.docker.internal:11434
LLM_MODEL=codellama:7b
SSH_KEY_ENCRYPTION_SECRET=replace-me-with-a-long-random-secret
PROVIDER_ENCRYPTION_SECRET=replace-me-with-a-different-long-random-secret
REPO_CACHE_DIR=/var/lib/myjungle/repos
SESSION_TTL=24h
SESSION_COOKIE_NAME=session
SESSION_SECURE_COOKIE=false  # Local development only. Set true in production with HTTPS.
```

## Security Notes

- Never expose private keys through API responses
- Encrypt SSH keys and provider credentials at rest
- Never log full API bearer tokens
- In production, serve over HTTPS and set `SESSION_SECURE_COOKIE=true` so session cookies are not sent over plaintext HTTP
- Enforce project-scope authorization on every query endpoint

## Phase 1 Non-Goals

- MCP write operations (code changes)
- Cross-project semantic search
- Language support beyond TypeScript and JavaScript
