# 01 — Foundation (Server, Config, DB, Logging, Health, Errors)

## Status
Done

## Goal
Built the foundational infrastructure for the backend-api service: HTTP server with Chi router and middleware chain, environment-based configuration, PostgreSQL connection pool with migrations and sqlc query layer, structured logging via log/slog, health/readiness endpoints with Prometheus metrics, a consistent error envelope, and the integration test infrastructure.

## Depends On
--

## Scope

### HTTP Server and Router

Restructured the project from a single-file skeleton into a proper Go project with the `internal/` package layout. The `App` struct in `internal/app/app.go` holds all dependencies (config, DB, Redis, handlers) and is constructed via `app.New(cfg)` which returns `(*App, error)`. The slim entrypoint `cmd/api/main.go` (~33 lines) loads config, optionally runs migrations via a `--migrate` flag, builds the app, and starts the server with graceful shutdown on SIGINT/SIGTERM.

Route registration lives in `internal/app/routes.go` using Chi subrouters. The middleware chain is: requestid, logging, metrics, recover, CORS, bodylimit, then route handlers. Custom JSON-returning 404 and 405 (with `Allow` header per RFC 7231) handlers replace Chi's plain-text defaults.

### Environment-Based Configuration

`internal/config/config.go` provides a typed `Config` struct loaded from environment variables via `config.Load()`. Defaults are defined as constants in `defaults.go`. Required variables (`POSTGRES_DSN`, `SSH_KEY_ENCRYPTION_SECRET`, `PROVIDER_ENCRYPTION_SECRET`) cause a panic with a descriptive message if missing. Validation covers port ranges, timeout positivity, pool sizes, and URL format. A `logSummary()` function emits a single structured log event with all config values (secrets redacted). A `LoadForTest()` helper provides sensible test defaults with no env reads and no validation.

Key config sections: `ServerConfig` (port, timeouts, CORS, body limit), `PostgresConfig` (DSN, pool sizes), `RedisConfig`, `SSHConfig`, `EmbeddingDefaults`, `LLMDefaults`, `IndexingConfig`, `JobsConfig`, `EventsConfig`, `SessionConfig`.

### PostgreSQL Connection and Migrations

`internal/storage/postgres/postgres.go` defines a `DB` struct wrapping `*pgxpool.Pool` and `*db.Queries` (sqlc-generated). `New(ctx, cfg)` parses the DSN, applies pool settings, pings with a 5-second timeout, and logs pool stats on success. A `WithTx(ctx, fn)` helper in `tx.go` provides transactional execution with automatic rollback on error.

Migrations use `golang-migrate` with SQL files embedded via `go:embed` in the `datastore` module (`datastore/postgres/migrate.go`). The backend-api has a thin wrapper at `internal/storage/postgres/migrate.go` that delegates to it. A `go.work` file at the repo root links `backend-api`, `backend-worker`, and `datastore` into a Go workspace, making sqlc-generated code in `datastore/postgres/sqlc/` importable by both services.

### Structured Logging

`internal/logger/logger.go` creates a configured `*slog.Logger` based on `LOG_LEVEL` (debug/info/warn/error, default: info) and `LOG_FORMAT` (json/text, default: json), then sets it as the slog default. JSON mode produces one object per line; text mode is human-readable for development. The logger is bootstrapped early in `main()` using raw env vars so that config loading itself benefits from structured output.

All `log.Printf` calls across production code were replaced with `slog.ErrorContext`, `slog.WarnContext`, or `slog.Info` as appropriate. Request logging middleware emits structured fields (request_id, method, path, status, duration, error_code) with level based on status code (Info for 2xx/3xx, Warn for 4xx, Error for 5xx). Panic recovery logs stack traces as a structured field.

### Health Checks and Metrics

`GET /health/live` returns 200 unconditionally with `status`, `service`, `version`, and `timestamp` fields. `GET /health/ready` runs dependency checks (Postgres required, others informational) with a 2-second per-check timeout and returns 200 or 503 with per-dependency status, latency, and error details. Both endpoints are outside auth middleware.

`GET /metrics` returns Prometheus-compatible text format with uptime gauge and request counter (method, path, status labels). The `internal/metrics/` package collects request metrics via middleware; `internal/health/` defines a `Checker` interface for pluggable dependency checks.

### Error Handling and Validation

`internal/domain/errors.go` defines `AppError` with Status, Code, Message, and Details fields. Predefined errors: `ErrNotFound`, `ErrConflict`, `ErrForbidden`, `ErrUnauthorized`, `ErrBadRequest`, `ErrInternal`. Constructor functions (`NotFound()`, `Conflict()`, `BadRequest()`, `ValidationError()`) create specific variants. `WriteAppError` in `handler/respond.go` serializes any error into the `{error, code, details}` JSON envelope.

