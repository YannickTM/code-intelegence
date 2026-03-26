# Phase 2 — platform Admin Role

## Goal

Add a system-level `platform_admin` role that provides:

1. **Cross-project access** — read and manage all projects without per-project membership
2. **platform user management** — view, deactivate, and manage all platform users
3. **platform project management** — oversight and administrative actions on any project
4. **Server-wide settings** — manage platform defaults (embedding, provider credentials, etc.)

This keeps project RBAC (`owner`, `admin`, `member`) intact while adding a platform-wide administrative layer for operations and support.

## Role Model Update

Project-scoped roles remain unchanged:

- `owner`
- `admin`
- `member`

New platform role:

- `platform_admin` (system-level, not tied to one project)

Design intent:

- `platform_admin` is not automatically inserted into `project_members`
- Existing project permissions continue to work unchanged for non-platform users
- platform admin actions are always auditable

## Database Changes

Use a dedicated table instead of a boolean flag on `users` so the model can support future platform roles (e.g. `platform_viewer`).

```sql
CREATE TABLE user_platform_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('platform_admin')),
  granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, role)
);

CREATE INDEX idx_ugr_user ON user_platform_roles(user_id);
CREATE INDEX idx_ugr_role ON user_platform_roles(role);
```

Notes:

- Keep `project_members` unchanged
- Do not create cross-join views that explode `platform_admin` into all projects
- Resolve platform role first in authorization middleware, then fall back to project membership checks

## Authorization Rules

For any project-scoped route:

1. If user has `platform_admin` → allow (treat as owner-level)
2. Else evaluate `project_members` role checks as today

Ownership safety invariants still apply:

- A project must always have at least one owner
- Operations that would leave zero owners must be rejected, even for `platform_admin`

## platform User Management

Phase 1 ships with GitHub OAuth via better-auth for user identity. Users authenticate through GitHub and manage their own profiles. platform admin adds platform-wide user oversight.

### Endpoints

| Method | Path | Handler | Auth | Notes |
|--------|------|---------|------|-------|
| GET | `/v1/admin/users` | `ListAllUsers` | `platform_admin` | Paginated list of all platform users |
| GET | `/v1/admin/users/{userId}` | `GetUser` | `platform_admin` | Full user detail including memberships |
| PATCH | `/v1/admin/users/{userId}` | `UpdateUser` | `platform_admin` | Update display_name, avatar_url |
| POST | `/v1/admin/users/{userId}/deactivate` | `DeactivateUser` | `platform_admin` | Soft-disable user account |
| POST | `/v1/admin/users/{userId}/activate` | `ActivateUser` | `platform_admin` | Re-enable user account |

### List All Users

Paginated, filterable, sortable user list for the admin dashboard.

Query params: `limit`, `offset`, `search` (name/email substring), `is_active` (filter), `sort` (`created_at`, `name`).

Response:
```json
{
  "items": [
    {
      "id": "uuid",
      "name": "Alice",
      "email": "alice@example.com",
      "image": "https://avatars.githubusercontent.com/...",
      "is_active": true,
      "created_at": "...",
      "project_count": 3,
      "platform_roles": ["platform_admin"]
    }
  ],
  "total": 42
}
```

`project_count` is the number of projects the user is a member of. `platform_roles` is included so the admin UI can show badges.

### Get User Detail

Returns full user information including all project memberships.

Response:
```json
{
  "user": {
    "id": "uuid",
    "name": "Alice",
    "email": "alice@example.com",
    "image": "https://avatars.githubusercontent.com/...",
    "is_active": true,
    "created_at": "...",
    "updated_at": "..."
  },
  "platform_roles": ["platform_admin"],
  "memberships": [
    {
      "project_id": "uuid",
      "project_name": "my-frontend",
      "role": "owner",
      "joined_at": "..."
    }
  ]
}
```

### Deactivate User

Soft-disables a user account. The user can no longer log in or perform any actions, but their data (project memberships, API keys, etc.) is preserved.

Rules:

- Cannot deactivate yourself (prevents lockout)
- Cannot deactivate the last active `platform_admin`
- Existing better-auth sessions are invalidated immediately
- Active API keys created by this user remain functional (they belong to projects, not the user's session) — decision: should deactivation also revoke API keys? See open questions.

### Activate User

Re-enables a previously deactivated user. No automatic session creation — the user must log in again via GitHub OAuth.

### Admin User Update

platform admin can update another user's `display_name` and `avatar_url`. This is useful for correcting display names or clearing inappropriate avatars.

## platform Project Management

Phase 1 project management is scoped: only project members can see and manage their projects. platform admin adds cross-project oversight.

### Endpoints

| Method | Path | Handler | Auth | Notes |
|--------|------|---------|------|-------|
| GET | `/v1/admin/projects` | `ListAllProjects` | `platform_admin` | All projects, paginated |
| GET | `/v1/admin/projects/{projectId}` | `GetProjectAdmin` | `platform_admin` | Full project detail with members and stats |
| POST | `/v1/admin/projects/{projectId}/transfer-ownership` | `TransferOwnership` | `platform_admin` | Assign a new owner |

Project CRUD (PATCH, DELETE) for platform admins is already covered by the cross-project authorization rule — the existing `/v1/projects/{projectId}` endpoints allow platform admin access at owner level. No duplicate admin endpoints needed for standard CRUD.

### List All Projects

Admin-level project list with additional metadata not available in the regular list endpoint.

Query params: `limit`, `offset`, `search` (name substring), `status` (`active`, `paused`), `sort` (`created_at`, `name`, `member_count`).

Response:
```json
{
  "items": [
    {
      "id": "uuid",
      "name": "my-frontend",
      "repo_url": "git@github.com:org/my-frontend.git",
      "status": "active",
      "member_count": 5,
      "owner_names": ["Alice", "Bob"],
      "has_active_ssh_key": true,
      "has_embedding_config": true,
      "last_indexed_at": "...",
      "created_at": "..."
    }
  ],
  "total": 15
}
```

### Get Project Admin Detail

Extended project view with members, indexing stats, and config status.

Response:
```json
{
  "project": { ... },
  "members": [
    { "user_id": "uuid", "name": "Alice", "role": "owner", "joined_at": "..." }
  ],
  "ssh_key": {
    "fingerprint": "SHA256:...",
    "assigned_at": "..."
  },
  "embedding_config": {
    "source": "project",
    "provider": "ollama",
    "model": "jina/jina-embeddings-v2-base-en"
  },
  "indexing_stats": {
    "last_indexed_at": "...",
    "active_snapshot_commit": "abc123",
    "total_files": 342,
    "total_chunks": 2150
  }
}
```

### Transfer Ownership

Emergency action for when a project's owner leaves or is deactivated. Assigns a new owner without requiring the current owner's participation.

Request:
```json
{
  "new_owner_user_id": "uuid"
}
```

Rules:

- Target user must already be a member of the project (promotes to owner)
- If target user is not a member, they are added as owner
- Does not remove existing owners — it adds a new one
- The existing ownership invariants still apply (project always has at least one owner)

## platform Role Management

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/v1/admin/platform-roles` | `ListplatformRoles` | `platform_admin` |
| POST | `/v1/admin/platform-roles` | `GrantplatformRole` | `platform_admin` |
| DELETE | `/v1/admin/platform-roles/{userId}/{role}` | `RevokeplatformRole` | `platform_admin` |

Rules:

- Cannot revoke your own `platform_admin` role (prevents lockout)
- Cannot revoke the last `platform_admin` (platform must always have at least one)
- `granted_by` is recorded for audit

## platform Server Settings

The `platform_admin` manages server-wide default configuration via `/v1/settings/*` endpoints.

### Two-Tier Settings Pattern

```
Resolution order:  project override  →  platform default  →  404 (not configured)
```

- **platform defaults**: managed by `platform_admin` via `/v1/settings/*`
- **Project overrides**: managed by project `admin+` via `/v1/projects/{id}/settings/*`
- Consumers call a `GetResolved*` method that applies the resolution order transparently

This pattern is designed once and reused across all settings types.

### Phase 1 Forward Compatibility

Phase 1 already scaffolds the platform settings endpoints (e.g. `/v1/settings/embedding`) but returns **403** with a message directing users to configure per-project instead. Once the `platform_admin` role ships, these endpoints become functional — only the auth gate is lifted.

### Settings Roadmap

| Setting | platform Endpoint | Project Override | Details |
|---------|----------------|-----------------|---------|
| Embedding config | `/v1/settings/embedding` | `/v1/projects/{id}/settings/embedding` | See `backlog/05-multiple-embedding-providers.md` |
| Provider credentials | `/v1/admin/credentials` | — | Platform-wide credential management |
| Chunking strategy | `/v1/settings/chunking` | `/v1/projects/{id}/settings/chunking` | Future |
| Indexing schedule | `/v1/settings/indexing` | `/v1/projects/{id}/settings/indexing` | Future |
| Language support | `/v1/settings/languages` | `/v1/projects/{id}/settings/languages` | Future |

## Bootstrap Path

For first deployment, there is no platform admin to grant the role via API. Bootstrap options:

- **Environment variable**: `PLATFORM_ADMIN_EMAILS=alice@example.com,bob@example.com` applied at startup
- Seeds `user_platform_roles` rows for users matching the listed emails (users must already exist via GitHub OAuth)
- Runs on every startup but is idempotent (skips existing grants)
- Once at least one platform admin exists, further role management happens through the API

## Middleware Implementation

### `RequireplatformAdmin` Middleware

For admin routes (`/v1/settings/*`, `/v1/admin/*`):

```go
func RequireplatformAdmin(next http.Handler) http.Handler {
    // 1. Get user from context (via better-auth session middleware)
    // 2. Query user_platform_roles for platform_admin
    // 3. If not platform_admin → 403
    // 4. Else → proceed
}
```

### Updated `RequireProjectRole` Middleware

For project-scoped routes, add a platform admin short-circuit:

```go
func RequireProjectRole(minRole string) func(http.Handler) http.Handler {
    // 1. Get user from context
    // 2. Check user_platform_roles for platform_admin → if yes, allow (treat as owner-level)
    // 3. Else check project_members as today
    // 4. No membership + no platform role → 404
}
```

Cache consideration: `is_platform_admin` check should use a short-lived cache (e.g. 60s TTL) since this is checked on every request for platform admins.

## Backoffice Changes

### Navigation

- **Admin section** (visible only to platform admins):
  - Users — list, search, deactivate/activate, view detail
  - Projects — list all, view detail, transfer ownership
  - platform Roles — grant/revoke
  - Server Settings — embedding defaults (and future settings)
  - Credentials — provider API key management (see `backlog/05-multiple-embedding-providers.md`)

### Service Health

The admin dashboard shows connectivity status for platform services:

- backend-api
- backend-worker (may be scaled to zero when idle)
- PostgreSQL
- Qdrant
- Redis
- Ollama (or configured embedding provider)

### User Management UI

- Searchable user list with status badges (active/deactivated, platform admin)
- User detail page showing all project memberships
- Deactivate/activate buttons with confirmation dialogs
- Audit log of admin actions on the user

### Project Overview UI

- All-projects list with health indicators (has SSH key, has embedding config, last indexed)
- Project detail with member list, config status, indexing stats
- Transfer ownership action for orphaned projects

### platform Admin Badge

- Clear `platform Admin` badge on the current user's profile
- Badge on other users in member lists when they have a platform role

## External Auth Integration

Build on `backlog/01-external-authentication.md`:

- Map trusted identity claims (group/role) to `user_platform_roles`
- Example: IdP group `myjungle-platform-admins` → `platform_admin`
- Sync strategy: login-time upsert (simple) or periodic reconciliation (stricter)

## Auditing and Security

- Log all platform admin actions with: actor identity, action, target (user/project), result, timestamp
- Log platform settings changes with before/after values
- Require explicit confirmation for destructive actions (deactivate user, transfer ownership, delete project)
- Recommend at least two platform admins for break-glass resilience
- Prefer assigning platform role via IdP group in production rather than manual grants

## Migration Plan

1. Create `user_platform_roles` table and indexes
2. Seed initial platform admin(s) from `PLATFORM_ADMIN_EMAILS` env var
3. Add `RequireplatformAdmin` middleware
4. Update `RequireProjectRole` to short-circuit on platform admin
5. Add platform user management endpoints (`/v1/admin/users/*`)
6. Add platform project management endpoints (`/v1/admin/projects/*`)
7. Add platform role management endpoints (`/v1/admin/platform-roles/*`)
8. **Unblock platform settings endpoints** (remove 403 guard on `/v1/settings/*`)
9. Update `GET /v1/projects` and `GET /v1/users/me/projects` to return all projects for platform admins
10. Add backoffice admin section (users, projects, roles, settings)
11. Add audit logging for admin actions
12. Integration tests:
    - platform admin can access any project without membership
    - Non-member without platform role still gets `404`
    - Owner invariants enforced even for platform admin
    - platform admin can list/deactivate/activate users
    - Cannot deactivate yourself or last platform admin
    - platform admin can list all projects and transfer ownership
    - platform admin can manage platform settings and provider credentials
    - Project admin cannot access `/v1/admin/*` endpoints

## Open Questions

- Should `platform_admin` be able to hard-delete projects, or only soft-delete (archive)?
- Should deactivating a user also revoke their API keys, or leave them active (they're project-scoped)?
- Should there be a `platform_viewer` role for read-only cross-project access (e.g. monitoring dashboards)?
- Should platform admin actions trigger notifications to affected project owners?
- Should the platform enforce a maximum number of platform admins, or is "at least two recommended" sufficient?
