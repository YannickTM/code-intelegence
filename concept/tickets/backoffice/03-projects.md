# 03 — Project List, Create Wizard & Detail Frame

## Status
Done

## Goal
Built the projects list page with search and filtering, a multi-step create project wizard that walks users through repository connection and SSH key setup, and the project detail frame with GitHub-like tab navigation. These three pieces form the primary navigation flow for project management.

## Depends On
01-app-shell, 02-dashboard

## Scope

### Projects List Page (`/projects`)

Table-based list view of all user-accessible projects from `users.listMyProjects`.

**Toolbar**: text search input (client-side filter by name), status filter chips (All / Active / Paused).

**Table columns**: Name (bold name + muted repo URL), Branch, Status (derived badge), Last Indexed (relative time with commit hash tooltip), Role (owner/admin/member badge), Actions (dropdown menu).

**Status derivation** (priority order): Indexing (blue, when `active_job_status` is queued/running) > Error (red, when `failed_job_id` is set) > Paused (yellow) > Active (green, default). Implemented in `deriveProjectStatus()` utility.

**Actions dropdown**: Open, Settings, Trigger Index (calls `projectIndexing.triggerIndex`), Pause/Resume, Delete (owner-only, with confirmation dialog).

**Empty state**: "No projects yet" with "Create Project" CTA.

**Loading state**: skeleton table rows.

All filtering is client-side; no server-side pagination (dataset bounded by user membership).

### Create Project Wizard (`/projects/new`)

Full-screen overlay route with a 4-step sequential assistant for connecting a new repository.

**Step 1 — Project Details**: repository URL input (validates `git@` or `https://` format), auto-populated project name from URL slug, default branch input (defaults to `main`). No API call.

**Step 2 — SSH Key**: two selectable cards — Option A: generate a new Ed25519 key (default, with editable key name pre-filled as `{project-name}-deploy-key`), Option B: select an existing key from `sshKeys.list` dropdown. On "Continue", Option A calls `sshKeys.create({ name })`.

**Step 3 — Add Deploy Key**: displays generated public key in a copyable code block with fingerprint, provider-specific deep links (GitHub/GitLab/Bitbucket detected from repo URL), collapsible setup instructions, confirmation checkbox ("I've added the deploy key"). On "Create Project", calls `projects.create({ name, repo_url, default_branch, ssh_key_id })`.

**Step 4 — Done**: project confirmation, "Start first index" button calling `projectIndexing.triggerIndex({ projectId, job_type: "full" })`, and "Go to project" navigation link.

**Navigation**: numbered step indicator (1-4), back button on steps 2-4, cancel button with contextual confirmation (warns about orphaned key if generated).

### Project Detail Frame (`/projects/[id]`)

GitHub-like project detail layout with pinned header and horizontal tab navigation.

**Project Header** (persistent across tabs in layout): project name (large), repo URL (clickable), branch badge + indexed commit hash, status badge (active/paused), health indicator dot + label, "Trigger Index" split button (incremental default, full in dropdown), overflow menu (Pause/Resume, Copy URL, Settings).

**Tab Navigation** (route-based via `usePathname()`):

| Tab | Route | Visibility |
|---|---|---|
| Overview | `/projects/:id` | All roles |
| Files | `/projects/:id/file` | All roles |
| Commits | `/projects/:id/commits` | All roles |
| Symbols | `/projects/:id/symbols` | All roles |
| Search | `/projects/:id/search` | All roles |
| Indexing | `/projects/:id/jobs` | All roles |
| Settings | `/projects/:id/settings` | Admin + owner only |

**Overview Tab** (two-column layout):
- Left (~65%): file browser component (implemented in separate ticket)
- Right (~35%): Index Summary Card (branch, commit, indexed-at, health dot), SSH Deploy Key Card (key label, fingerprint, copyable public key, provider doc links, reassign button), Quick Actions (trigger full/incremental index)

**Settings Tab** (admin/owner only): General edit form (name, repo_url, default_branch, status toggle), SSH key assignment section, Members section, Embedding settings section, LLM settings section, API keys section, Danger Zone (delete project with name-confirmation dialog, owner-only).

