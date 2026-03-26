# 07 — Project & Personal API Keys

## Status
Done

## Goal
Implemented a dual-type API key system for programmatic access. Project keys (`mj_proj_*`) are scoped to a single project and managed by project admins. Personal keys (`mj_pers_*`) span all projects the user belongs to, with access derived dynamically from membership. Keys are stored as SHA-256 hashes with the plaintext returned exactly once on creation.

## Depends On
- Ticket 04 (User Identity & Session)
- Ticket 05 (Project CRUD)
- Ticket 06 (Project Membership)

## Scope

### Key Types and Formats

| Type | Prefix | Scoping | Managed By |
|------|--------|---------|------------|
| Project | `mj_proj_` + 32 base62 chars | Single project via `project_id` FK | Project admin+ |
| Personal | `mj_pers_` + 32 base62 chars | All user projects via `project_members` JOIN | User (session auth) |

Keys are generated with `crypto/rand` (24 random bytes, base62-encoded to 32 characters). The first 12 characters are stored as `key_prefix` for display identification. The full key is hashed with SHA-256 and only the hash is persisted.

### Endpoints

**Project keys** (`/v1/projects/{projectID}/keys`):

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| POST | `/v1/projects/{projectID}/keys` | `HandleCreateProjectKey` | admin+ |
| GET | `/v1/projects/{projectID}/keys` | `HandleListProjectKeys` | member+ |
| DELETE | `/v1/projects/{projectID}/keys/{keyID}` | `HandleDeleteProjectKey` | admin+ |

**Personal keys** (`/v1/users/me/keys`):

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| POST | `/v1/users/me/keys` | `HandleCreatePersonalKey` | session |
| GET | `/v1/users/me/keys` | `HandleListPersonalKeys` | session |
| DELETE | `/v1/users/me/keys/{keyID}` | `HandleDeletePersonalKey` | session |

### Role Semantics

- **Project keys**: The key's `role` field (`read` or `write`) is the effective access level directly.
- **Personal keys**: Effective role is `MIN(key_role, mapped_membership_role)`. Membership roles map as: owner/admin -> write, member -> read. Access is dynamic -- adding or removing a user from a project immediately changes their personal key's access.

### Access Validation

A unified `api_key_project_access` database view handles both key types. The `CheckAPIKeyProjectAccess` query returns `effective_role` for a given `key_hash` + `project_id` pair, handling expiry/active filtering and role computation. `last_used_at` is updated on each authenticated request via a fire-and-forget goroutine in `IdentityResolver`.

### Validation

- `name`: optional, max 100 characters
- `role`: optional (defaults to `read`), must be `read` or `write`
- `expires_at`: optional, must be a future RFC 3339 timestamp
- Project key deletion requires admin+ and verifies `project_id` matches
- Personal key deletion verifies `created_by = caller`

## Key Files

| File | Purpose |
|------|---------|
| `backend-api/internal/apikey/service.go` | Business logic for both key types |
| `backend-api/internal/apikey/keygen.go` | Key generation with crypto/rand and SHA-256 hashing |
| `backend-api/internal/handler/apikey.go` | Six HTTP handlers (create/list/delete for each type) |
| `datastore/postgres/queries/api_keys.sql` | 8 queries including `CheckAPIKeyProjectAccess` |
| `datastore/postgres/migrations/000001_init.up.sql` | `api_keys` table and `api_key_project_access` view |

## Acceptance Criteria
- [x] `mj_proj_` and `mj_pers_` key formats generated correctly with crypto/rand
- [x] Plaintext key returned only on creation, never on list
- [x] Key hash stored in database (SHA-256)
- [x] Project key uses direct `project_id` FK (no join table)
- [x] Personal key has `project_id = NULL`
- [x] `api_key_project_access` view returns correct `effective_role` for both types
- [x] Personal key access is dynamic -- membership changes take effect immediately
- [x] MIN(key_role, mapped_membership_role) enforced for personal keys
- [x] Project key creation requires admin+ on the project
- [x] Project key deletion requires admin+ and verifies key belongs to that project
- [x] Personal key deletion verifies `created_by = caller`
- [x] Expired/inactive keys excluded from access checks (view handles this)
- [x] `last_used_at` updated on each authenticated request
- [x] Invalid inputs rejected with 400 and descriptive errors
- [x] Max 50 keys per entity enforced
- [x] Expiration support with future-date validation
