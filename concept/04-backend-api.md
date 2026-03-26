# Backend API (Go)

## Role

The backend-api is the platform core:

- GitHub OAuth identity via better-auth (delegated to backoffice) and session-based authentication
- API key authentication for MCP and programmatic access
- Project-scoped role-based authorization (`owner > admin > member`)
- Platform admin role for global settings management
- Project lifecycle management
- SSH key lifecycle management and Git authentication
- Indexing orchestration (job creation and enqueue; execution is worker-side)
- Query execution and response assembly for MCP tools
- Pluggable provider management (embedding + LLM) with connectivity testing
- Two-tier provider settings resolution (project override, global default)
- Job queue publishing via Redis/asynq
- Real-time event publishing via SSE hub
- Internal API for MCP server and backoffice

The backend-api does not parse code or execute indexing pipelines. It creates `indexing_jobs` rows and enqueues tasks to Redis. The `backend-worker` dequeues and executes those tasks using its embedded parser and the configured embedding provider.

## Service Layout

```text
cmd/
  api/                    # HTTP server entrypoint
internal/
  app/                    # wiring, config, DI, route registration
    app.go                # App struct, dependency construction
    routes.go             # chi router tree
  config/                 # env-based configuration loader
  domain/                 # shared domain models and error types
  handler/                # HTTP handler implementations
    auth.go               # login/logout
    project.go            # project CRUD, indexing trigger, query endpoints
    membership.go         # project member management
    sshkey.go             # SSH key library endpoints
    apikey.go             # project + personal API key endpoints
    embedding.go          # embedding provider settings (global + project)
    llm.go                # LLM provider settings (global + project)
    provider.go           # supported provider catalog
    dashboard.go          # cross-project dashboard summary
    event.go              # SSE stream handler
    admin.go              # platform admin user/role management
    worker.go             # worker status (read-only)
    health.go             # liveness/readiness/metrics
  middleware/
    identity.go           # identity resolution (API key or session)
    requireuser.go        # reject non-user (API-key-only) requests
    projectrole.go        # project RBAC enforcement
    platformadmin.go      # platform admin gate
    cors.go               # CORS policy
    logging.go            # structured request logging
    metrics.go            # Prometheus request metrics
    recover.go            # panic recovery
    requestid.go          # request ID propagation
    bodylimit.go          # request body size limit
  auth/                   # session context helpers + token utilities
  project/                # project lifecycle use-cases (if separated from handler)
  membership/             # project member management (roles/invariants)
  sshkey/                 # SSH key generation, encrypted storage, assignment
  apikey/                 # API key hashing, creation, revocation
  embedding/              # embedding provider settings service (global + project)
  llm/                    # LLM provider settings service (global + project)
  providers/              # pluggable provider registry + connectivity testing
    registry.go           # supported provider catalog
    types.go              # Capability, ProviderInfo, ConnectivityResult
    connectivity.go       # dispatch connectivity tests by provider
    connectivity_allowlist.go  # endpoint allowlist (SSRF protection)
    ollama.go             # Ollama-specific connectivity probe
    endpoint.go           # endpoint URL validation
  providersetting/        # shared helpers for embedding/llm settings
    common.go             # credential resolution, field validation, locking
  secrets/                # AES-256-GCM encrypt/decrypt for stored credentials
  queue/                  # Redis/asynq job producer
    publisher.go          # EnqueueIndexJob, workflow mapping
  sse/                    # real-time event system
    hub.go                # in-memory client registry, project-scoped broadcast
    publisher.go          # publish events to hub
    subscriber.go         # Redis pub/sub → hub bridge
  storage/
    postgres/             # sqlc-based repos, migration runner, tx helper
  dbconv/                 # DB row ↔ domain model converters
  validate/               # shared input validation helpers
  health/                 # health check service
  metrics/                # Prometheus metric collectors
  logger/                 # structured logging setup
  redisclient/            # shared Redis connection
  testutil/               # test helpers
```