### tRPC Procedures

| Procedure | Backend Endpoint | Purpose |
|---|---|---|
| `users.listMyProjects` | `GET /v1/users/me/projects` | All user projects with health fields |
| `projects.create` | `POST /v1/projects` | Create project (name, repo_url, default_branch, ssh_key_id) |
| `projects.get` | `GET /v1/projects/{id}` | Project detail with health fields |
| `projects.update` | `PATCH /v1/projects/{id}` | Update project settings |
| `projects.delete` | `DELETE /v1/projects/{id}` | Delete project |
| `projects.getSSHKey` | `GET /v1/projects/{id}/ssh-key` | Current assigned SSH key |
| `projects.assignSSHKey` | `PUT /v1/projects/{id}/ssh-key` | Assign/reassign SSH key |
| `sshKeys.list` | `GET /v1/ssh-keys` | Available SSH keys for selection |
| `sshKeys.create` | `POST /v1/ssh-keys` | Generate new SSH key |
| `projectIndexing.triggerIndex` | `POST /v1/projects/{id}/index` | Trigger full/incremental index |
| `projectIndexing.listJobs` | `GET /v1/projects/{id}/jobs` | Job list for index summary |

### Hooks

| Hook | Purpose |
|---|---|
| `useProjectDetail(projectId)` | Fetches project, derives health status, provides `role` and `isAdminOrOwner` |
| `useProjectDetailMutations(projectId)` | Centralized mutations for trigger index, update, delete with toast and query invalidation |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/projects/page.tsx` | Server page rendering ProjectsContent |
| `src/app/(app)/projects/projects-content.tsx` | Project list: queries, filters, mutations |
| `src/app/(app)/projects/lib/project-status.ts` | `deriveProjectStatus()` + badge config |
| `src/app/(app)/projects/components/projects-toolbar.tsx` | Search + status filter chips |
| `src/app/(app)/projects/components/projects-table.tsx` | Table with all columns |
| `src/app/(app)/projects/components/project-row.tsx` | Clickable row with navigation |
| `src/app/(app)/projects/components/project-actions.tsx` | Dropdown menu actions |
| `src/app/(app)/projects/components/projects-empty-state.tsx` | Empty state CTA |
| `src/app/(app)/projects/components/delete-project-dialog.tsx` | Delete confirmation |
| `src/app/(app)/projects/new/page.tsx` | Create project wizard (4-step) |
| `src/app/(app)/projects/[id]/layout.tsx` | Project header + tab navigation |
| `src/app/(app)/projects/[id]/page.tsx` | Overview tab (two-column) |
| `src/app/(app)/projects/[id]/settings/page.tsx` | Settings tab sections |
| `src/app/(app)/projects/[id]/lib/use-project-detail.ts` | Project data hook |
| `src/app/(app)/projects/[id]/lib/use-project-detail-mutations.ts` | Mutation hooks |
| `src/app/(app)/projects/[id]/components/project-detail-header.tsx` | Header with split trigger button |

## Acceptance Criteria
- [x] Projects list renders with search, status filters, and all table columns
- [x] Status badges derived correctly (Indexing > Error > Paused > Active)
- [x] Row click navigates to `/projects/[id]`; actions dropdown functional
- [x] "Trigger Index" action works end-to-end with toast feedback
- [x] Empty state shows "No projects yet" with CTA
- [x] Create wizard: 4-step flow with step indicator and back navigation
- [x] Wizard Step 1: repo URL validation and auto-populated project name
- [x] Wizard Step 2: generate new key or select existing from dropdown
- [x] Wizard Step 3: copyable public key, provider deep links, confirmation checkbox
- [x] Wizard Step 4: "Start first index" triggers full index
- [x] Project detail header shows name, repo, branch, commit, health status
- [x] Split trigger button sends `job_type: "incremental"` (default) or `"full"` (dropdown)
- [x] Tab navigation route-based; Settings tab hidden for member role
- [x] Overview tab: Index Summary Card, SSH Deploy Key Card, Quick Actions
- [x] Settings tab: General form, SSH key, Members, API keys, Danger Zone sections
- [x] Sidebar auto-collapses on project detail entry