`internal/validate/validate.go` provides reusable validators: `Required`, `UUID`, `MinMax`, `OneOf`, `URL`, `MaxLength`, `Email`, `RepoURL`, plus a batch `Errors` type with `Add()`, `HasErrors()`, and `ToAppError()` for aggregating per-field validation failures.

`internal/middleware/bodylimit.go` wraps `http.MaxBytesReader` to reject oversized request bodies (configurable via `SERVER_BODY_LIMIT_BYTES`, default 1MB) with 413 status. `internal/middleware/cors.go` supports configurable origins via `CORS_ALLOWED_ORIGINS` and `CORS_WILDCARD` env vars.

### Integration Test Infrastructure

`tests/integration/setup_test.go` uses testcontainers-go to spin up a `postgres:16-alpine` container per test suite. `TestMain` manages container lifecycle. Shared helpers: `setupTestApp` (creates App with test config and runs migrations), `truncateAll` (cleans tables between tests), `doRequest`, `decodeJSON`, `registerUser`, `loginUser`, `authHeader`. A `TEST_POSTGRES_DSN` env var allows CI to override with a pre-existing database.

223+ integration tests across `health_test.go`, `user_test.go`, `project_test.go`, `sshkey_test.go`, `membership_test.go`, `apikey_test.go`, `apikey_auth_test.go`, `embedding_test.go`. Dockerfiles rewritten for Go workspace multi-module build (repo root context). Root `Makefile` with `test-unit`, `test-integration`, `build`, `lint`, `sqlc-generate` targets.

## Key Files

| File/Package | Purpose |
|---|---|
| `cmd/api/main.go` | Slim entrypoint: config, migrations, app, server |
| `internal/app/app.go` | App struct, dependency construction, graceful shutdown |
| `internal/app/routes.go` | Chi router tree with all route registration |
| `internal/config/config.go` | Typed Config struct, env loading, validation |
| `internal/config/defaults.go` | Default values as constants |
| `internal/storage/postgres/postgres.go` | DB struct, pgxpool setup, Ping |
| `internal/storage/postgres/tx.go` | WithTx transaction helper |
| `internal/storage/postgres/migrate.go` | Migration runner wrapper |
| `internal/logger/logger.go` | slog factory, level/format config |
| `internal/health/checker.go` | Checker interface, PostgresChecker |
| `internal/handler/health.go` | Live, ready, and metrics handlers |
| `internal/handler/respond.go` | WriteJSON, WriteAppError helpers |
| `internal/domain/errors.go` | AppError type and predefined errors |
| `internal/validate/validate.go` | Input validation helpers |
| `internal/middleware/` | requestid, logging, metrics, recover, cors, bodylimit |
| `internal/metrics/` | Prometheus request metric collectors |
| `tests/integration/setup_test.go` | testcontainers setup, shared test helpers |
| `Makefile` | Root build/test/lint targets |
| `go.work` | Go workspace linking backend-api, backend-worker, datastore |

## Acceptance Criteria
- [x] `go build ./cmd/api` succeeds
- [x] `go vet ./...` passes
- [x] Server starts on configured port with full middleware chain
- [x] `config.Load()` reads all env vars with correct defaults and panics on missing required vars
- [x] Config summary logged at startup with secrets redacted
- [x] pgxpool connects to Postgres, respects pool config, supports Ping for health checks
- [x] sqlc Queries accessible through DB struct via Go workspace
- [x] Transaction helper commits on success, rolls back on error
- [x] `--migrate` flag runs golang-migrate migrations before serving
- [x] `GET /health/live` returns 200 with version info
- [x] `GET /health/ready` returns 200/503 based on dependency checks
- [x] `GET /metrics` returns valid Prometheus text format
- [x] Health/metrics endpoints require no authentication
- [x] All error responses use consistent `{error, code, details}` envelope
- [x] Validation errors include per-field messages in details
- [x] Panics caught by recovery middleware return 500 JSON (no server crash)
- [x] Every request has X-Request-ID header
- [x] Request bodies over limit rejected with 413
- [x] Unknown routes return 404 JSON; wrong methods return 405 with Allow header
- [x] `LOG_LEVEL` and `LOG_FORMAT` env vars control structured logging output
- [x] JSON log mode produces one object per line; text mode is human-readable
- [x] No imports of standard `"log"` package remain in production code
- [x] All 223+ integration tests pass against real Postgres via testcontainers
- [x] Docker build succeeds with Go workspace multi-module context
