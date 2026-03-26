# Phase 2 — Multiple Embedding Providers

## Goal

Extend the embedding system from Ollama-only to a pluggable provider architecture that supports self-hosted and cloud-based embedding services. The `platform_admin` manages server-wide provider defaults; project admins can override per project.

## Current State

The provider architecture is partially implemented:

- **Provider registry pattern** is implemented in `backend-api/internal/providers/` with a capability-based design
- **Ollama provider** is shipped and functional for both embedding and connectivity testing
- **`provider_credentials` table** exists with encrypted credential storage (AES-256-GCM via `SSH_KEY_ENCRYPTION_SECRET`)
- **Connectivity testing** works — the API validates provider reachability and model availability before saving config
- **Two-tier config resolution** is implemented in `backend-api/internal/providersetting/` with shared helpers for credential handling, project-level locking, and validation
- **Embedding and LLM provider settings** share a common service pattern via `providersetting.ValidateBaseFields`, `providersetting.ResolveCredentials`, etc.
- **28 language grammars** are supported by the embedded go-tree-sitter parser in the worker

Remaining scope covers adding cloud providers and the backoffice management UI.

## Depends On

- Phase 1: Embedding settings with two-tier pattern (shipped)
- Phase 2: platform Admin role (`08-platform-admin.md`)

## Remaining Scope

- **OpenAI provider** implementation (embed endpoint, model listing, connectivity test)
- **Voyage AI provider** implementation (code-optimized embeddings)
- **Credential management UI** in backoffice (list, add, revoke, test credentials)
- **Per-project override UI** in backoffice (provider selector with "Use Default" option)
- **Provider info endpoint** for frontend to discover available providers and their requirements

## Provider Architecture

### Provider Interface

```go
// internal/embedding/provider.go

type EmbeddingProvider interface {
    // TestConnectivity verifies the provider is reachable and the model is available.
    TestConnectivity(ctx context.Context, cfg *domain.EmbeddingConfig) (*ConnectivityResult, error)

    // Embed generates embedding vectors for the given texts.
    // Returns one vector per input text.
    Embed(ctx context.Context, cfg *domain.EmbeddingConfig, texts []string) ([][]float32, error)

    // ListModels returns available models from the provider (optional, not all providers support this).
    ListModels(ctx context.Context, cfg *domain.EmbeddingConfig) ([]string, error)
}
```

### Provider Registry

The registry pattern is already implemented. Adding a new provider means:

1. Implementing the `EmbeddingProvider` interface
2. Registering it in the provider registry with its capability declaration

```go
// Current state: Ollama is registered
// Future additions:
// r.Register("openai", &OpenAIProvider{})
// r.Register("voyage", &VoyageProvider{})
```

The service layer uses the registry to dispatch to the correct provider based on the config's provider field.

### Provider Implementations

```text
internal/
  providers/
    registry.go          # provider registry + capability validation (implemented)
    ollama.go            # Ollama provider (implemented)
    openai.go            # OpenAI provider (future)
    voyage.go            # Voyage AI provider (future)
  providersetting/
    common.go            # shared helpers: credential resolution, validation, locking (implemented)
```

## Supported Providers

### Ollama (Implemented)

Self-hosted embedding via the Ollama API. No credentials needed.

| Field | Required | Notes |
|-------|----------|-------|
| `endpoint_url` | Yes | e.g. `http://ollama:11434` |
| `model` | Yes | e.g. `jina/jina-embeddings-v2-base-en`, `mxbai-embed-large` |
| `dimensions` | Yes | Model-dependent (768, 1024, etc.) |

Connectivity test: `GET {endpoint_url}/api/tags` → check model in list.

Embedding call: `POST {endpoint_url}/api/embed` with `{"model": "...", "input": [...]}`.

### OpenAI (Future)

Cloud-based embedding via the OpenAI API. Requires an API key.

| Field | Required | Notes |
|-------|----------|-------|
| `endpoint_url` | No | Defaults to `https://api.openai.com/v1`. Can be overridden for Azure OpenAI or compatible APIs. |
| `model` | Yes | e.g. `text-embedding-3-small`, `text-embedding-3-large` |
| `dimensions` | Yes | Model-dependent (1536, 3072, or custom with `text-embedding-3-*`) |

Connectivity test: `GET /v1/models` with API key → check model in list.

Embedding call: `POST /v1/embeddings` with `{"model": "...", "input": [...]}`.

Note: OpenAI's `text-embedding-3-*` models support a `dimensions` parameter for Matryoshka-style truncation, allowing flexible dimension choices without retraining.

### Voyage AI (Future)

Cloud-based embedding optimized for code. Requires an API key.

| Field | Required | Notes |
|-------|----------|-------|
| `endpoint_url` | No | Defaults to `https://api.voyageai.com/v1` |
| `model` | Yes | e.g. `voyage-code-3`, `voyage-3` |
| `dimensions` | Yes | Model-dependent (1024 for voyage-code-3) |

Connectivity test: `POST /v1/embeddings` with minimal input and API key → verify 200 response.

Embedding call: `POST /v1/embeddings` with `{"model": "...", "input": [...], "input_type": "document"}`.

Note: Voyage's code-specific models (`voyage-code-3`) are particularly relevant for this platform's use case.

### Future Providers

The interface is generic enough to support additional providers:

- **Cohere** (`embed-english-v3.0`, `embed-multilingual-v3.0`)
- **Local ONNX** (self-hosted model inference without Ollama)
- **Custom HTTP** (any OpenAI-compatible API endpoint)

Adding a new provider means implementing the `EmbeddingProvider` interface and registering it in the provider registry.

## Credential Management

Cloud providers (OpenAI, Voyage, etc.) require API keys. These are sensitive secrets that need encrypted storage.

