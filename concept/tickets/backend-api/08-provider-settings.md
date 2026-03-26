# 08 — Embedding & LLM Provider Configuration

## Status
Done

## Goal
Built a pluggable provider configuration system for both embedding and LLM capabilities with two-tier resolution (project override -> global default), a shared provider registry, credential encryption, and full platform-level multi-config management. This enables projects to use platform-provided shared providers or bring their own, with Ollama as the phase 1 implementation.

## Depends On
- Ticket 04 (User Identity & Session)
- Ticket 05 (Project CRUD)
- Ticket 06 (Project Membership)

## Scope

### Provider Registry (`internal/providers/`)

A code-backed registry defines which providers are currently implemented. Phase 1 supports `ollama` for both embedding and LLM capabilities. The `GET /v1/settings/providers` endpoint exposes this to any authenticated user. Planned providers (aws, azure, google, openai, claude, openrouter, huggingface) are excluded from the response until implemented.

### Two-Tier Resolution

When a project needs an embedding or LLM config, the resolution chain is:

1. Active project custom config (`project_id IS NOT NULL, is_active = TRUE`)
2. Project-selected platform global config (`selected_embedding_global_config_id` / `selected_llm_global_config_id` on the `projects` table)
3. Active global default config (`project_id IS NULL, is_default = TRUE, is_active = TRUE`)
4. 404 if none exist

The `/resolved` endpoints return the effective config with a `source` field (`"custom"`, `"global"`, or `"global_default"`).

### Project-Level Endpoints (Embedding and LLM)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/projects/{id}/settings/{cap}` | member | Current setting mode and config |
| PUT | `/v1/projects/{id}/settings/{cap}` | admin+ | Set mode: `"global"` (select platform config) or `"custom"` (own config) |
| DELETE | `/v1/projects/{id}/settings/{cap}` | admin+ | Reset to default mode |
| POST | `/v1/projects/{id}/settings/{cap}/test` | admin+ | Connectivity test (ad-hoc or resolved) |
| GET | `/v1/projects/{id}/settings/{cap}/resolved` | member | Effective config after resolution |
| GET | `/v1/projects/{id}/settings/{cap}/available` | member | List selectable platform global configs |

Where `{cap}` is `embedding` or `llm`.

### Platform-Level Endpoints (RequirePlatformAdmin)

Single-default CRUD (Task 30):

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/platform-management/settings/{cap}` | platform_admin | List all global configs |
| PUT | `/v1/platform-management/settings/{cap}` | platform_admin | Create/update global default (immutable-append) |
| POST | `/v1/platform-management/settings/{cap}/test` | platform_admin | Test global default or ad-hoc config |

Multi-config management (Task 32):

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/v1/platform-management/settings/{cap}` | platform_admin | Create additional non-default provider |
| PATCH | `/v1/platform-management/settings/{cap}/{configId}` | platform_admin | Update specific provider fields |
| DELETE | `/v1/platform-management/settings/{cap}/{configId}` | platform_admin | Deactivate non-default provider |
| POST | `/v1/platform-management/settings/{cap}/{configId}/promote` | platform_admin | Promote to default (atomic) |
| POST | `/v1/platform-management/settings/{cap}/{configId}/test` | platform_admin | Test specific provider connectivity |

### Connectivity Testing

Ollama testing calls `GET {endpoint_url}/api/tags` and verifies the model exists in the response. For LLM configs without a model, only endpoint reachability is checked. Tests have a 5-second timeout. Error messages sanitize URLs to strip userinfo and raw parse errors.

### Credential Handling

Credentials are accepted as a JSON object in request payloads, encrypted before storage in `credentials_encrypted` (BYTEA), and never returned in responses. Only `has_credentials: true/false` is exposed. Omitting credentials on update preserves existing encrypted credentials; sending `{}` or `null` clears them. Global credentials are platform-owned; project custom credentials are project-owned.

### Schema

Two provider config tables: `embedding_provider_configs` and `llm_provider_configs`. Both use `project_id IS NULL` for global configs, `project_id IS NOT NULL` for project-owned custom configs. Partial unique indexes enforce one default per capability globally and one active config per project per capability. A CHECK constraint prevents project-owned rows from having `is_default` or `is_available_to_projects` set.

### Immutable-Append Pattern

Global default updates and project custom config updates follow immutable-append: within a transaction, deactivate the current config, insert a new row with `is_active = TRUE`. For embedding configs, a corresponding `embedding_versions` row is created with a collision-resistant version label (`{provider}-{model}-{hex}`). Non-default global configs use in-place PATCH updates to preserve project foreign key references.

### Bootstrap

Default global configs for embedding (`jina/jina-embeddings-v2-base-en`, 768 dimensions) and LLM (Ollama, no model) are seeded at startup pointing to `http://host.docker.internal:11434`. Seeding is idempotent and does not overwrite existing configs.

## Key Files

| File | Purpose |
|------|---------|
| `backend-api/internal/providers/registry.go` | Shared provider registry with capability mapping |
| `backend-api/internal/embedding/service.go` | Embedding config resolution, CRUD, connectivity testing |
| `backend-api/internal/llm/service.go` | LLM config resolution, CRUD, connectivity testing |
| `backend-api/internal/handler/embedding.go` | Embedding handlers (global + project + multi-config) |
| `backend-api/internal/handler/llm.go` | LLM handlers (mirrors embedding) |
| `backend-api/internal/handler/provider.go` | `HandleListProviders` for supported provider types |
| `datastore/postgres/queries/keys_and_settings.sql` | Provider config queries (global, project, available, resolved) |

## Acceptance Criteria
- [x] `GET /v1/settings/providers` returns currently supported provider types (embedding: ollama, llm: ollama)
- [x] Project with no setting resolves to seeded global default
- [x] Project admin can select a platform global config (mode: "global")
- [x] Project admin can create custom project-owned config (mode: "custom")
- [x] DELETE resets project to default mode
- [x] `/resolved` returns correct source ("custom", "global", or "global_default")
- [x] Global credentials never exposed in any response; only `has_credentials` returned
- [x] Platform admin can list, create, update, delete, and promote global configs
- [x] Cannot delete the default provider (400); cannot delete a provider in use by projects (409)
- [x] Promote operation is atomic (single transaction)
- [x] Connectivity test with 5-second timeout; messages sanitize URLs
- [x] Immutable-append for default updates; in-place PATCH for non-default configs
- [x] Embedding version rows created on config changes
- [x] Endpoint URL validation rejects non-http(s) schemes and userinfo
- [x] Bootstrap seeds default configs idempotently on startup
