# 15 — Platform Administration

## Status
Done

## Goal
Introduced the `platform_admin` role as a system-level authorization layer with user management, role management, seed admin bootstrapping, and worker status monitoring. This provides the foundation for all platform-level operations: managing users, granting/revoking platform roles, and observing worker health via Redis.

## Depends On
- Task 04 (User identity and session middleware — `IdentityResolver`, `RequireUser`)
- Task 08 (Project membership — `RequireProjectRole` updated with platform admin short-circuit)
- Redis infrastructure (used by SSE subscriber and queue publisher)

## Scope

### Platform Admin Role & Middleware

A `user_platform_roles` table (in 000001_init migration) stores platform-level role assignments with a `(user_id, role)` unique constraint. Currently only `platform_admin` is supported.

`RequirePlatformAdmin` middleware queries `HasPlatformRole` and stores the result in context via `auth.ContextWithPlatformRoles`. Returns 401 for unauthenticated requests, 403 for non-admin users.

`RequireProjectRole` was updated with a platform admin short-circuit: platform admins receive synthetic owner-level access to any project without requiring a `project_members` row. Falls back to normal membership check if the platform role query fails.

### User Management Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/platform-management/users` | Paginated user list with `search`, `is_active`, `sort` params |
| GET | `/v1/platform-management/users/{userId}` | User detail with memberships and platform roles |
| PATCH | `/v1/platform-management/users/{userId}` | Update display_name, avatar_url |
| POST | `/v1/platform-management/users/{userId}/deactivate` | Soft-disable with session invalidation |
| POST | `/v1/platform-management/users/{userId}/activate` | Re-enable deactivated user |

Deactivation uses a transactional last-admin guard: `CountActivePlatformAdminsForUpdate` acquires `FOR UPDATE` locks on both `user_platform_roles` and `users` rows to serialize concurrent attempts, preventing TOCTOU races that could deactivate the last platform admin. Self-deactivation is blocked (400).

### Role Management Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/platform-management/platform-roles` | List all active role assignments with user info |
| POST | `/v1/platform-management/platform-roles` | Grant role (idempotent via `ON CONFLICT DO NOTHING`) |
| DELETE | `/v1/platform-management/platform-roles/{userId}/{role}` | Revoke role with last-admin guard |

Revocation follows a lock-first-then-delete pattern inside a transaction to avoid deadlocks. Self-revocation of `platform_admin` is blocked (400). Revoking the last `platform_admin` returns 409.

### Seed Admin on Startup

Migration `000002_seed_admin_account` creates a built-in `admin` user (`admin@local.local`) and grants `platform_admin`, making the platform manageable immediately after deployment. The `admin` username is reserved and rejected during normal registration.

Additional admins can be promoted at startup via `PLATFORM_ADMIN_USERNAMES` environment variable (comma-separated usernames). The Go bootstrap calls `UpsertPlatformRoleByUsername` for each, is idempotent, and logs warnings for unresolved usernames.

### Worker Status via Redis

`GET /v1/platform-management/workers` reads ephemeral worker heartbeat keys (`worker:status:*`) from Redis using SCAN + MGET. Workers publish JSON status with a 30s TTL, refreshed every 10s. The endpoint returns all live workers sorted by `worker_id`, gracefully skipping malformed entries.

A dedicated `redisclient.Reader` package provides a reusable Redis client for key reads, separate from SSE and queue concerns. The endpoint is conditionally registered (not registered when `REDIS_URL` is not configured). Redis failures return 502, not 500.

### Domain Types

`domain.WorkerStatus` mirrors the contract schema: `worker_id`, `status`, `started_at`, `last_heartbeat_at`, `supported_workflows`, `hostname`, `version`, `current_job_id`, `current_project_id`, `drain_reason`.

`domain.PlatformRoleAdmin` constant and `PlatformRoleKnown` validator in `domain/models.go`.

## Key Files

| File | Description |
|------|-------------|
| `backend-api/internal/middleware/platformadmin.go` | `RequirePlatformAdmin` middleware |
| `backend-api/internal/middleware/projectrole.go` | Platform admin short-circuit in `RequireProjectRole` |
| `backend-api/internal/handler/admin.go` | `AdminHandler` — user management and role management endpoints |
| `backend-api/internal/handler/worker.go` | `WorkerHandler` — `HandleListWorkers` via Redis |
| `backend-api/internal/redisclient/reader.go` | Redis reader with `ScanKeys`, `MGetJSON`, `Ping`, `Close` |
| `backend-api/internal/auth/session.go` | Platform role context helpers (`ContextWithPlatformRoles`, `IsPlatformAdmin`) |
| `backend-api/internal/domain/models.go` | `PlatformRoleAdmin` constant |
| `backend-api/internal/domain/worker.go` | `WorkerStatus` type |
| `backend-api/internal/config/config.go` | `PlatformAdminUsernames` config, `parsePlatformAdminUsernames` |
| `backend-api/internal/validate/validate.go` | `ReservedUsername` validator |
| `datastore/postgres/migrations/000002_seed_admin_account.up.sql` | Seed admin user and platform_admin role |
| `datastore/postgres/queries/platform_roles.sql` | `HasPlatformRole`, `GrantPlatformRole`, `RevokePlatformRole`, `CountActivePlatformAdminsForUpdate`, `UpsertPlatformRoleByUsername` |
| `datastore/postgres/queries/admin_users.sql` | `AdminListUsers`, `AdminGetUserByID`, `AdminUpdateUser`, `ActivateUser` |

## Acceptance Criteria
- [x] `user_platform_roles` table created in 000001_init with constraints, indexes, and FK cascade
- [x] `RequirePlatformAdmin` middleware returns 401/403 as appropriate and stores roles in context
- [x] `RequireProjectRole` short-circuits to owner-level access for platform admins
- [x] Platform admin can access any project endpoint without a `project_members` row
- [x] `GET /users` returns paginated list with `project_count` and `platform_roles`; supports `search`, `is_active`, `sort`
- [x] `GET /users/{userId}` returns user detail with memberships and platform roles
- [x] `PATCH /users/{userId}` updates display_name and/or avatar_url; validates at least one field
- [x] Deactivation soft-disables user, invalidates sessions; cannot deactivate self (400) or last admin (409)
- [x] Activation re-enables deactivated user; already-active returns 400
- [x] Role grant is idempotent; role revoke uses lock-first pattern with last-admin guard (409)
- [x] Cannot revoke own `platform_admin` role (400)
- [x] Seed migration creates `admin` user with `platform_admin` role; idempotent
- [x] `admin` username reserved during registration (422)
- [x] `PLATFORM_ADMIN_USERNAMES` env var promotes additional users at startup; idempotent
- [x] `GET /workers` returns active worker statuses from Redis; empty array when no workers online
- [x] SCAN used instead of KEYS; malformed entries skipped; Redis failure returns 502
- [x] Worker endpoint not registered when `REDIS_URL` is not configured
- [x] All admin endpoints require `RequireUser` + `RequirePlatformAdmin` middleware
- [x] All admin endpoints use the consistent error envelope `{error, code, details}`
