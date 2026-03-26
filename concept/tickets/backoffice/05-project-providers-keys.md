# 05 — Project Provider Settings & API Keys

## Status
Done

## Goal
Built the per-project embedding and LLM provider configuration UI, and the project-scoped API key management. Provider settings allow admins to view the resolved config (project override or global default), create/update overrides, test connectivity, and reset to global defaults. API keys provide project-scoped tokens for CI/CD and programmatic access with create, list, and revoke operations.

## Depends On
03-projects

## Scope

### Embedding Settings Section

Card section in `/projects/[id]/settings` between the SSH Key and Members sections.

**Card Header**: title "Embedding Provider", description shows source indicator: "Using project override" / "Using global default" / "No configuration" variants.

**Admin Form** (4 fields):
- Provider (select dropdown from `EMBEDDING_PROVIDERS` constant, currently Ollama)
- Endpoint URL (text input, http/https validation)
- Model (text input)
- Dimensions (number input, 1-65536)

**Action Buttons**:
- "Save" — creates/updates project override via `projectEmbedding.put`. Disabled when pristine or pending.
- "Test Connection" — sends current form values as ad-hoc test via `projectEmbedding.test`. Result shown as inline alert (green CheckCircle2 on success, red XCircle on failure). Alert dismissed on any field change.
- "Reset to Global Default" — visible only when source is "project". Confirmation dialog, then `projectEmbedding.delete`. Ghost variant with RotateCcw icon.

**Read-Only View** (member role): config values as plain text, no action buttons.

**Form State**: initialized from `projectEmbedding.getResolved` query data. Dirty detection compares current values to resolved config. Saving when source is "global" creates a new project override.

### LLM Settings Section

Identical UI pattern to Embedding Settings, positioned below it in the settings page.

**Card Header**: title "LLM Provider", same source indicator pattern.

**Same form structure**: Provider, Endpoint URL, Model (no Dimensions field for LLM).

**Same actions**: Save (`projectLLM.put`), Test Connection (`projectLLM.test`), Reset to Global Default (`projectLLM.delete`).

**Same read-only view** for members.

### Available Providers

Both sections also call `*.getAvailable` procedures to populate the provider dropdown with backend-registered providers.

### Project API Keys Section

Card section in `/projects/[id]/settings`, visible only to admin/owner (hidden for member role).

**Section Header**: "API Keys" title, "Project-scoped keys for CI/CD and programmatic access" subtext, "Create Key" button.

**Keys Table** (shared `ApiKeyTable` component from `src/components/api-keys/`): Name, Key prefix (monospace), Role (read muted / write amber badge), Expires (relative time or "Never"), Last Used (relative time or "Never"), Created (relative time), Delete action (trash icon).

**Create Key Dialog** (shared `CreateKeyDialog`): name input, role radio (Read/Write, default Read), expiration presets (Never/30d/90d/1yr/Custom). Calls `projectKeys.create({ projectId, name, role, expires_at })`. Post-creation: shared `KeyReveal` component shows plaintext key once with warning banner and copy button.

**Delete Key Dialog** (shared `DeleteKeyDialog`): "Revoke API Key" confirmation. Calls `projectKeys.delete({ projectId, keyId })`.

Shared components (`ApiKeyTable`, `CreateKeyDialog`, `DeleteKeyDialog`, `KeyReveal`, `RoleBadge`) are parameterized and reused with personal API keys (Settings > API Keys).

### Danger Zone Section

Located at the bottom of the settings page.

**Leave Project**: available to all admin+ users. Confirmation dialog, calls remove-self mutation.

**Delete Project**: owner-only. Confirmation requires typing the project name. Calls `projects.delete({ projectId })`. Navigates to `/projects` on success. Non-owners see disabled button with tooltip.

### tRPC Procedures

| Procedure | Backend Endpoint | Purpose |
|---|---|---|
| `projectEmbedding.get` | `GET /v1/projects/{id}/settings/embedding` | Project embedding override |
| `projectEmbedding.put` | `PUT /v1/projects/{id}/settings/embedding` | Create/update embedding override |
| `projectEmbedding.delete` | `DELETE /v1/projects/{id}/settings/embedding` | Remove override (fall back to global) |
| `projectEmbedding.getResolved` | `GET /v1/projects/{id}/settings/embedding/resolved` | Resolved config (project or global) |
| `projectEmbedding.test` | `POST /v1/projects/{id}/settings/embedding/test` | Test connectivity (ad-hoc or resolved) |
| `projectEmbedding.getAvailable` | `GET /v1/projects/{id}/settings/embedding/available` | Available embedding providers |
| `projectLLM.get` | `GET /v1/projects/{id}/settings/llm` | Project LLM override |
| `projectLLM.put` | `PUT /v1/projects/{id}/settings/llm` | Create/update LLM override |
| `projectLLM.delete` | `DELETE /v1/projects/{id}/settings/llm` | Remove LLM override |
| `projectLLM.getResolved` | `GET /v1/projects/{id}/settings/llm/resolved` | Resolved LLM config |
| `projectLLM.test` | `POST /v1/projects/{id}/settings/llm/test` | Test LLM connectivity |
| `projectLLM.getAvailable` | `GET /v1/projects/{id}/settings/llm/available` | Available LLM providers |
| `projectKeys.list` | `GET /v1/projects/{id}/keys` | List project API keys |
| `projectKeys.create` | `POST /v1/projects/{id}/keys` | Create project API key |
| `projectKeys.delete` | `DELETE /v1/projects/{id}/keys/{keyId}` | Revoke (soft-delete) API key |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/projects/[id]/components/settings-embedding.tsx` | Embedding provider settings Card section |
| `src/app/(app)/projects/[id]/components/settings-llm.tsx` | LLM provider settings Card section |
| `src/app/(app)/projects/[id]/settings/page.tsx` | Settings page assembling all sections |
| `src/components/api-keys/api-key-table.tsx` | Shared API key table component |
| `src/components/api-keys/create-key-dialog.tsx` | Shared create key dialog |
| `src/components/api-keys/delete-key-dialog.tsx` | Shared delete key confirmation |
| `src/components/api-keys/key-reveal.tsx` | One-time plaintext key display |
| `src/server/api/routers/project-embedding.ts` | Embedding settings tRPC router |
| `src/server/api/routers/project-llm.ts` | LLM settings tRPC router |
| `src/server/api/routers/project-keys.ts` | Project API keys tRPC router |

## Acceptance Criteria
- [x] Embedding settings card shows resolved source indicator (project/global/none)
- [x] Admin form with Provider, Endpoint URL, Model, Dimensions fields
- [x] Save creates/updates project override; disabled when form is pristine
- [x] Test Connection sends ad-hoc config; inline result alert (green/red)
- [x] Test result dismissed on any field change
- [x] Reset to Global Default: confirmation dialog, deletes project override
- [x] Read-only view for member role (no form, no actions)
- [x] LLM settings card follows identical pattern (no Dimensions field)
- [x] Available providers populated from backend via `getAvailable` procedures
- [x] API keys section hidden for member role
- [x] Keys table shows name, prefix, role badge, expires, last used, created, delete
- [x] Create key dialog with name, role (read/write), expiration presets
- [x] Plaintext key shown once after creation with copy button and warning
- [x] Revoke key: confirmation dialog, soft-delete, key disappears from list
- [x] Shared API key components reused between project and personal keys
- [x] Danger zone: delete project requires name confirmation, owner-only
