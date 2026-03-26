# 04 — Project Members & SSH Keys

## Status
Done

## Goal
Built the project member management UI within project settings and the user-scoped SSH key library under personal settings. Members can be listed, added by username/email lookup, have their roles changed via actions menu, and be removed with confirmation. SSH keys can be listed, generated, imported, viewed with assigned projects, and retired.

## Depends On
03-projects

## Scope

### Project Members (Settings Section)

Located as a Card section in `/projects/[id]/settings`, replacing the placeholder. Requires admin+ project role to access (settings tab hidden for members).

**Members Table** (shadcn Table):
- Columns: Member (avatar + display name + username, "(you)" for current user), Role (badge: owner purple, admin amber, member muted), Joined (relative time), Actions (dropdown)
- Sorted by `created_at` ascending (project creator at top)

**Actions Menu** (per-row `MoreHorizontal` dropdown):
- For admin viewing a member: "Change role" submenu (admin/member) + "Remove from project"
- For owner viewing any non-owner: "Change role" submenu (owner/admin/member) + "Remove from project"
- For owner viewing another owner: "Change role" submenu (admin/member) + "Remove from project"
- Self row: no actions (self-role-change blocked; self-removal deferred)
- Hidden entirely when no valid actions exist

**Role Change Flow**: submenu shows valid target roles, current role checked and disabled. Owner promotion triggers a confirmation dialog. Other changes apply immediately. Last-owner invariant enforced (backend rejects demotion of last owner).

**Add Member Dialog**: two-step flow — (1) `users.lookupUser({ q })` resolves username or email to user info, (2) confirmation with resolved avatar/name/email, (3) `projectMembers.add({ projectId, user_id, role })`. Role selector defaults to "member"; owners also see "Owner" option.

**Remove Member Dialog**: confirmation with destructive "Remove" button, calls `projectMembers.remove({ projectId, userId })`.

**States**: loading (3 skeleton rows), error (inline with retry), empty ("No members found").

### SSH Key Library (`/settings/ssh-keys`)

User-scoped SSH key management page under Settings.

**Keys Table** (shadcn Table):
- Columns: Name (bold), Fingerprint (truncated 16 chars, full in tooltip), Type (ed25519/rsa), Status (Active green / Retired grey badge), Created (relative time), Actions (dropdown)
- Active keys first, then by `created_at` descending
- Retired keys have reduced opacity styling
- Row click opens Key Detail dialog

**Actions Dropdown**: View details, Copy public key (clipboard + toast), Retire (active keys only, opens confirmation).

**Create Key Dialog**: two modes toggled by radio — Generate (name input, calls `sshKeys.create({ name })` for Ed25519) or Import (name + private key textarea, calls `sshKeys.create({ name, private_key })`). Post-creation: dialog transitions to "Key Created" view showing public key in copyable code block + fingerprint, "Copy Public Key" button, "Done" closes dialog.

**Key Detail Dialog**: full key info (fingerprint, public key in monospace code block, type, status, created date), assigned projects list (loaded via `sshKeys.getProjects({ id })`, each with link to `/projects/[id]`), "Copy Public Key" and "Retire Key" action buttons.

**Retire Key Flow**: pre-check via `sshKeys.getProjects` — if key has active project assignments, show blocking warning listing projects ("Reassign or remove the key before retiring"). If no assignments, show confirmation dialog, call `sshKeys.retire({ id })`.

**States**: loading (skeleton rows), empty ("No SSH keys yet" with CTA).

### tRPC Procedures

| Procedure | Backend Endpoint | Purpose |
|---|---|---|
| `projectMembers.list` | `GET /v1/projects/{id}/members` | List all project members |
| `projectMembers.add` | `POST /v1/projects/{id}/members` | Add member by user_id + role |
| `projectMembers.updateRole` | `PATCH /v1/projects/{id}/members/{userId}` | Change member role |
| `projectMembers.remove` | `DELETE /v1/projects/{id}/members/{userId}` | Remove member |
| `users.lookupUser` | `GET /v1/users/lookup?q=xxx` | Resolve username/email to user info |
| `sshKeys.list` | `GET /v1/ssh-keys` | List all user SSH keys |
| `sshKeys.create` | `POST /v1/ssh-keys` | Generate or import SSH key |
| `sshKeys.get` | `GET /v1/ssh-keys/{id}` | Key detail |
| `sshKeys.getProjects` | `GET /v1/ssh-keys/{id}/projects` | Projects assigned to a key |
| `sshKeys.retire` | `POST /v1/ssh-keys/{id}/retire` | Retire (deactivate) a key |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/projects/[id]/components/settings-members.tsx` | Members Card section with table |
| `src/app/(app)/projects/[id]/components/add-member-dialog.tsx` | Username/email lookup + add flow |
| `src/app/(app)/projects/[id]/components/remove-member-dialog.tsx` | Remove confirmation dialog |
| `src/app/(app)/settings/ssh-keys/page.tsx` | SSH keys settings page |
| `src/app/(app)/settings/ssh-keys/ssh-keys-content.tsx` | Keys table, create/detail/retire dialogs |
| `src/server/api/routers/project-members.ts` | Member management tRPC router |
| `src/server/api/routers/ssh-keys.ts` | SSH key library tRPC router |
| `src/server/api/routers/users.ts` | User lookup procedure |

## Acceptance Criteria
- [x] Members table renders with avatar, name, role badge, joined date, and actions
- [x] Current user row shows "(you)" label with no actions menu
- [x] Add member dialog: two-step lookup (username/email) then confirmation
- [x] Add member: role defaults to "member"; owners see "Owner" option
- [x] Role change via submenu; owner promotion requires confirmation dialog
- [x] Last-owner protection: backend rejects demotion/removal of last owner
- [x] Remove member: confirmation dialog with destructive action
- [x] All member mutations show toast feedback and invalidate member list
- [x] SSH keys table renders with name, fingerprint, type, status, created date
- [x] Create key dialog: generate (name only) and import (name + private key) modes
- [x] Post-creation: public key shown in copyable block before dialog closes
- [x] Key detail dialog: full info, assigned projects list, copy and retire actions
- [x] Retire flow: blocked when key has active project assignments
- [x] Retired keys shown with reduced opacity and "Retired" badge
- [x] Error and empty states handled for both members and SSH keys
