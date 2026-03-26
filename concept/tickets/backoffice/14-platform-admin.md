# 14 — Platform Administration UI

## Status
Done

## Goal
Built the platform administration pages under `/platform-settings/*` for managing users, embedding providers, LLM providers, and monitoring worker status. The entire section is gated by the `platform_admin` role with a client-side redirect to `/dashboard` for non-admins. Each sub-page follows a consistent card/table pattern with CRUD operations and confirmation dialogs.

## Depends On
01-app-shell

## Scope

### Platform Settings Layout (`/platform-settings/*`)
`src/app/(app)/platform-settings/layout.tsx` renders a `PageHeader` with title "Platform Settings" and a responsive left nav with four items:
- Users (`Users` icon) at `/platform-settings/users`
- Embedding (`Database` icon) at `/platform-settings/embedding`
- LLM (`Bot` icon) at `/platform-settings/llm`
- Workers (`Activity` icon) at `/platform-settings/workers`

**Role gate**: layout queries `api.auth.me` and checks for `platform_admin` in `platform_roles`. Non-admins are redirected to `/dashboard` via `router.replace()`. Layout renders nothing during the auth check to prevent flash.

### User Management (`/platform-settings/users`)
`<UserManagementContent>` client component with:

**Filter bar**:
- Search input with `Search` icon prefix, debounced at 300 ms, filters by username/display name/email
- Status filter buttons: All / Active / Inactive
- Role filter buttons: All / Platform Admin / Regular User

**Users table** via `<UserTable>`:

| Column | Content |
|---|---|
| User | Avatar + display name + `@username` (stacked) |
| Email | Email address |
| Role | `Platform Admin` badge when user has `platform_admin` role |
| Status | `Active` (green badge) or `Inactive` (muted badge) |
| Joined | Relative date with tooltip for absolute |
| Actions | `<UserActions>` dropdown menu |

**Per-user actions dropdown** via `<UserActions>` (triggered by `MoreHorizontal` icon):

| Action | Condition | Confirmation |
|---|---|---|
| Grant Platform Admin | User has no `platform_admin` role | Yes -- dialog |
| Revoke Platform Admin | User has `platform_admin` role | Yes -- dialog (destructive) |
| Deactivate User | User is active | Yes -- dialog (destructive) |
| Activate User | User is inactive | No confirmation |

**Self-protection**: current user cannot deactivate themselves or revoke their own admin role; those actions are disabled with tooltip explanation.

**Last admin protection**: backend enforces the last active admin cannot lose their role or be deactivated (409). UI handles with toast: "Cannot remove the last platform admin."

**Pagination**: server-side, 20 items per page, page-number navigation with ellipsis via `getVisiblePages()`.

**States**: loading skeleton (5 rows), empty "No users match your filters" with reset, error alert with retry, mutation errors via toast.

### Embedding Settings (`/platform-settings/embedding`)
`<EmbeddingConfigForm>` client component with:
- **Provider card list** via `<ProviderCard>`: each card shows provider name, model, dimensions, max tokens, endpoint URL, default/active badges, credentials status indicator
- **Add provider form**: name, provider selector, endpoint URL (validated via `isValidProviderEndpointURL`), model, dimensions, max tokens, "available to projects" checkbox
- **Per-provider actions** (via `DropdownMenu` on each card): Edit (inline form), Delete (confirmation dialog), Set as Default, Test Connection
- **Test connection**: calls `providers.testEmbeddingConnectivity`, shows success/failure inline on the card
- tRPC procedures: `platformEmbedding.list`, `platformEmbedding.create`, `platformEmbedding.update`, `platformEmbedding.delete`, `platformEmbedding.setDefault`

### LLM Settings (`/platform-settings/llm`)
`<LLMProviderList>` client component following the same card pattern as embedding:
- **Provider card list** via shared `<ProviderCard>`: provider name, model, endpoint URL, default/active badges, credentials status
- **Add provider form**: name, provider selector, endpoint URL, model, "available to projects" checkbox
- **Per-provider actions**: Edit, Delete, Set as Default, Test Connection
- **Test connection**: calls `providers.testLLMConnectivity`
- tRPC procedures: `platformLLM.list`, `platformLLM.create`, `platformLLM.update`, `platformLLM.delete`, `platformLLM.setDefault`

### Worker Status (`/platform-settings/workers`)
`<WorkersContent>` client component with:
- **Header**: "Workers" title + description, manual "Refresh" button (with spinning icon during fetch)
- **Worker table** via `<WorkerTable>`:

