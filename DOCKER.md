# Docker Runbook

## Purpose

This repository uses Docker Compose as the baseline runtime for local development and initial self-hosted staging.

## Compose Profiles

- Default profile: infrastructure services only (`postgres`, `qdrant`, `redis`)
- `app` profile: `backend-api`, `backend-worker`, `mcp-server`, `backoffice`

This allows infra to start independently. The `app` profile builds local images from service Dockerfiles.

## Build Sources

Compose builds the `app` profile from local Dockerfiles:

- `backend-api` -> `./backend-api/Dockerfile` (context: repo root)
- `backend-worker` -> `./backend-worker/Dockerfile` (context: repo root)
- `mcp-server` -> `./mcpserver/Dockerfile`
- `backoffice` -> `./backoffice/Dockerfile`

## Prerequisites

- Docker Engine 24+
- Docker Compose v2+
- Optional GPU runtime for Ollama acceleration

## Startup

### Start infrastructure

```bash
docker compose up -d
```

### Start backend only (for local frontend development)

```bash
docker compose -f docker-compose.backend.yaml up -d --build
# or: make backend-up
```

Starts infra + `backend-api`, `backend-worker`, `mcp-server`. No backoffice container — run the frontend locally against `http://localhost:8080`.

### Start full stack

```bash
docker compose --profile app up -d --build
```

### Stop stack

```bash
docker compose down
```

### Stop and remove volumes

```bash
docker compose down -v
```

## Service Endpoints

- Backoffice: `http://localhost:5173`
- Backend API: `http://localhost:8080`
- MCP server (SSE/HTTP): `http://localhost:3000`
- Qdrant: `http://localhost:6333`
- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- Ollama: external via `OLLAMA_URL` (example: `http://host.docker.internal:11434`)

## Environment Overrides

Create `.env` in repository root to override defaults from `docker-compose.yaml`.

Typical keys:

```env
POSTGRES_DB=codeintel
POSTGRES_USER=app
POSTGRES_PASSWORD=app
SSH_KEY_ENCRYPTION_SECRET=replace-this-secret
OLLAMA_MODEL=jina/jina-embeddings-v2-base-en
OLLAMA_URL=http://host.docker.internal:11434
API_BASE_URL=http://backend-api:8080
BETTER_AUTH_SECRET=replace-this-better-auth-secret-replace-this-better-auth-secret
BETTER_AUTH_URL=http://localhost:5173
NEXT_PUBLIC_OIDC_PROVIDER_ID=oidc
# OIDC_DISCOVERY_URL=https://your-oidc-provider.example.com/.well-known/openid-configuration
# OIDC_CLIENT_ID=your-client-id
# OIDC_CLIENT_SECRET=your-client-secret
```

## Health Verification

```bash
docker compose ps
docker compose logs -f backend-api backend-worker mcp-server backoffice
```

Infra checks:

```bash
curl -fsS http://localhost:6333/healthz
curl -fsS "${OLLAMA_URL:-http://host.docker.internal:11434}/api/tags"
```

## Data Persistence

Named volumes:

- `postgres_data`
- `qdrant_data`
- `redis_data`
- `repo_cache`

## Troubleshooting

### App profile containers are not starting

Cause: a service build fails (for example missing source scaffolding or dependency issues).

Action:

- Run `docker compose --profile app build` and fix the first failing service.
- Ensure each service has required source files (for example `backend-api`/`backend-worker` `go.mod` and `cmd/*` entries, plus any parser grammar assets the worker build expects).
- Re-run with `--no-cache` if dependency layers are stale.

### Ollama is healthy but embeddings fail

- Validate configured model exists in Ollama (`/api/tags`)
- Check backend embedding settings endpoint
- Trigger test endpoint: `POST /v1/settings/embedding/test`

### SSE stream disconnects

- Confirm backend API is healthy
- Check reverse proxy timeout settings (if any)
- Verify `GET /v1/events/stream` connectivity

### Slow indexing

- Reduce embedding batch size
- Confirm Redis and PostgreSQL are not resource constrained
- Verify backend-worker CPU saturation during parser-heavy indexing
