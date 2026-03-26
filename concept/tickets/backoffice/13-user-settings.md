# 13 — User Profile & Personal Keys

## Status
Done

## Goal
Built the user settings pages under `/settings/*` covering profile editing, personal API key management, and a global SSH key library. The settings layout provides a left sidebar nav (Profile, SSH Keys, API Keys, System) with the main content area to the right. API key and SSH key components are shared across personal and project contexts.

## Depends On
01-app-shell

## Scope

### Settings Layout (`/settings/*`)
`src/app/(app)/settings/layout.tsx` renders a `PageHeader` with title "Settings" and a responsive left nav with four items:
- Profile (`User` icon) at `/settings/profile`
- SSH Keys (`KeyRound` icon) at `/settings/ssh-keys`
- API Keys (`Key` icon) at `/settings/api-keys`
- System (`Settings` icon) at `/settings/system`

Active nav item highlighted via `usePathname()` comparison with `bg-accent text-accent-foreground`.

### Profile Page (`/settings/profile`)
- `<ProfileForm>` client component fetches current user via `api.auth.me.useQuery()`
- **Read-only section**: large `Avatar` with initials fallback via `getInitials()`, email address, member-since date
- **Editable form**: display name input (max 100 chars), avatar URL input (`type="url"`)
- **UX**: loading skeleton, dirty tracking (save/cancel buttons only appear when form is modified), inline success/error alerts, `api.users.updateMe` mutation with query invalidation on save (updates sidebar NavUser display name)

### Personal API Keys Page (`/settings/api-keys`)
- `<APIKeysContent>` client component using shared components from `~/components/api-keys`
- **Key table** via `<ApiKeyTable>`: columns for Name (bold), Key prefix (monospace muted), Role badge (`read` default/muted, `write` amber), Expires (relative time or "Never"), Last Used (relative time or "Never"), Created (relative time with tooltip), Actions (delete button)
- **Create key dialog** via `<CreateKeyDialog>`: name input (required, max 100 chars), role selector (read/write, default read), expiry presets (Never, 30 days, 90 days, 1 year, Custom date). On success transitions to key reveal view.
- **Key reveal** via `<KeyReveal>`: amber warning banner "Copy your key now. You won't be able to see it again.", full `plaintext_key` in monospace code block, copy button with "Copied" feedback, summary (name, role, expires), "Done" button closes dialog and invalidates query.
- **Delete confirmation** via `<DeleteKeyDialog>`: "Revoke API Key" heading, warning about immediate access loss, destructive "Revoke" button. Toast on success/error.
- **Empty state**: `Key` icon, "No API keys yet", "Create Key" CTA
- tRPC procedures: `users.listMyKeys`, `users.createMyKey`, `users.deleteMyKey`

### SSH Keys Page (`/settings/ssh-keys`)
- `<SSHKeysContent>` client component using shared components from `~/components/ssh-keys`
- **Key table** via `<SSHKeyTable>`: columns for Name, Key Type, Fingerprint (truncated monospace), Status badge (Active green / Retired muted), Created (relative time), Project Count, Actions dropdown (View details, Copy fingerprint, Retire)
- **Create key dialog** via `<CreateKeyDialog>`: key name input, calls `sshKeys.create`. On success shows the generated key details.
- **Key detail dialog** via `<KeyDetailDialog>`: full public key in copyable code block, fingerprint, key type, associated projects list fetched via `sshKeys.listKeyProjects`
- **Retire confirmation** via `<RetireKeyDialog>`: warns about project impact, destructive "Retire" button. Toast on success/error.
- **Empty state**: `KeyRound` icon, "No SSH keys yet", "Create Key" CTA
- tRPC procedures: `sshKeys.list`, `sshKeys.create`, `sshKeys.get`, `sshKeys.retire`, `sshKeys.listKeyProjects`

### Shared API Key Components (`src/components/api-keys/`)
Extracted to share between personal keys (this ticket) and project keys (ticket 05):

| Component | Purpose |
|---|---|
| `ApiKeyTable` | Table with name, prefix, role badge, expires, last used, created, actions |
| `CreateKeyDialog` | Name + role + expiry form with post-creation key reveal |
| `DeleteKeyDialog` | Revocation confirmation |
| `KeyReveal` | One-time plaintext key display with copy button and warning |
| `RoleBadge` | `read` / `write` badge styling |
| `types.ts` | `APIKeyBase` and `CreateAPIKeyResponseBase` shared types |

### Shared SSH Key Components (`src/components/ssh-keys/`)

| Component | Purpose |
|---|---|
| `SSHKeyTable` | Table with name, type, fingerprint, status, date, project count, actions dropdown |
| `CreateKeyDialog` | Key name form with creation mutation |
| `KeyDetailDialog` | Full public key display with associated projects |
| `RetireKeyDialog` | Retirement confirmation |
| `types.ts` | `SSHKey` and `SSHKeyProject` shared types |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/settings/layout.tsx` | Settings layout with left sidebar nav |
| `src/app/(app)/settings/profile/page.tsx` | Server page rendering ProfileForm |
| `src/components/settings/profile-form.tsx` | Profile edit form: avatar, display name, dirty tracking, mutation |
| `src/app/(app)/settings/api-keys/page.tsx` | Server page rendering APIKeysContent |
| `src/components/settings/api-keys-content.tsx` | Personal API keys: list, create, revoke using shared components |
| `src/app/(app)/settings/ssh-keys/page.tsx` | Server page rendering SSHKeysContent |
| `src/components/settings/ssh-keys-content.tsx` | SSH key library: list, create, detail, retire using shared components |
| `src/components/api-keys/` | Shared API key components (ApiKeyTable, CreateKeyDialog, DeleteKeyDialog, KeyReveal, RoleBadge) |
| `src/components/ssh-keys/` | Shared SSH key components (SSHKeyTable, CreateKeyDialog, KeyDetailDialog, RetireKeyDialog) |
| `src/server/api/routers/users.ts` | `updateMe`, `listMyKeys`, `createMyKey`, `deleteMyKey` procedures |
| `src/server/api/routers/ssh-keys.ts` | `list`, `create`, `get`, `retire`, `listKeyProjects` procedures |

## Acceptance Criteria
- [x] Settings layout renders with left sidebar nav and active item highlighting
- [x] Profile page shows read-only avatar/email and editable display name/avatar URL
- [x] Profile form tracks dirty state; save/cancel buttons appear only when modified
- [x] Profile save invalidates `auth.me` query and updates sidebar user display
- [x] API keys table shows all columns with correct formatting (monospace prefix, role badges, relative times)
- [x] Create key dialog with name, role, expiry presets; transitions to key reveal on success
- [x] Key reveal shows plaintext key once with copy button and amber warning
- [x] Delete key shows confirmation dialog; toast on success
- [x] SSH keys table shows name, type, truncated fingerprint, status badge, project count
- [x] SSH key detail dialog shows full public key, fingerprint, and associated projects
- [x] Retire key confirmation with toast feedback
- [x] Empty states render correctly for both API keys and SSH keys
- [x] Loading skeleton states render correctly on all three settings pages
- [x] Shared components used by both personal and project key pages
