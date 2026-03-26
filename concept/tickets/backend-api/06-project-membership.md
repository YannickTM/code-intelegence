# 06 — RBAC & Member Management

## Status
Done

## Goal
Built a three-level role hierarchy (owner > admin > member) for project membership with full CRUD operations, ownership invariants, and real-time SSE notifications. This provides the authorization backbone for all project-scoped operations across the platform.

## Depends On
- Ticket 04 (User Identity & Session Middleware)
- Ticket 05 (Project CRUD)

## Scope

### Role Hierarchy

Three roles with numeric levels: `owner (3) > admin (2) > member (1)`. Role enforcement is applied at the service layer inside transactions, not at the middleware level alone.

### Endpoints

| Method | Path | Handler | Min Role |
|--------|------|---------|----------|
| GET | `/v1/projects/{projectID}/members` | `HandleList` | member |
| POST | `/v1/projects/{projectID}/members` | `HandleAdd` | admin |
| PATCH | `/v1/projects/{projectID}/members/{userID}` | `HandleUpdate` | admin |
| DELETE | `/v1/projects/{projectID}/members/{userID}` | `HandleRemove` | member |

The DELETE route is gated at member level to allow self-removal (leaving a project). Removing other members requires admin or higher, enforced in the service layer.

### Membership Service (`internal/membership/`)

Service methods: `List`, `Add`, `UpdateRole`, `Remove`. All role and ownership invariant checks are performed within PostgreSQL transactions using `SELECT FOR UPDATE` (`CountProjectOwnersForUpdate`) to prevent TOCTOU races on owner demotion and removal.

### Ownership Invariants

- The last owner of a project cannot be demoted or removed (409 Conflict).
- A user cannot change their own role (400 Bad Request).
- Admins cannot grant, revoke, or demote the owner role.
- Any member may self-remove (leave), except the last owner.

### SSE Integration

Member changes emit SSE events (`member:added`, `member:role_updated`, `member:removed`) via `Hub.PublishToUser` for immediate delivery to the affected user. Events are also forwarded through a Redis `EventPublisher` so remote API instances receive the notification. The Hub's live membership sets are updated in real time: `AddProjectForUser` on add, `RemoveProjectForUser` on remove.

### Response Shapes

List returns `{ "items": [...] }` with each item containing `id`, `project_id`, `user_id`, `username`, `display_name`, `avatar_url`, `role`, `created_at` (joined from the `users` table). Update returns the updated member object. Remove returns 204 No Content.

## Key Files

| File | Purpose |
|------|---------|
| `backend-api/internal/membership/service.go` | Business logic with role hierarchy and ownership invariants |
| `backend-api/internal/handler/membership.go` | HTTP handlers for all four endpoints |
| `backend-api/internal/sse/hub.go` | `AddProjectForUser`, `RemoveProjectForUser`, `PublishToUser` |
| `datastore/postgres/queries/auth.sql` | `ListProjectMembers`, `CountProjectOwnersForUpdate` |

## Acceptance Criteria
- [x] List members returns user info (username, display_name, avatar_url) joined with membership
- [x] Owner can promote anyone to any role including owner
- [x] Admin can set admin/member but not owner
- [x] Member cannot change any roles
- [x] Last owner cannot be demoted or removed (409)
- [x] Self-role-change is rejected (400)
- [x] Any member can leave (self-remove) unless they are the last owner
- [x] Role changes use correct 403/404/409 status codes
- [x] All invariants enforced within transactions using SELECT FOR UPDATE
- [x] SSE events emitted on add, role update, and remove
- [x] Hub live membership sets updated on add/remove for already-connected clients
- [x] Events forwarded via Redis EventPublisher for multi-instance delivery
