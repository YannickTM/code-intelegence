# 04 — Project CRUD, Structure and Health

## Status
Done

## Goal
Implemented the full project lifecycle: create with SSH key assignment and automatic owner membership, list with pagination, get with live index health fields, update, and delete. Also implemented project membership management with role hierarchy and ownership invariants, the project structure endpoint (nested file tree), and the extended project detail with health data from index snapshots and indexing jobs.

## Depends On
01-foundation, 02-authentication, 05-ssh-key-management

## Scope

### Project Create

`POST /v1/projects` accepts `name`, `repo_url`, `default_branch` (optional, defaults to "main"), and `ssh_key_id`. Executes in a single transaction:
1. Validate SSH key exists, is active, and belongs to the creator (`created_by = actor`)
2. Insert into `projects` table
3. Insert `project_members` row (creator as "owner")
4. Insert `project_ssh_key_assignments` row (active)

Validation: name required (1-100 chars), repo_url required (must start with `git@` or `https://`), ssh_key_id must reference an active key owned by the caller.

### Project List and Get

`GET /v1/projects` returns paginated list with `limit` (default 20, max 200) and `offset` query params, plus `total` count.

`GET /v1/projects/{id}` uses `GetProjectWithHealth` query that extends the base project fields with live health data via `LEFT JOIN LATERAL` subqueries:
- **Active snapshot**: `index_git_commit`, `index_branch`, `index_activated_at` from `index_snapshots`
- **Active job**: `active_job_id`, `active_job_status` from `indexing_jobs` (queued/running)
- **Recent failure**: `failed_job_id`, `failed_job_finished_at`, `failed_job_type` from `indexing_jobs` (failed within 24h, not superseded by a later success)

All health fields are nullable -- null when no data exists. Backward-compatible with consumers that ignore unknown fields.

### Project Update and Delete

`PATCH /v1/projects/{id}` (admin+) accepts optional `name`, `default_branch`, `status` (active/paused). Uses COALESCE pattern for partial updates.

`DELETE /v1/projects/{id}` (owner only) performs hard delete with CASCADE on child rows.

### SSH Key Operations on Projects

`GET /v1/projects/{id}/ssh-key` (member+) returns the assigned public key and fingerprint.

`PUT /v1/projects/{id}/ssh-key` (admin+) supports two flows:
- **Reassign**: provide `ssh_key_id` -- validates key exists, is active, belongs to caller, deactivates old assignment, creates new active assignment.
- **Generate-and-assign**: provide `generate: true` and `name` -- generates new Ed25519 key in caller's library and assigns it atomically.

`DELETE /v1/projects/{id}/ssh-key` (admin+) deactivates current assignment, returns 204. Project has no active key until reassigned.

### Project Membership and RBAC

Endpoints under `/v1/projects/{id}/members`:
- `GET` (member+): list members with user info (username, display_name, email, avatar_url, role)
- `POST` (admin+): add member with specified role
- `PATCH /{userID}` (admin+): change member role
- `DELETE /{userID}` (member+ for self-removal, admin+ for removing others)

Role hierarchy: `owner (3) > admin (2) > member (1)`. Invariants enforced in `internal/membership/service.go`:
- Owners can set any role; admins can set admin/member but not owner
- Cannot change your own role
- Cannot demote or remove the last owner (checked via `CountProjectOwnersForUpdate` with row locking)
- Any member can self-remove (leave) unless they are the last owner

### Project Structure

`GET /v1/projects/{id}/structure` (member+) returns the project's nested file/directory tree from the active index snapshot. The response represents the repository structure as a hierarchical JSON tree.

### Dashboard

`GET /v1/dashboard/summary` returns a cross-project summary for the authenticated user including project counts, active jobs, recent failures, and index health overview.

### Jobs List

`GET /v1/projects/{id}/jobs` (member+) returns the indexing job history for a project with pagination.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/handler/project.go` | HandleCreate, HandleList, HandleGet, HandleUpdate, HandleDelete, HandleGetSSHKey, HandleSetSSHKey, HandleRemoveSSHKey, HandleStructure, HandleIndex, HandleListJobs |
| `internal/handler/membership.go` | HandleList, HandleAdd, HandleUpdate, HandleRemove |
| `internal/handler/dashboard.go` | HandleSummary |
| `internal/membership/service.go` | Role hierarchy enforcement, ownership invariants |
| `internal/domain/models.go` | Project, ProjectMember, role constants, RoleRank/RoleSufficient |
| `internal/app/routes.go` | Project-scoped routes with memberAccess/adminAccess/ownerAccess stacks |
| `datastore/postgres/queries/projects.sql` | GetProjectWithHealth, ListUserProjectsWithHealth, CRUD queries |
| `tests/integration/project_test.go` | 27+ integration tests for CRUD, SSH key, access control |
| `tests/integration/membership_test.go` | 31+ integration tests for role changes, invariants, RBAC |

## Acceptance Criteria
- [x] Project create with SSH key assignment works in a single transaction
- [x] Creator automatically becomes project owner
- [x] List projects supports pagination with total count
- [x] Get project returns full detail with live health fields for members
- [x] Health fields are null when no snapshot/job data exists
- [x] Active job and recent failure fields populated from indexing_jobs
- [x] Failed jobs older than 24h or superseded by success return null
- [x] Update project allows partial updates (COALESCE pattern)
- [x] Delete project removes all related data (CASCADE)
- [x] SSH key reassignment deactivates old, creates new assignment
- [x] SSH key reassignment validates key belongs to caller's library
- [x] Generate-and-assign creates new key and assigns in one transaction
- [x] Remove SSH key deactivates assignment and returns 204
- [x] Owner can promote anyone to any role including owner
- [x] Admin can set admin/member but not owner
- [x] Last owner cannot be demoted or removed
- [x] Self-demotion is rejected; self-removal allowed unless last owner
- [x] Non-members get 404; insufficient role gets 403
- [x] Project structure endpoint returns nested file tree
- [x] Dashboard summary returns cross-project health overview
- [x] Response is backward-compatible with existing consumers
