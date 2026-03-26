# 16 — Dashboard & Health Metrics

## Status
Done

## Goal
Implemented the dashboard summary endpoint and extended the project list with per-project index health data. The summary provides aggregate platform metrics scoped to the authenticated user's projects. The integration test infrastructure (testcontainers with real PostgreSQL) was established as part of this work, reaching 223+ tests across all backend-api features.

## Depends On
- Task 07 (Project CRUD)
- Task 08 (Project Membership)
- Task 12 (Integration Tests — testcontainers infrastructure)

## Scope

### Dashboard Summary Endpoint

`GET /v1/dashboard/summary` returns aggregate statistics scoped to the authenticated user's projects:

| Field | Type | Description |
|-------|------|-------------|
| `projects_total` | int | Total projects the user is a member of |
| `jobs_active` | int | Count of queued + running indexing jobs across user's projects |
| `jobs_failed_24h` | int | Count of failed jobs in the last 24 hours |
| `query_count_24h` | int | Total queries logged in the last 24 hours |
| `p95_latency_ms_24h` | int | 95th percentile query latency (ms) over the last 24 hours |

All values are non-negative integers. When the user has zero projects, all values are 0. The p95 calculation uses PostgreSQL's `percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms)` for accurate percentile computation.

### SQL Queries

Three new queries in `datastore/postgres/queries/indexing.sql`:
- `CountActiveJobsForUser` — joins `indexing_jobs` with `project_members` to scope by user
- `CountFailedJobsForUser24h` — same join, filtered to `status = 'failed'` and `finished_at >= NOW() - INTERVAL '24 hours'`
- `GetQueryStats24hForUser` — joins `query_log` with `project_members`, returns count and `percentile_cont` p95

### Extended Project List

`ListUserProjectsWithHealth` query extends the existing project list with per-project health fields via three `LEFT JOIN LATERAL` subqueries:
- **Active snapshot**: `index_git_commit`, `index_branch`, `index_activated_at` (from most recent active snapshot)
- **Active job**: `active_job_id`, `active_job_status` (most recent queued/running job)
- **Failed job**: `failed_job_id`, `failed_job_finished_at`, `failed_job_type` (most recent failure in last 24h)

All health fields are nullable; null indicates no data (never indexed, no active job, no recent failure). COALESCE wraps nullable text columns to avoid pgx NULL-to-string scan errors. ORDER BY clauses include `id DESC` tiebreakers for deterministic row selection.

### Handler Implementation

`DashboardHandler` was refactored from a zero-dependency stub to a real handler with `*postgres.DB` constructor injection. `HandleSummary` extracts the user from context, executes the four aggregate queries, and returns the JSON response. The `HandleJobs` stub was removed (job status is now embedded in the project list).

`HandleMyProjects` in the user handler was updated to use `ListUserProjectsWithHealth` and serialize the nullable health fields.

### Integration Test Infrastructure

The test suite uses `testcontainers-go` with `postgres:16-alpine` for real database testing. Key infrastructure in `tests/integration/setup_test.go`:
- `TestMain` manages container lifecycle
- `TEST_POSTGRES_DSN` environment variable for CI override
- Shared helpers: `setupTestApp`, `truncateAll`, `doRequest`, `decodeJSON`, `registerUser`, `loginUser`, `authHeader`
- 223+ integration tests across all backend-api features

## Key Files

| File | Description |
|------|-------------|
| `backend-api/internal/handler/dashboard.go` | `DashboardHandler` with `HandleSummary`; DB-backed implementation |
| `backend-api/internal/handler/user.go` | `HandleMyProjects` extended with health fields |
| `backend-api/internal/app/app.go` | `DashboardHandler` wired with `*postgres.DB` |
| `backend-api/internal/app/routes.go` | `/v1/dashboard/summary` route; `/v1/dashboard/jobs` removed |
| `datastore/postgres/queries/indexing.sql` | `CountActiveJobsForUser`, `CountFailedJobsForUser24h`, `GetQueryStats24hForUser` |
| `datastore/postgres/queries/auth.sql` | `ListUserProjectsWithHealth` |
| `backend-api/tests/integration/setup_test.go` | Test infrastructure: testcontainers, helpers, container lifecycle |
| `backend-api/tests/integration/dashboard_test.go` | 12 dashboard integration tests |

## Acceptance Criteria
- [x] `GET /v1/dashboard/summary` returns 200 with `projects_total`, `jobs_active`, `jobs_failed_24h`, `query_count_24h`, `p95_latency_ms_24h`
- [x] All values are non-negative integers; zero-project user gets all zeros
- [x] `p95_latency_ms_24h` uses `percentile_cont` for accurate percentile calculation
- [x] Dashboard is scoped to the authenticated user's projects (multi-user isolation)
- [x] Returns 401 without valid session
- [x] Extended project list includes nullable health fields: index snapshot, active job, failed job
- [x] Health fields are null when no data exists (never indexed, no active job, no recent failure)
- [x] Failed job health only shows failures within the last 24 hours
- [x] Lateral join ORDER BY clauses include deterministic `id DESC` tiebreakers
- [x] `DashboardHandler` receives `*postgres.DB` via constructor injection
- [x] `HandleJobs` stub removed; job status embedded in project list
- [x] Integration test infrastructure established with testcontainers-go (`postgres:16-alpine`)
- [x] 223+ integration tests pass across all backend-api features
- [x] Tests cover: zero-data case, single-project case, multi-project with mixed health states, stale failed jobs excluded
