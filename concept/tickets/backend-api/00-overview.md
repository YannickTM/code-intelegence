# 00 — Backend API Component Overview

## Status
Done

## Role

Go HTTP API service serving as the platform core. Listens on port 8080 via a Chi v5 router with a full middleware chain (requestid, logging, metrics, recover, CORS, bodylimit). Handles user identity, project lifecycle, SSH key management, API key authentication, provider settings, index job orchestration, real-time SSE events, and query execution for MCP tools.

## Tech Stack

| Tool | Purpose |
|------|---------|
| Go 1.24 | Language runtime |
| Chi v5 | HTTP router and middleware |
| pgx/v5 | PostgreSQL driver and connection pool |
| sqlc | Type-safe SQL query generation |
| golang-migrate | Database schema migrations |
| asynq | Redis-backed async job queue |
| log/slog | Structured, leveled logging (stdlib) |
| crypto/ed25519 | SSH key generation |
| crypto/aes | AES-256-GCM encryption for secrets at rest |
| testcontainers-go | Integration test infrastructure |
| Prometheus | Metrics exposition |

## Service Layout

```text
cmd/
  api/                    # HTTP server entrypoint (main.go, ~33 lines)
internal/
  app/                    # App struct, DI wiring, route registration
  config/                 # Env-based configuration loader with validation
  domain/                 # Shared domain models and error types
  handler/                # HTTP handlers (auth, project, sshkey, apikey, embedding, llm, provider, dashboard, event, admin, worker, health, membership, commits)
  middleware/             # identity, requireuser, projectrole, platformadmin, cors, logging, metrics, recover, requestid, bodylimit
  auth/                   # Session context helpers and token utilities
  membership/             # Project member management with role invariants
  sshkey/                 # SSH key generation, encrypted storage, assignment
  apikey/                 # API key hashing, creation, revocation
  embedding/              # Embedding provider settings service (global + project)
  llm/                    # LLM provider settings service (global + project)
  providers/              # Pluggable provider registry, connectivity testing, SSRF allowlist
  providersetting/        # Shared helpers for embedding/llm settings
  secrets/                # AES-256-GCM encrypt/decrypt for stored credentials
  queue/                  # Redis/asynq job producer
  sse/                    # SSE hub, publisher, Redis pub/sub subscriber
  storage/postgres/       # pgxpool setup, migration runner, transaction helper
  dbconv/                 # DB row to domain model converters
  validate/               # Input validation helpers
  health/                 # Health check service
  metrics/                # Prometheus metric collectors
  logger/                 # Structured logging setup
  redisclient/            # Shared Redis connection
  testutil/               # Test helpers
tests/
  integration/            # 223+ integration tests via testcontainers-go
```

## Ticket Progression

| # | Ticket | Title | Depends On |
|---|--------|-------|------------|
| 00 | `00-overview` | Component overview | -- |
| 01 | `01-foundation` | Foundation (server, config, DB, logging, health, errors) | -- |
| 02 | `02-authentication` | Session-based and API key authentication | 01 |
| 03 | `03-user-management` | User registration, profile, and lookup | 01, 02 |
| 04 | `04-project-lifecycle` | Project CRUD, structure, and health | 01, 02, 05 |
| 05 | `05-ssh-key-management` | SSH key CRUD with encryption | 01, 02 |
| 06 | -- | Project membership and RBAC | 02, 04 |
| 07 | -- | API key management (project + personal) | 02, 04, 06 |
| 08 | -- | Embedding settings endpoints | 01, 02 |
| 09 | -- | LLM + embedding provider settings | 08, 02, 06 |
| 10 | -- | Dashboard API | 04, 06 |
| 11 | -- | Index job creation | 04, 08, 09 |
| 12 | -- | Index job enqueue and queue consistency | 11 |
| 13 | -- | SSE streaming infrastructure | 02, 06 |
| 14 | -- | File structure and content endpoints | 11 |
| 15 | -- | Commit history and diff endpoints | 11 |
| 16 | -- | Platform admin and global settings | 02, 06, 08, 09 |

## API Route Summary

### Health and Metrics (unauthenticated)
- `GET /health/live` -- liveness probe
- `GET /health/ready` -- readiness probe (checks Postgres, Redis, Qdrant)
- `GET /metrics` -- Prometheus metrics

### Public
- `POST /v1/users` -- register user
- `POST /v1/auth/login` -- create session

### User-Only (session required)
- `POST /v1/auth/logout`
- `GET /v1/users/me`, `PATCH /v1/users/me`
- `GET /v1/users/me/projects`
- `GET /v1/users/lookup?q={query}`
- `GET /v1/projects`, `POST /v1/projects`
- SSH key library: `POST`, `GET /v1/ssh-keys`, `GET /v1/ssh-keys/{id}`, `GET /v1/ssh-keys/{id}/projects`, `POST /v1/ssh-keys/{id}/retire`
- Personal API keys: `POST`, `GET`, `DELETE /v1/users/me/keys`
- `GET /v1/settings/providers`
- `GET /v1/dashboard/summary`
- `GET /v1/events/stream` (SSE)

### Platform Management (platform admin)
- User management: `GET`, `GET/{id}`, `PATCH/{id}`, deactivate, activate under `/v1/platform-management/users`
- Platform roles: `GET`, `POST`, `DELETE` under `/v1/platform-management/platform-roles`
- Global embedding settings: CRUD, promote, test under `/v1/platform-management/settings/embedding`
- Global LLM settings: same structure under `/v1/platform-management/settings/llm`
- Worker status: `GET /v1/platform-management/workers`

### Project-Scoped (dual auth: session or API key)
- Member-level read: `GET /v1/projects/{id}`, members, jobs, SSH key, query/search, symbols, dependencies, structure, files, conventions, commits, settings read
- Admin-level: `POST /v1/projects/{id}/index`, `PATCH`, SSH key PUT/DELETE, members POST/PATCH, settings write, project API keys
- Owner-level: `DELETE /v1/projects/{id}`
