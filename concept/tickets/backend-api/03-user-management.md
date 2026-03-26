# 03 — User Registration, Profile and Lookup

## Status
Done

## Goal
Implemented user registration, profile management, email field, and user lookup. Users register with a username and email, manage their profile (display name, avatar, email), view their project memberships, and can be looked up by username or email for cross-project member invitations.

## Depends On
01-foundation, 02-authentication

## Scope

### User Registration

`POST /v1/users` (public, no auth) accepts `username`, `email`, and optional `display_name`. Username is normalized (lowercase, trimmed). Email is validated, normalized to lowercase, and stored with a unique constraint. Duplicate username returns 409; duplicate email returns 409. The user is created with `is_active = TRUE`.

### User Profile

`GET /v1/users/me` returns the authenticated user's full profile including `id`, `username`, `email`, `display_name`, `avatar_url`, `is_active`, `created_at`, `updated_at`.

`PATCH /v1/users/me` accepts optional `display_name`, `avatar_url`, and `email`. Rejects empty payloads (no fields provided) with 400. Rejects blank display_name with 400. Avatar clearing uses `CASE WHEN ... THEN NULLIF(..., '') ELSE avatar_url END` pattern. Email updates validate format and handle unique constraint violations (409). All provided fields use `COALESCE`/conditional SQL to leave omitted fields unchanged.

### User Projects

`GET /v1/users/me/projects` returns all projects the authenticated user is a member of, with their role and live index health fields (`index_git_commit`, `index_branch`, `index_activated_at`, `active_job_id`, `active_job_status`, `failed_job_id`, `failed_job_finished_at`, `failed_job_type`). Uses `ListUserProjectsWithHealth` query with `LEFT JOIN LATERAL` subqueries against `index_snapshots` and `indexing_jobs`.

### User Lookup

`GET /v1/users/lookup?q={query}` resolves a username or email to a minimal user profile. Requires authenticated session. Tries username match first, then email match (both case-insensitive via normalization). Returns `{id, username, email, display_name, avatar_url}` on success. Returns 404 for non-existent or inactive users (does not leak active/inactive status). Returns 400 for missing or empty `q` parameter.

### Email Field

The `users` table in `000001_init` includes `email TEXT NOT NULL` with a `UNIQUE` index. The `Email` field propagated through: sqlc queries (`CreateUser`, `UpdateUserProfile`, `GetUserByEmail`, `ListProjectMembers`), domain model (`domain.User`), dbconv (`DBUserToDomain`), handler serialization, and member list responses.

`internal/validate/validate.go` includes an `Email` validator that checks for a single `@` with non-empty local and domain parts.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/handler/user.go` | HandleRegister, HandleGetMe, HandleUpdateMe, HandleMyProjects, HandleLookupUser |
| `internal/domain/models.go` | User struct with Email field |
| `internal/dbconv/dbconv.go` | DBUserToDomain with Email mapping |
| `internal/validate/validate.go` | Email validator |
| `internal/app/routes.go` | User routes under RequireUser group |
| `datastore/postgres/migrations/000001_init.up.sql` | Users table with email column |
| `datastore/postgres/queries/auth.sql` | CreateUser, UpdateUserProfile, GetUserByEmail, GetUserByUsername |
| `tests/integration/user_test.go` | 18+ integration tests for registration, login, profile, lookup |

## Acceptance Criteria
- [x] `POST /v1/users` creates user with username and email
- [x] Username is normalized to lowercase
- [x] Email is validated, normalized to lowercase, and unique
- [x] Duplicate username returns 409
- [x] Duplicate email returns 409
- [x] `GET /v1/users/me` returns full profile including email
- [x] `PATCH /v1/users/me` supports optional display_name, avatar_url, email updates
- [x] Empty PATCH payloads rejected with 400
- [x] Blank display_name rejected with 400
- [x] Email update handles unique constraint violation with 409
- [x] `GET /v1/users/me/projects` returns projects with role and live health fields
- [x] `GET /v1/users/lookup?q=username` finds user by username
- [x] `GET /v1/users/lookup?q=email` finds user by email
- [x] Lookup returns 404 for non-existent or inactive users
- [x] Lookup returns 400 for missing or empty q parameter
- [x] Lookup returns 401 for unauthenticated requests
- [x] Lookup response excludes is_active and timestamps
- [x] Init migration creates users table with email as NOT NULL with UNIQUE index
- [x] Login response includes email in user object
- [x] Member list response includes email per member