### Provider Credentials Table (Implemented)

The `provider_credentials` table already exists with encrypted credential storage:

```sql
CREATE TABLE provider_credentials (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider TEXT NOT NULL,
  credential_type TEXT NOT NULL DEFAULT 'api_key',
  credential_encrypted BYTEA NOT NULL,
  label TEXT,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Design notes:

- Encryption uses the same `SSH_KEY_ENCRYPTION_SECRET` pattern as SSH private keys (AES-256-GCM)
- `credential_type` allows for future expansion (OAuth tokens, certificates)
- Credentials are global (not per-project) — a platform admin manages them
- The `providersetting` package resolves credentials at runtime via `ResolveCredentials`

### Credential Resolution (Implemented)

The two-tier resolution is implemented in `backend-api/internal/providersetting/common.go`:

```
1. Read provider config → get provider name and credential update
2. Resolve credentials: carry forward, clear, or encrypt new data
3. Store encrypted credential alongside the provider config
```

Project-level overrides can specify a different provider but share the same global credentials. If per-project credentials are needed later, add an optional `project_id` column to `provider_credentials` (same two-tier pattern).

### Credential API Endpoints (Future)

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/v1/admin/credentials` | `ListCredentials` | `platform_admin` |
| POST | `/v1/admin/credentials` | `CreateCredential` | `platform_admin` |
| DELETE | `/v1/admin/credentials/{id}` | `RevokeCredential` | `platform_admin` |
| POST | `/v1/admin/credentials/{id}/test` | `TestCredential` | `platform_admin` |

Credentials are never returned in plaintext — the GET endpoint returns `provider`, `credential_type`, `label`, `created_at`, and a masked preview (e.g. `sk-...xyz`).

## Embedding Config API Changes

### Updated Validation

Provider-specific validation is already dispatched through the registry. The `providersetting.ValidateBaseFields` function handles shared validation (name, provider, endpoint_url, settings), while capability-specific fields (model, dimensions) are validated separately per provider.

### Provider Info Endpoint (Future)

New read-only endpoint to help the frontend show provider options:

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/v1/settings/embedding/providers` | `ListProviders` | any authenticated user |

Response:
```json
{
  "providers": [
    {
      "name": "ollama",
      "label": "Ollama (Self-Hosted)",
      "requires_credentials": false,
      "requires_endpoint_url": true,
      "default_endpoint_url": null,
      "has_active_credential": false
    },
    {
      "name": "openai",
      "label": "OpenAI",
      "requires_credentials": true,
      "requires_endpoint_url": false,
      "default_endpoint_url": "https://api.openai.com/v1",
      "has_active_credential": true
    },
    {
      "name": "voyage",
      "label": "Voyage AI",
      "requires_credentials": true,
      "requires_endpoint_url": false,
      "default_endpoint_url": "https://api.voyageai.com/v1",
      "has_active_credential": false
    }
  ]
}
```

## Re-Indexing Impact

Changing the embedding provider or model means existing vectors are **incompatible** with new ones (ADR-004: never mix embedding versions). The system must handle this:

### On Provider/Model Change

When `PUT /v1/settings/embedding` or `PUT /v1/projects/{id}/settings/embedding` changes the provider or model:

1. A new `embedding_version` row is created (immutable append pattern, already exists)
2. Existing `index_snapshots` still reference the old `embedding_version`
3. New indexing jobs use the new embedding version → new Qdrant collection
4. The backoffice should prompt: "Embedding model changed. Re-index projects to use the new model?"
5. Until re-indexed, projects continue to serve results from the old embedding version

### Compatibility Check

The resolved config endpoint already returns the effective config. The indexing service compares the current `embedding_version` with the version used by the active `index_snapshot`:

```
if snapshot.embedding_version_id != current_embedding_version.id {
    // Snapshot is stale — needs re-indexing with new model
}
```

The backoffice shows a warning badge on projects where the active snapshot uses an outdated embedding version.

## Backoffice UI Changes (Future)

### Server Settings → Embedding (platform admin)

- Provider selector dropdown (Ollama, OpenAI, Voyage)
- Dynamic form fields based on selected provider
- Credential status indicator (configured / not configured) for cloud providers
- Link to credential management for cloud providers
- "Test Connection" button before saving

### Project Settings → Embedding (project admin+)

- Same provider selector as global, but with "Use Default" option
- Shows inherited global config when no override is set
- Clear "Override" / "Reset to Default" toggle

### Admin → Credentials (platform admin)

- List of configured provider credentials with masked previews
- Add/revoke credentials per provider
- Test credential connectivity

## Migration Plan

Steps 1-3 are already complete. Remaining work:

1. ~~Create `provider_credentials` table~~ (done)
2. ~~Refactor Ollama into provider interface implementation~~ (done)
3. ~~Create provider registry with Ollama registered~~ (done)
4. Add OpenAI provider implementation + register
5. Add Voyage provider implementation + register
6. Add credential management endpoints for platform admin
7. Add provider info endpoint
8. Update backoffice with provider selector and credential management
9. Integration tests:
    - Ollama provider works as before (covered)
    - OpenAI provider connects and embeds (mock or staging key)
    - Voyage provider connects and embeds
    - Provider change triggers new embedding version
    - Missing credentials return clear error
    - Credential test endpoint works per provider

## Open Questions

- Should we support per-project credentials (e.g. a team has their own OpenAI key with separate billing)?
- Should there be a cost estimation feature for cloud providers before triggering a full re-index?
- Should we support OpenAI-compatible APIs (e.g. vLLM, text-embeddings-inference) as a generic "custom" provider, or require explicit provider implementations?
- Should embedding dimension auto-detection be supported (query the model metadata instead of requiring manual entry)?
- What rate limiting / retry strategy should cloud provider implementations use?
