# 02 — Session-Based and API Key Authentication

## Status
Done

## Goal
Implemented the dual authentication system: session-based auth for backoffice users (login/logout, Bearer tokens, cookies) and API key auth for MCP/programmatic access (project-scoped and personal keys). Built the identity resolution middleware that unifies both auth paths, the requireuser middleware that blocks API-key-only access on user routes, and the project role middleware that enforces RBAC for both identity types.

## Depends On
01-foundation

## Scope

### Session Authentication

`POST /v1/auth/login` accepts a username, creates a server-side session, stores a SHA-256 hash of the token in the database, and returns the raw token once. `POST /v1/auth/logout` invalidates the session. Session TTL, cookie name, and secure-cookie flag are configurable via `SESSION_TTL`, `SESSION_COOKIE_NAME`, and `SESSION_SECURE_COOKIE` env vars.

Auth context helpers in `internal/auth/session.go` provide `UserFromContext`/`ContextWithUser` and `MembershipFromContext`/`ContextWithMembership` using unexported context key types. Guards against typed-nil values.

### API Key Authentication

API keys use the `mj_` prefix for identification. Two types exist:
- **Project keys** (`mj_proj_*`): scoped to a single project with a fixed role (`read` or `write`). Managed by project admins/owners via `POST /v1/projects/{id}/keys`, `GET /v1/projects/{id}/keys`, `DELETE /v1/projects/{id}/keys/{keyID}`.
- **Personal keys** (`mj_pers_*`): scoped to all projects the user is a member of. Effective role is `MIN(key_role, mapped_membership_role)`. Managed via `POST /v1/users/me/keys`, `GET /v1/users/me/keys`, `DELETE /v1/users/me/keys/{keyID}`.

Key generation in `internal/apikey/keygen.go` produces 24 bytes of `crypto/rand`, base62-encoded to 32 chars, prefixed with key type. SHA-256 hash stored in DB; plaintext returned only on creation. Key prefix (first 12 chars) stored for display identification.

`internal/apikey/service.go` provides `CreateProjectKey`, `ListProjectKeys`, `DeleteProjectKey`, `CreatePersonalKey`, `ListPersonalKeys`, `DeletePersonalKey`. The `api_key_project_access` database view handles access checks and role computation for both key types uniformly.

### Identity Resolution Middleware

`internal/middleware/identity.go` runs on all `/v1` routes. Resolution priority:
1. `Authorization: Bearer <mj_*>` -- resolve as API key identity via `api_keys` table
2. `Authorization: Bearer <token>` -- resolve as session (SHA-256 hash + DB lookup)
3. Session cookie -- resolve as session
4. None matched -- continue as anonymous

Bearer present but invalid/expired returns hard 401 (no fallthrough). DB errors return 500. API key `last_used_at` is updated in a fire-and-forget goroutine on each authenticated request.

### RequireUser Middleware

`internal/middleware/requireuser.go` returns 401 if no user is in context. Used on routes that should only be accessible to session-authenticated users (not API-key-only), such as user profile, project management, and SSH key operations.

### Project Role Middleware

`internal/middleware/projectrole.go` enforces RBAC for project-scoped routes. Accepts both session users and API key identities. Extracts `{projectID}` from URL params, looks up membership via `GetProjectMember` query. No membership returns 404 (hides project existence). Insufficient role returns 403. Stores the resolved `ProjectMember` in context for handler use. Validates `minRole` at construction time (panics on invalid role for fail-fast).

Role hierarchy: `owner (3) > admin (2) > member (1)`. Domain model in `internal/domain/models.go` defines `RoleRank()`, `RoleKnown()`, and `RoleSufficient()` with fail-closed semantics.

### Platform Admin Middleware

`internal/middleware/platformadmin.go` gates `/v1/platform-management` routes. Resolves platform admin status from the `user_platform_roles` table. Only session-authenticated users with the `platform_admin` role can access global settings management.

### DB-Domain Conversion

`internal/dbconv/` provides converters: `DBUserToDomain`, `DBMemberToDomain`, `DBAPIKeyToDomain`, `PgUUIDToString`/`StringToPgUUID`, `PgTimestamptzToTime`.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/auth/session.go` | Context helpers for user and membership storage/retrieval |
| `internal/middleware/identity.go` | Unified identity resolution (session or API key) |
| `internal/middleware/requireuser.go` | Blocks anonymous and API-key-only requests |
| `internal/middleware/projectrole.go` | Project-scoped RBAC enforcement |
| `internal/middleware/platformadmin.go` | Platform admin gate |
| `internal/handler/auth.go` | Login/logout handlers |
| `internal/apikey/keygen.go` | API key generation and hashing |
| `internal/apikey/service.go` | API key CRUD for both key types |
| `internal/handler/apikey.go` | 6 handlers for project + personal API key endpoints |
| `internal/domain/models.go` | User, ProjectMember, APIKeyInfo, role constants |
| `internal/dbconv/dbconv.go` | DB row to domain model converters |
| `internal/app/routes.go` | Route groups with auth middleware stacks |
| `tests/integration/apikey_test.go` | API key lifecycle integration tests |
| `tests/integration/apikey_auth_test.go` | API key auth and access check tests |

## Acceptance Criteria
- [x] Bearer token and session cookie resolve to user from database
- [x] No auth credentials results in anonymous request (no error at middleware level)
- [x] RequireUser returns 401 for anonymous and API-key-only requests
- [x] RequireProjectRole returns 404 for non-members, 403 for insufficient role
- [x] Project role middleware works for both session users and API key identities
- [x] Invalid minRole panics at startup (fail-fast)
- [x] DB errors in identity resolution return 500
- [x] API keys use `mj_proj_` and `mj_pers_` prefixes with crypto/rand generation
- [x] Key hash (SHA-256) stored in database; plaintext returned only on creation
- [x] Project key creation requires admin+ on project
- [x] Personal key access is dynamic -- membership changes take immediate effect
- [x] MIN(key_role, mapped_membership_role) enforced for personal keys
- [x] `api_key_project_access` view returns correct effective_role for both key types
- [x] `last_used_at` updated on each authenticated API key request
- [x] Expired/inactive keys excluded from access checks
- [x] Platform admin middleware gates global settings routes
- [x] Integration tests cover session lifecycle, API key CRUD, auth middleware, and access checks