Note: there is no `storage/parser/` or gRPC client. Parsing is embedded in the `backend-worker` process; the API never calls a parser directly.

## Authentication

### GitHub OAuth (Backoffice Sessions)

Backoffice authentication uses GitHub OAuth, managed by the `better-auth` library running inside the backoffice (Next.js) service. The flow:

1. User initiates GitHub OAuth from the backoffice UI
2. Backoffice handles the OAuth callback and provisions a user record
3. Backoffice creates a session and sets a session cookie
4. Backend-api validates the session token (from cookie or `Authorization: Bearer <token>`) against the sessions table

Session tokens are SHA-256 hashed before storage. Only the hash is persisted; the raw token is returned to the client once. Session TTL, cookie name, and secure-cookie flag are configurable via environment variables.

### API Key Authentication (MCP / Programmatic)

API keys provide machine-auth for MCP server requests and automation:

- All API keys use the `mj_` prefix for identification
- Keys are SHA-256 hashed before storage (same as session tokens)
- Two key types: **project-scoped** (bound to a single project with a fixed role) and **personal** (inherit the user's memberships)
- The `IdentityResolver` middleware detects `mj_`-prefixed Bearer tokens and resolves them via the `api_keys` table

### Authorization Model

Authorization is always project-scoped and resolved from `project_members`:

- Role hierarchy: `owner > admin > member`
- `owner`: full control including project deletion, owner grants, SSH key management
- `admin`: operational control (indexing, settings, SSH key reassignment, invites up to admin, provider settings, API key management)
- `member`: read/query access

Ownership invariants enforced in services:

- A project must always have at least one owner
- Admins cannot grant, remove, or demote owners
- Last owner cannot remove or demote themselves

### Platform Admin

A separate `platform_admin` role grants access to global settings management (embedding/LLM provider defaults, user management, platform role grants). Platform admin status is resolved from the `user_platform_roles` table and enforced by the `RequirePlatformAdmin` middleware.

### Identity Resolution Flow

```text
Request arrives at /v1/*
  → IdentityResolver middleware runs:
    1. Authorization: Bearer <mj_*>  → resolve as API key identity
    2. Authorization: Bearer <token>  → resolve as session (hash + DB lookup)
    3. Cookie (session_cookie_name)   → resolve as session (hash + DB lookup)
    4. None matched                   → continue as anonymous
  → Downstream middleware enforces access:
    - RequireUser: rejects API-key-only and anonymous requests
    - RequireProjectRole: resolves membership for both user sessions and API keys
    - RequirePlatformAdmin: checks platform_admin role
```

Project-scoped failure behavior:

- Return `404 Not Found` when no membership exists (avoids project existence leaks)
- Return `403 Forbidden` when membership exists but role is insufficient

## SSH Key Management

### Key Ownership

Each user maintains a private SSH key library. Keys are scoped by `created_by` -- users can only list, view, and retire their own keys via the `/v1/ssh-keys` endpoints. When a key is assigned to a project, all project members can view the public key and fingerprint through the project endpoint. Only project owners and admins can reassign, generate, or remove key assignments.

### Key Generation

1. Generate key pair using Go's `crypto/ed25519`
2. Encode public key in OpenSSH authorized_keys format
3. Encrypt private key using AES-256-GCM (via `secrets` package) before storage
4. Store both in the `ssh_keys` table with fingerprint and `created_by = current user`
5. Return key metadata (id, public key, fingerprint) to the caller

### Project Assignment

- Project creation requires an `ssh_key_id` in the request body (key must belong to the creator)
- Reassignment deactivates the prior assignment; the new key must belong to the caller
- Generate-and-assign creates a new key in the caller's library and assigns it atomically
- A single SSH key can be assigned to multiple projects
- All project members can view the assigned public key; only owner/admin can modify assignments

### Git Operations

All git clone/pull operations (executed by the worker) resolve the project's active SSH key assignment:

- Decrypt the referenced private key and write to a temporary file (mode 0600)
- Set `GIT_SSH_COMMAND` to use the temp key file
- Clean up the temp key file after the git operation completes

## Provider Architecture

### Pluggable Provider Registry

The `providers` package defines a capability-based registry for embedding and LLM providers:

```text
Capability: "embedding" | "llm"

ProviderInfo {
    ID           string           // e.g. "ollama"
    Label        string           // human-readable name
    Capabilities []Capability     // what this provider can do
    Tester       ConnectivityTesterFn  // probe function
}
```

Providers are registered in `registry.go` with their supported capabilities. The registry exposes:

- `SupportedProviderIDs(capability)` -- list providers for a capability
- `SupportedProviderCatalog()` -- full catalog keyed by capability
- `ValidateProvider(capability, providerID)` -- check if a provider supports a capability
- `TestEmbeddingConnectivity` / `TestLLMConnectivity` -- dispatch connectivity probes

Currently Ollama is the supported provider for both embedding and LLM capabilities. The registry is designed for extension with additional providers.

### Connectivity Testing

Connectivity tests verify that a configured provider endpoint is reachable and the specified model is available. Tests are dispatched through the registry to provider-specific probe functions.

### Endpoint Allowlist (SSRF Protection)

The `connectivity_allowlist.go` module restricts which hosts the backend-api will probe during connectivity tests:

- When `PROVIDER_CONNECTIVITY_ALLOWED_HOSTS` is set, only listed hosts are permitted
- When unset, only loopback and `host.docker.internal` addresses are allowed by default
- DNS resolution is checked to prevent rebinding attacks (all resolved IPs must be loopback)

## Provider Settings

### Two-Tier Resolution Pattern

Both embedding and LLM settings follow the same resolution pattern. Each project can operate in one of three modes:

1. **Default** -- uses the platform default global config (no project-level selection)
2. **Global** -- project explicitly selects a specific global config by ID
3. **Custom** -- project defines its own provider, endpoint, model, and credentials

Resolution order when reading the effective config:

```text
1. Active project-owned custom config?  → return it (source: "custom")
2. Explicit global config selection?     → return it (source: "global")
3. Fall through to default global config → return it (source: "default")
```

### Credential Management

Provider credentials (API keys, tokens) are encrypted at rest using AES-256-GCM via the `secrets` package, keyed by `PROVIDER_ENCRYPTION_SECRET`. The `providersetting` package handles credential lifecycle:

- Credentials are carried forward on updates unless the provider or endpoint changes
- When provider or endpoint changes, credentials must be explicitly re-entered
- Credentials can be explicitly cleared

### Global Config Management (Platform Admin)

Platform admins manage global provider configs through `/v1/platform-management/settings/{embedding,llm}`:

- Create, update, and delete global configs
- Promote a non-default config to default
- Test connectivity for any global config
- Control per-config project availability via `is_available_to_projects`

## Query Pipeline

The backend-api serves six MCP tool endpoints. The search path for vector-based queries:

1. Convert natural language query to embedding vector via the project's resolved embedding provider
2. Retrieve top-k chunks from Qdrant (project-scoped collection)
3. Enrich results with symbol/file metadata from PostgreSQL
4. Load raw code from `code_chunks` table for top results
5. Deduplicate and rank
6. Return compact structured payload with line/file references

The six MCP-facing query endpoints:

| Endpoint | Purpose |
|---|---|
| `POST /query/search` | Semantic code search (vector retrieval) |
| `GET /symbols`, `GET /symbols/{id}` | Symbol lookup and detail |
| `GET /dependencies` | Dependency graph for a file or symbol |
| `GET /structure` | Project file/directory tree |
| `GET /files/context` | File-level context (imports, exports, symbols) |
| `GET /conventions` | Project coding conventions and patterns |

## Job Orchestration

The backend-api is responsible for creating jobs and enqueuing them. It does not execute pipeline stages.

### Job Creation Flow

1. Handler validates request, resolves project and caller identity
2. Creates an `indexing_jobs` row in PostgreSQL with status `queued`
3. Maps job type (`full` / `incremental`) to workflow name (`full-index` / `incremental-index`)
4. Enqueues an asynq task to Redis with the job ID, workflow, and project ID
5. Publishes a `job:queued` SSE event to connected backoffice clients

### Queue Configuration

- Redis-backed asynq queues
- Per-workflow timeouts: 30m for full index, 10m for incremental
- Max 3 retries for indexing jobs
- Deduplication: uniqueness window prevents duplicate active jobs per project+type

## API Surfaces

### Public Routes

- `POST /v1/users` -- register user
- `POST /v1/auth/login` -- start session

### Authenticated User Routes

- `POST /v1/auth/logout`
- `GET /v1/users/me`, `PATCH /v1/users/me`
- `GET /v1/users/me/projects`
- `GET /v1/users/lookup?q={query}`

### SSH Keys (User Library)

- `POST /v1/ssh-keys` -- create key pair
- `GET /v1/ssh-keys` -- list caller's keys
- `GET /v1/ssh-keys/{id}` -- get public key + fingerprint
- `GET /v1/ssh-keys/{id}/projects` -- list projects assigned to key
- `POST /v1/ssh-keys/{id}/retire` -- mark key inactive

### Personal API Keys

- `POST /v1/users/me/keys`
- `GET /v1/users/me/keys`
- `DELETE /v1/users/me/keys/{keyID}`

### Providers

- `GET /v1/settings/providers` -- supported provider catalog

### Platform Management (Platform Admin)

Global embedding settings:
- `GET /v1/platform-management/settings/embedding` -- list global configs
- `PUT /v1/platform-management/settings/embedding` -- update default config
- `POST /v1/platform-management/settings/embedding` -- create additional config
- `POST /v1/platform-management/settings/embedding/test` -- test default connectivity
- `PATCH /v1/platform-management/settings/embedding/{configId}` -- partial update
- `DELETE /v1/platform-management/settings/embedding/{configId}` -- deactivate config
- `POST /v1/platform-management/settings/embedding/{configId}/promote` -- promote to default
- `POST /v1/platform-management/settings/embedding/{configId}/test` -- test specific config

Global LLM settings:
- Same structure under `/v1/platform-management/settings/llm`

User management:
- `GET /v1/platform-management/users` -- list users
- `GET /v1/platform-management/users/{userId}` -- get user
- `PATCH /v1/platform-management/users/{userId}` -- update user
- `POST /v1/platform-management/users/{userId}/deactivate`
- `POST /v1/platform-management/users/{userId}/activate`

Platform roles:
- `GET /v1/platform-management/platform-roles`
- `POST /v1/platform-management/platform-roles`
- `DELETE /v1/platform-management/platform-roles/{userId}/{role}`

Worker status:
- `GET /v1/platform-management/workers` -- list active workers (read-only)

### Dashboard

- `GET /v1/dashboard/summary`

### Real-Time Events (SSE)

- `GET /v1/events/stream` -- SSE stream scoped to caller's project memberships

The SSE hub manages concurrent client connections with project-scoped broadcast:

- Clients register with their user ID and current project membership set
- Events are filtered: a client only receives events for projects they belong to
- Membership changes are propagated to live connections (add/remove project)
- Periodic membership refresh reconciles drift from missed Redis pub/sub deltas
- Hub enforces a configurable max-connections limit

### Project-Scoped Routes (Dual-Auth)

These routes accept both user sessions and API keys. Access level is determined by the project role.

Member-level (read/query):
- `GET /v1/projects/{id}` -- project detail
- `GET /v1/projects/{id}/ssh-key` -- assigned public key
- `GET /v1/projects/{id}/members`
- `GET /v1/projects/{id}/jobs` -- indexing job history
- `POST /v1/projects/{id}/query/search`
- `GET /v1/projects/{id}/symbols`
- `GET /v1/projects/{id}/symbols/{symbolID}`
- `GET /v1/projects/{id}/dependencies`
- `GET /v1/projects/{id}/dependencies/graph`
- `GET /v1/projects/{id}/structure`
- `GET /v1/projects/{id}/files/context`
- `GET /v1/projects/{id}/files/dependencies`
- `GET /v1/projects/{id}/files/exports`
- `GET /v1/projects/{id}/files/references`
- `GET /v1/projects/{id}/files/jsx-usages`
- `GET /v1/projects/{id}/files/network-calls`
- `GET /v1/projects/{id}/files/history`
- `GET /v1/projects/{id}/conventions`
- `GET /v1/projects/{id}/commits`
- `GET /v1/projects/{id}/commits/{commitHash}`
- `GET /v1/projects/{id}/commits/{commitHash}/diffs`
- `GET /v1/projects/{id}/settings/embedding` (read)
- `GET /v1/projects/{id}/settings/embedding/available`
- `GET /v1/projects/{id}/settings/embedding/resolved`
- `GET /v1/projects/{id}/settings/llm` (read)
- `GET /v1/projects/{id}/settings/llm/available`
- `GET /v1/projects/{id}/settings/llm/resolved`

Admin-level (dual-auth):
- `POST /v1/projects/{id}/index` -- trigger indexing

Admin-level (user-only):
- `PATCH /v1/projects/{id}` -- update project
- `PUT /v1/projects/{id}/ssh-key` -- reassign SSH key
- `DELETE /v1/projects/{id}/ssh-key` -- remove SSH key
- `POST /v1/projects/{id}/members` -- add member
- `PATCH /v1/projects/{id}/members/{userID}` -- change role
- `PUT /v1/projects/{id}/settings/embedding` -- update embedding setting
- `DELETE /v1/projects/{id}/settings/embedding` -- reset to default
- `POST /v1/projects/{id}/settings/embedding/test`
- `PUT /v1/projects/{id}/settings/llm` -- update LLM setting
- `DELETE /v1/projects/{id}/settings/llm` -- reset to default
- `POST /v1/projects/{id}/settings/llm/test`
- `GET /v1/projects/{id}/keys` -- list project API keys
- `POST /v1/projects/{id}/keys` -- create project API key
- `DELETE /v1/projects/{id}/keys/{keyID}` -- revoke project API key

Member-level (user-only):
- `DELETE /v1/projects/{id}/members/{userID}` -- remove member (allows self-removal)

Owner-level (user-only):
- `DELETE /v1/projects/{id}` -- delete project

### Health and Metrics

- `GET /health/live` -- liveness probe
- `GET /health/ready` -- readiness probe (checks postgres, qdrant, redis)
- `GET /metrics` -- Prometheus metrics

## Configuration

All configuration is loaded from environment variables at startup. Required variables cause a panic if missing.

### Core Settings

| Variable | Required | Description |
|---|---|---|
| `SERVER_PORT` | no | HTTP listen port (default: 8080) |
| `POSTGRES_DSN` | yes | PostgreSQL connection string |
| `REDIS_URL` | no | Redis connection URL for asynq and pub/sub |
| `QDRANT_URL` | no | Qdrant vector database URL |

### Security

| Variable | Required | Description |
|---|---|---|
| `SSH_KEY_ENCRYPTION_SECRET` | yes | AES key for stored SSH private keys |
| `PROVIDER_ENCRYPTION_SECRET` | yes | AES key for stored provider credentials |
| `SESSION_TTL` | no | Session token lifetime (default: 24h) |
| `SESSION_COOKIE_NAME` | no | Cookie name for sessions (default: session) |
| `SESSION_SECURE_COOKIE` | no | Require HTTPS for cookies (default: false) |

### Provider Defaults

| Variable | Required | Description |
|---|---|---|
| `EMBEDDING_PROVIDER` | no | Default embedding provider (default: ollama) |
| `OLLAMA_URL` | no | Default embedding endpoint URL |
| `OLLAMA_MODEL` | no | Default embedding model |
| `OLLAMA_DIMENSIONS` | no | Default embedding vector dimensions |
| `EMBED_BATCH_SIZE` | no | Embedding batch size (default: 64) |
| `LLM_PROVIDER` | no | Default LLM provider (default: ollama) |
| `LLM_URL` | no | Default LLM endpoint URL |
| `LLM_MODEL` | no | Default LLM model |
| `PROVIDER_CONNECTIVITY_ALLOWED_HOSTS` | no | Comma-separated hosts allowed for connectivity tests |

### CORS and Logging

| Variable | Required | Description |
|---|---|---|
| `CORS_ALLOWED_ORIGINS` | no | Comma-separated allowed origins |
| `CORS_WILDCARD` | no | Allow all origins (default: false) |
| `LOG_LEVEL` | no | debug, info, warn, error (default: info) |
| `LOG_FORMAT` | no | json or text (default: json) |

### Indexing and Jobs

| Variable | Required | Description |
|---|---|---|
| `REPO_CACHE_DIR` | no | Local git checkout/cache directory |
| `INDEX_FULL_TIMEOUT` | no | Full index job timeout (default: 30m) |
| `INDEX_INCREMENTAL_TIMEOUT` | no | Incremental index timeout (default: 10m) |
| `MAX_RETRIES` | no | Job retry limit (default: 3) |
| `MAX_SSE_CONNECTIONS` | no | SSE connection limit (default: 100) |

## Docker Notes

From `docker-compose.yaml`:

```yaml
backend-api:
  image: myjungle/backend-api:local
  container_name: myjungle-backend-api
  command: ["--migrate"]        # runs DB migrations on startup
  ports:
    - "8080:8080"
  depends_on:
    postgres:  { condition: service_healthy }
    qdrant:    { condition: service_healthy }
    redis:     { condition: service_healthy }
  volumes:
    - repo_cache:/var/lib/myjungle/repos
  healthcheck:
    test: ["CMD-SHELL", "wget -q -O /dev/null http://localhost:8080/health/ready || exit 1"]
    interval: 10s
    timeout: 5s
    retries: 15
```

Key points:

- Port 8080 is the only exposed port
- `--migrate` flag runs `golang-migrate` migrations before serving
- Depends on postgres, qdrant, and redis (all must be healthy)
- Shared `repo_cache` volume with `backend-worker` for cloned repositories
- `extra_hosts` maps `host.docker.internal` for reaching host-local Ollama
- Readiness check (`/health/ready`) gates downstream services (mcp-server, backoffice)

## Go Tooling

- Router: `chi`
- DB driver: `pgx/v5`
- Query layer: `sqlc`
- Migrations: `golang-migrate`
- Config: environment variables (no YAML config files)
- Job queue: `asynq` (Redis-backed)
- SSH key generation: `crypto/ed25519` + `golang.org/x/crypto/ssh`
- Encryption: `crypto/aes` (AES-256-GCM) for SSH keys and provider credentials
- Observability: structured logging (`log/slog`), Prometheus metrics

## Security Notes

- Never expose private keys through API responses
- SSH keys and provider credentials are encrypted at rest using AES-256-GCM
- Session tokens and API keys are SHA-256 hashed before storage; raw values are never persisted
- Never log full Bearer tokens or API keys
- In production, serve over HTTPS and set `SESSION_SECURE_COOKIE=true`
- Enforce project-scope authorization on every query endpoint
- Connectivity test endpoints are protected by an endpoint allowlist to prevent SSRF

## Non-Goals

- MCP write operations (code modifications)
- Cross-project global semantic search (per-project only)
- Password-based auth stack (password reset, MFA, enterprise SSO)
