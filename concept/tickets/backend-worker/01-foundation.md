# 01 — Worker Bootstrap, Config, Logging & Deployment

## Status
Done

## Goal
Replace the one-file idle worker skeleton with a structured application bootstrap that loads validated configuration from environment variables, wires all runtime dependencies, provides structured logging, and packages into a single-binary Docker container.

## Depends On
—

## Scope

### Application Bootstrap

`cmd/worker/main.go` was reduced to a thin entrypoint that calls `config.Load()` and delegates to `internal/app/` for lifecycle management. The `App` struct owns startup sequencing, dependency construction, and shutdown orchestration.

Startup order: load config, initialize logger, connect PostgreSQL pool, connect Redis, construct parser pool, construct embedding client, construct Qdrant client, wire workflow handlers, start queue consumer.

Shutdown: honors `SIGINT` and `SIGTERM`, stops accepting new work, drains in-flight tasks with a configurable timeout, closes all network clients cleanly.

### Configuration (`internal/config/config.go`)

All settings are loaded from environment variables at startup. Required variables cause a panic with a descriptive message if missing. Secrets are redacted in log output.

**Required variables:** `POSTGRES_DSN`, `REDIS_URL`, `SSH_KEY_ENCRYPTION_SECRET`.

**Optional variables with defaults:**

| Variable | Default | Description |
|---|---|---|
| `LOG_LEVEL` | `info` | Minimum log level |
| `LOG_FORMAT` | `json` | Output format: `json` or `text` |
| `POSTGRES_MAX_CONNS` | `10` | Maximum PostgreSQL connections |
| `POSTGRES_MIN_CONNS` | `2` | Minimum PostgreSQL connections |
| `POSTGRES_MAX_CONN_LIFE` | `30m` | Maximum connection lifetime |
| `QUEUE_NAME` | `default` | Asynq queue name |
| `WORKER_CONCURRENCY` | `4` | Concurrent workflow handlers per worker |
| `QUEUE_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `PARSER_POOL_SIZE` | `0` (= `runtime.NumCPU()`) | Tree-sitter parser pool size |
| `PARSER_TIMEOUT` | `5m` | Batch parse timeout |
| `PARSER_TIMEOUT_PER_FILE` | `30s` | Per-file parse timeout |
| `PARSER_MAX_FILE_SIZE` | `10485760` (10 MB) | Max file size in bytes |
| `OLLAMA_URL` | `http://host.docker.internal:11434` | Embedding provider URL |
| `OLLAMA_MODEL` | `jina/jina-embeddings-v2-base-en` | Embedding model |
| `QDRANT_URL` | `http://qdrant:6333` | Qdrant URL |
| `REPO_CACHE_DIR` | `/var/lib/myjungle/repos` | Repository cache directory |
| `WORKER_ID` | `os.Hostname()` | Worker identifier for registry |

Config struct hierarchy: `Config` -> `LogConfig`, `PostgresConfig`, `RedisConfig`, `QueueConfig`, `ParserConfig`, `EmbeddingConfig`, `QdrantConfig`, `SSHConfig`, `WorkspaceConfig`.

Validation panics on: invalid log level/format, non-positive pool sizes, missing URL schemes, min > max connection counts.

### Structured Logging (`internal/logger/`)

Uses Go 1.21+ `log/slog` with JSON or text handler based on `LOG_FORMAT`. The log level is parsed from `LOG_LEVEL`. All worker logs use structured fields (job_id, project_id, workflow, etc.) for filtering.

DSN passwords are redacted via regex-based replacement (both URL-style and key-value style). Redis URLs have userinfo stripped. The `SSH_KEY_ENCRYPTION_SECRET` is always logged as `***`.

### Dockerfile

Single-stage build producing a Go binary. Uses `CGO_ENABLED=1` for go-tree-sitter C bindings. Build stage installs `gcc` and `musl-dev`. Final image is Alpine-based with the worker binary, `git`, `openssh-client`, and `ssh-keyscan` for repository operations.

No separate process or entrypoint script needed -- the container runs the single Go binary directly.

### Docker Compose Wiring

```yaml
backend-worker:
  build:
    context: .
    dockerfile: backend-worker/Dockerfile
  profiles: ["app"]
  depends_on:
    postgres: { condition: service_healthy }
    qdrant:   { condition: service_healthy }
    redis:    { condition: service_healthy }
  volumes:
    - repo_cache:/var/lib/myjungle/repos
```

No public ports. Shared `repo_cache` volume for workspace persistence across restarts. Host gateway mapped for Ollama access.

### Makefile

Build targets: `build` (CGO_ENABLED=1 go build), `test` (go test ./...), `lint`, `docker-build`.

## Key Files

| File/Package | Purpose |
|---|---|
| `cmd/worker/main.go` | Thin entrypoint: config load + app startup |
| `internal/app/app.go` | Bootstrap, lifecycle wiring, shutdown |
| `internal/config/config.go` | Env parsing, defaults, validation, redaction |
| `internal/logger/` | Structured slog setup |
| `Dockerfile` | Single Go binary container build |
| `docker-compose.yaml` | Service wiring with health checks |

## Acceptance Criteria
- [x] `cmd/worker/main.go` is only the thin process entrypoint
- [x] Worker config is loaded from env with clear validation errors on missing required vars
- [x] Required secrets are redacted from log output (DSN passwords, SSH secret)
- [x] PostgreSQL pool and Redis client are wired behind worker-local constructors
- [x] Worker starts and shuts down cleanly on `SIGINT` or `SIGTERM`
- [x] Worker compiles even before real workflows are implemented
- [x] Dockerfile produces a single Go binary container with CGO and git tools
- [x] Docker Compose health-check dependencies prevent premature startup
- [x] Unit tests cover config parsing, validation edge cases, and bootstrap failure paths
- [x] `LoadForTest()` provides sensible defaults for test suites without env vars