| Column | Content |
|---|---|
| Worker ID | Monospace text |
| Status | `<WorkerStatusBadge>` (starting/idle/busy/draining/stopped with color coding, drain reason tooltip) |
| Hostname | Worker hostname |
| Version | Worker version |
| Workflows | `<WorkflowBadges>` showing supported workflow types (full-index, incremental-index, code-analysis, etc.) |
| Current Job | Job ID if busy, empty otherwise |
| Uptime | Formatted via `formatUptime()` |
| Last Heartbeat | Formatted via `formatHeartbeat()` |

- **Read-only**: no mutations, display only
- **Redis error handling**: detects Redis connection errors in query response and shows contextual error message
- **Empty state**: `Activity` icon, "No workers are currently registered"
- tRPC procedure: `platformWorkers.list`

### Shared Provider Card Component
`<ProviderCard>` in `src/components/platform-settings/provider-card.tsx` is shared between embedding and LLM pages:
- Card layout with header (name, badges) and content (model, endpoint, dimensions)
- Actions dropdown: Edit, Delete (with confirmation dialog), Set as Default, Test Connection
- Inline edit mode with form fields and save/cancel
- Test result display (success checkmark / failure X with error message)
- Loading and error states

### tRPC Routers

| Router | Procedures |
|---|---|
| `platformUsers` | `list`, `updateStatus`, `grantRole`, `revokeRole` |
| `platformEmbedding` | `list`, `create`, `update`, `delete`, `setDefault` |
| `platformLLM` | `list`, `create`, `update`, `delete`, `setDefault` |
| `platformWorkers` | `list` |
| `providers` | `testEmbeddingConnectivity`, `testLLMConnectivity` |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/platform-settings/layout.tsx` | Platform settings layout with admin role gate and left sidebar nav |
| `src/app/(app)/platform-settings/users/page.tsx` | Server page rendering UserManagementContent |
| `src/components/platform-settings/user-management-content.tsx` | User list: search, status/role filters, pagination |
| `src/components/platform-settings/user-table.tsx` | User table with avatar, role badges, status badges |
| `src/components/platform-settings/user-actions.tsx` | Per-user actions dropdown with confirmation dialogs and self-protection |
| `src/app/(app)/platform-settings/embedding/page.tsx` | Server page rendering EmbeddingConfigForm |
| `src/components/platform-settings/embedding-config-form.tsx` | Embedding provider CRUD with add form and card list |
| `src/app/(app)/platform-settings/llm/page.tsx` | Server page rendering LLMProviderList |
| `src/components/platform-settings/llm-provider-list.tsx` | LLM provider CRUD with add form and card list |
| `src/components/platform-settings/provider-card.tsx` | Shared provider card: display, edit, delete, test connection |
| `src/app/(app)/platform-settings/workers/page.tsx` | Server page rendering WorkersContent |
| `src/components/platform-settings/workers-content.tsx` | Worker status with refresh button and Redis error handling |
| `src/components/platform-settings/worker-table.tsx` | Worker table with all status columns |
| `src/components/platform-settings/worker-status-badge.tsx` | Color-coded worker status badge with drain reason tooltip |
| `src/components/platform-settings/workflow-badges.tsx` | Supported workflow type badges |
| `src/server/api/routers/platform-users.ts` | User management tRPC router |
| `src/server/api/routers/platform-embedding.ts` | Embedding provider tRPC router |
| `src/server/api/routers/platform-llm.ts` | LLM provider tRPC router |
| `src/server/api/routers/platform-workers.ts` | Worker status tRPC router |
| `src/server/api/routers/providers.ts` | Provider connectivity test router |

## Acceptance Criteria
- [x] Platform settings layout gated by `platform_admin` role; non-admins redirected to dashboard
- [x] User management: paginated table with avatar, name, email, role badge, status badge, join date
- [x] User search debounced at 300 ms filtering by username/display name/email
- [x] Role and status filter buttons work correctly and combine with search
- [x] Grant/revoke admin actions with confirmation dialogs
- [x] Deactivate user with confirmation dialog; activate without confirmation
- [x] Self-protection: current user cannot deactivate self or revoke own admin role
- [x] Last admin protection: 409 error handled with toast message
- [x] Embedding settings: multi-provider card list with add/edit/delete/set-default/test-connection
- [x] LLM settings: same card pattern as embedding with provider-specific fields
- [x] Provider endpoint URL validated before submission
- [x] Test connection shows inline success/failure result on provider card
- [x] Worker status: read-only table with status badges, workflow badges, uptime, heartbeat
- [x] Worker refresh button with spinning icon animation during fetch
- [x] Redis error detection with contextual error message
- [x] Loading, empty, and error states render correctly on all four sub-pages
- [x] Mutation errors surfaced via toast notifications
