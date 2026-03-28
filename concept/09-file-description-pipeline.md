# File Description Pipeline

## 1. Role

The File Description Pipeline is a post-indexing stage that uses LLM analysis to generate structured, human-readable descriptions of source files. It consumes the indexed codebase data (symbols, imports, exports, references, file metadata) produced by the indexing pipeline and produces per-file description artifacts stored in PostgreSQL and embedded into Qdrant for semantic search.

### Goal

Transform structural indexing data into semantic understanding. The indexing pipeline answers "what exists in this file?" (symbols, imports, chunks). The description pipeline answers "what does this file do, why does it exist, and how does it relate to the rest of the codebase?"

### Responsibilities

- Select files for description based on scope (full codebase, incremental, single file)
- Assemble per-file context from indexed artifacts (symbols, imports, exports, references, consumers, file metadata)
- Generate structured descriptions via the project's configured LLM provider
- Validate LLM output against a defined schema
- Persist descriptions to PostgreSQL as first-class artifacts tied to index snapshots
- Embed descriptions into Qdrant for semantic retrieval
- Track generation metadata (model, version, token usage, timing) for cost and quality monitoring

## 2. Job Types

All three job types use the same description pipeline logic. Only the file selection strategy changes. The pipeline is scope-driven, not mode-driven: one shared processing implementation, one shared schema, one shared prompt and validation logic. The only difference is which files are selected for processing.

### Full Codebase (`describe-full`)

Describes all files in the active index snapshot for a project branch.

Use case: initial generation, full refresh after major refactors, rebuilding all descriptions from a clean state.

File selection: all file records in the active `index_snapshot`.

### Incremental (`describe-incremental`)

Describes only files affected by a recent indexing change.

Use case: routine updates after ongoing development, efficient refresh of description data.

File selection: union of changed, new, and dependency-affected files (see section 8).

### Single File (`describe-file`)

Describes one specific file using the same pipeline, prompt structure, validation, and output schema as the other modes.

Use case: targeted analysis, debugging description quality, ad hoc regeneration for one file.

File selection: one file identified by `target_file_path` on the job row.

### Job Routing

The `job_type` field in `indexing_jobs` is extended with three new values:

| `job_type` | Workflow name | Scope |
|---|---|---|
| `describe-full` | `describe-full` | All files in snapshot |
| `describe-incremental` | `describe-incremental` | Changed + affected files |
| `describe-file` | `describe-file` | Single file by path |

Queue payload follows the existing pattern (ADR-006):

```json
{
  "job_id": "uuid",
  "workflow": "describe-full",
  "enqueued_at": "2026-03-28T10:00:00Z",
  "project_id": "uuid",
  "requested_by": "user:uuid"
}
```

For `describe-file`, the target file path is stored on the job row (not in the queue payload) and loaded at execution time via `LoadExecutionContext`.

## 3. Worker Input (Per File)

The description generator receives a structured context bundle per file. This is assembled from existing indexed data — no re-parsing or re-cloning is needed. The description pipeline is a retrieval-oriented workflow: all input comes from PostgreSQL.

### Required Context

| Field | Source |
|---|---|
| `file_path` | `files.file_path` |
| `full_content` | `file_contents.content` (via `file_content_id`) |
| `language` | `files.language` |
| `symbols` | `symbols` table: name, kind, signature, doc_text, modifiers, flags |
| `imports` | `dependencies` table: import_name, import_type, target_file_path |
| `exports` | `exports` table: export_kind, exported_name |
| `references` | `symbol_references` table: reference_kind, target_name, confidence |
| `consumers` | Reverse lookup on `dependencies`: files where `target_file_path` = this file |
| `file_facts` | `files.file_facts` JSONB (HasJsx, HasTests, HasConfigPatterns, etc.) |
| `file_metadata` | `files.line_count`, `files.size_bytes` |

### Recommended Context

| Field | Source | Purpose |
|---|---|---|
| `folder_context` | Sibling file paths in the same directory | Module/package context |
| `dependency_graph_snippet` | 1-hop import/consumer graph | Architectural positioning |
| `previous_description` | `file_descriptions` table (prior run) | Stability and delta awareness |
| `change_metadata` | `commit_file_diffs` for incremental | What changed and why |
| `project_context` | Project name, repo URL, language distribution | Repository-level framing |

### Context Size Management

File content and symbol data can be large. The context assembler must respect the LLM's context window:

- Truncate file content to a configurable character limit (default: 12,000 chars)
- Limit symbols to the top N by significance (exported first, then by line count)
- Summarize import/consumer lists when they exceed a threshold
- Include previous description only when it exists and the file changed

## 4. Worker Output (Per File)

Each file produces a structured description object persisted to PostgreSQL.

### Description Schema

```json
{
  "file_path": "internal/handler/auth.go",
  "language": "go",
  "file_role": "http_handler",
  "summary": "HTTP handler for user authentication endpoints including login, logout, and session validation.",
  "description": "This file implements the auth HTTP handler for the backend API. It exposes three endpoints: POST /login creates a new session from GitHub OAuth credentials, POST /logout invalidates the active session, and GET /me returns the current user profile. The handler delegates credential validation to the auth service and session management to the session store. It follows the chi middleware pattern used across all handlers in this package.",
  "key_symbols": [
    {
      "name": "HandleLogin",
      "kind": "function",
      "role": "Processes login requests and creates sessions"
    },
    {
      "name": "HandleLogout",
      "kind": "function",
      "role": "Invalidates the caller's active session"
    }
  ],
  "imports_summary": "Depends on auth service, session store, domain models, and chi router.",
  "consumers_summary": "Consumed by the route registration in app/routes.go.",
  "architectural_notes": "Part of the handler layer in the backend-api service layout. Follows the same interface-based dependency injection pattern as other handlers.",
  "confidence": "high",
  "uncertainty_notes": null,
  "generation_metadata": {
    "model": "ollama/llama3.1",
    "llm_provider_config_id": "uuid",
    "prompt_version": "v1",
    "input_tokens": 2400,
    "output_tokens": 350,
    "latency_ms": 1200,
    "generated_at": "2026-03-28T10:15:00Z",
    "job_id": "uuid"
  }
}
```

### File Role Taxonomy

The `file_role` field uses a controlled vocabulary:

| Role | Description |
|---|---|
| `http_handler` | HTTP request handler |
| `service` | Business logic service |
| `repository` | Data access layer |
| `model` | Data model / type definitions |
| `middleware` | Request/response middleware |
| `config` | Configuration loading |
| `utility` | Shared utility functions |
| `test` | Test file |
| `migration` | Database migration |
| `entrypoint` | Application entry point (main) |
| `router` | Route registration |
| `client` | External service client |
| `types` | Type/interface definitions |
| `constants` | Constants and enums |
| `component` | UI component |
| `hook` | React/framework hook |
| `style` | Stylesheet |
| `script` | Build/deploy script |
| `documentation` | Documentation file |
| `other` | Does not fit established roles |

The LLM is instructed to select from this list. Unknown roles default to `other`.

## 5. Pipeline Flow

### Step-by-Step Processing

```
1. Job dispatch
   ├── API creates indexing_jobs row (job_type = describe-*)
   ├── API enqueues workflow task to Redis
   └── Worker dequeues and dispatches to description handler

2. Context load
   ├── Load execution context (project, LLM config)
   ├── Acquire project advisory lock
   ├── Claim job (queued → running)
   └── Resolve active index snapshot for branch

3. File selection
   ├── describe-full: load all file records from active snapshot
   ├── describe-incremental: compute affected file set (see section 8)
   └── describe-file: load single file record by path

4. Context assembly (per file)
   ├── Load file content from file_contents table
   ├── Load symbols, imports, exports, references for file
   ├── Compute consumer list (reverse dependency lookup)
   ├── Load file_facts and metadata
   ├── Optionally load previous description
   └── Build prompt from assembled context

5. LLM generation (per file, batched by concurrency limit)
   ├── Send prompt to LLM provider
   ├── Parse structured JSON response
   ├── Validate against description schema
   └── Retry on parse failure (up to 2 retries with adjusted prompt)

6. Persistence
   ├── Upsert file_descriptions row per file
   ├── Embed description text and upsert to Qdrant
   └── Update job progress counters

7. Completion
   ├── Mark job completed
   └── Release advisory lock
```

### Concurrency Model

LLM calls are the bottleneck. The pipeline uses bounded concurrency:

- Default: 4 concurrent LLM calls per job (configurable via `DESCRIBE_CONCURRENCY`)
- Each call is independent (no cross-file ordering dependency)
- Rate limiting: configurable inter-request delay to avoid overwhelming the LLM provider (default: 0ms, configurable via `DESCRIBE_RATE_LIMIT_MS`)
- Progress updates every 50 files

### No Repository Clone Required

The description pipeline reads all input from PostgreSQL. No git clone, no filesystem access. This makes description jobs lightweight and fast to start compared to indexing jobs.

## 6. Database Schema

### New Table: `file_descriptions`

```sql
CREATE TABLE file_descriptions (
  id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  index_snapshot_id UUID      NOT NULL REFERENCES index_snapshots(id) ON DELETE CASCADE,
  file_id         UUID        NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  file_path       TEXT        NOT NULL,
  language        TEXT,
  file_role       TEXT,
  summary         TEXT        NOT NULL,
  description     TEXT        NOT NULL,
  key_symbols     JSONB       NOT NULL DEFAULT '[]'::jsonb,
  imports_summary TEXT,
  consumers_summary TEXT,
  architectural_notes TEXT,
  confidence      TEXT        NOT NULL DEFAULT 'medium',
  uncertainty_notes TEXT,
  generation_metadata JSONB   NOT NULL DEFAULT '{}'::jsonb,
  content_hash    TEXT        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  UNIQUE(project_id, index_snapshot_id, file_path)
);

CREATE INDEX idx_file_descriptions_snapshot
  ON file_descriptions(index_snapshot_id);

CREATE INDEX idx_file_descriptions_project_path
  ON file_descriptions(project_id, file_path);

CREATE INDEX idx_file_descriptions_role
  ON file_descriptions(project_id, index_snapshot_id, file_role);
```

Design notes:

- `content_hash` is SHA-256 of the concatenated input context (file content hash + symbols hash + imports hash + exports hash + consumers hash). Used for incremental skip: if the input hash has not changed, the existing description is still valid.
- `generation_metadata` JSONB stores model, token counts, latency, prompt version, and the `llm_provider_config_id` used. This provides an audit trail without adding columns for every metadata field.
- Descriptions are scoped to `index_snapshot_id`, tying them to the exact indexed state that produced them. When a new snapshot is activated, descriptions from the old snapshot remain available until regenerated or garbage-collected.

### Extend `indexing_jobs`

Per ADR-009, the existing `indexing_jobs` table is reused rather than creating a separate table. The new job types (`describe-full`, `describe-incremental`, `describe-file`) are added as valid `job_type` values.

```sql
ALTER TABLE indexing_jobs
  ADD COLUMN target_file_path TEXT;

ALTER TABLE indexing_jobs
  ADD COLUMN descriptions_generated INT NOT NULL DEFAULT 0;
```

`target_file_path` is NULL for all existing job types and for `describe-full` / `describe-incremental`. It is set only for `describe-file` jobs.

### Description Versioning: Snapshot-Tied

Descriptions are tied to `index_snapshot_id`, not independently versioned.

Rationale:

- Descriptions consume indexed data from a specific snapshot. If the index changes, the description's input changed.
- Independent versioning would require tracking which indexed state produced which description, duplicating the snapshot concept.
- When a new index snapshot is activated, the prior snapshot's descriptions remain queryable via explicit snapshot ID. New descriptions are generated against the new snapshot.
- The `content_hash` field provides a secondary staleness check: even within the same snapshot, if the LLM model or prompt version changes, descriptions can be selectively regenerated.

## 7. LLM Integration

### Provider Resolution

The description pipeline uses the project's resolved LLM provider config, following the same two-tier pattern as embedding (ADR-022):

1. Active project-owned LLM config → use it
2. Explicit global LLM config selection on project → use it
3. Fall through to default global LLM config

The resolved config ID is stored on the `indexing_jobs.llm_provider_config_id` column at job creation time (this column already exists in the schema).

### Execution Context Extension

The worker's `execution.Context` is extended with an `LLMConfig` field:

```go
type LLMConfig struct {
    ID          pgtype.UUID
    Provider    string
    EndpointURL string
    Model       string
    Credentials []byte // decrypted
}
```

`LoadExecutionContext` loads the LLM config from `llm_provider_configs` using the job's `llm_provider_config_id`, mirroring how it already loads the embedding config.

### Prompt Structure

The prompt is assembled from a versioned template with injected file context. Prompt versioning is tracked in `generation_metadata.prompt_version` for reproducibility.

Prompt design principles:

- System message defines the role: "You are a code analysis assistant that produces structured file descriptions."
- User message contains the file context in a structured format (path, language, content excerpt, symbols, imports, exports, consumers, file facts)
- Response format is specified as JSON matching the description schema
- The prompt explicitly lists the `file_role` taxonomy for consistent classification
- Token budget guidance: instruct the LLM to keep summaries under 50 words and descriptions under 200 words

### LLM Client

A new `llmclient` package in the worker provides a provider-agnostic completion interface:

```go
type Completer interface {
    Complete(ctx context.Context, messages []Message) (string, CompletionMeta, error)
}

type Message struct {
    Role    string // "system", "user", "assistant"
    Content string
}

type CompletionMeta struct {
    InputTokens  int
    OutputTokens int
    LatencyMs    int64
    Model        string
}
```

Phase 1 implements an Ollama-backed completer using the `/api/chat` endpoint. The interface supports future OpenAI, Anthropic, and other provider implementations.

### Rate Limiting and Cost Control

- Per-job token budget: configurable maximum total tokens per job (default: unlimited). When exceeded, the job completes with partial results and a warning.
- Per-file timeout: configurable per-LLM-call timeout (default: 60s). Files that time out are skipped with a diagnostic.
- Request spacing: configurable minimum delay between LLM calls (default: 0ms). Prevents overwhelming self-hosted LLM servers.
- Cost estimation: `generation_metadata` captures per-file token usage. Aggregate cost can be computed from job-level totals.

## 8. Incremental Reprocessing Rules

The `describe-incremental` workflow must determine which files need new or updated descriptions. This goes beyond "file content changed" because a file's description depends on its relationships.

### Change Detection Strategy

A file requires re-description when any of the following conditions is true:

**Direct changes:**
1. File content hash differs between the current and previous description's `content_hash`
2. File is new (no existing description in any snapshot for this project+path)

**Dependency-driven changes:**
3. Any file imported by this file (`dependencies.target_file_path`) has a changed content hash
4. Any file imported by this file changed its public exports (`exports` table diff)
5. Any file that imports this file (`consumers`) was added or removed

### Implementation

```
1. Load active snapshot's file records
2. Load previous descriptions (from prior snapshot or same snapshot)
3. For each file in the active snapshot:
   a. Compute input context hash
   b. Compare against stored content_hash on existing description
   c. If hash matches → skip (description is still valid)
   d. If hash differs or no description exists → mark for re-description
4. Additionally, check dependency-driven changes:
   a. Build a set of files whose exports changed
   b. For each file that imports a changed-exports file → mark for re-description
   c. Build a set of files with changed consumer lists
   d. Mark those files for re-description
5. Process the union of all marked files through the description pipeline
```

The `content_hash` provides a fast skip path. It is computed from the concatenation of: file content hash + symbols hash + imports hash + exports hash + consumers hash. If none of these changed, the description is still valid regardless of other snapshot changes.

### Copy-Forward

Descriptions for unchanged files are copied forward to the new snapshot, similar to how the incremental indexing pipeline copies forward unchanged artifacts. This ensures the active snapshot always has a complete set of descriptions.

```sql
INSERT INTO file_descriptions (
  project_id, index_snapshot_id, file_id, file_path, language,
  file_role, summary, description, key_symbols,
  imports_summary, consumers_summary, architectural_notes,
  confidence, uncertainty_notes, generation_metadata, content_hash
)
SELECT
  fd.project_id, $new_snapshot_id, f_new.id, fd.file_path, fd.language,
  fd.file_role, fd.summary, fd.description, fd.key_symbols,
  fd.imports_summary, fd.consumers_summary, fd.architectural_notes,
  fd.confidence, fd.uncertainty_notes, fd.generation_metadata, fd.content_hash
FROM file_descriptions fd
JOIN files f_new ON f_new.index_snapshot_id = $new_snapshot_id
  AND f_new.file_path = fd.file_path
  AND f_new.project_id = fd.project_id
WHERE fd.index_snapshot_id = $old_snapshot_id
  AND fd.file_path NOT IN (/* files being re-described */);
```

## 9. API Surface

### Trigger Endpoint

Description jobs are triggered via the unified action endpoint (shared with indexing):

```
POST /v1/projects/{id}/action
```

Request body:

```json
{
  "type": "describe-full",
  "file_path": null
}
```

| `type` | Behavior |
|---|---|
| `full` | Creates a full indexing job |
| `incremental` | Creates an incremental indexing job |
| `describe-full` | Creates a `describe-full` job |
| `describe-incremental` | Creates a `describe-incremental` job |
| `describe-file` | Creates a `describe-file` job; requires `file_path` |

For describe types, the endpoint additionally validates that an LLM provider is configured (required) and that an active index snapshot exists (prerequisite). SSH key validation is skipped for describe types since they read from PostgreSQL, not git.

Response: the created job record (same shape as indexing job responses).

### Read Endpoints

```
GET /v1/projects/{id}/files/description?file_path=internal/handler/auth.go
```

Returns the file description for the specified path from the active snapshot.

```
GET /v1/projects/{id}/descriptions
```

Returns all file descriptions for the active snapshot. Supports pagination (`limit`, `offset`) and filtering (`file_role`, `language`, `confidence`).

```
GET /v1/projects/{id}/descriptions/summary
```

Returns aggregate statistics: total files described, role distribution, confidence distribution, generation cost summary.

### MCP Tool Extension

A new MCP tool `get_file_description` is added:

- Backend: `GET /v1/projects/{project_id}/files/description?file_path={path}`
- Required: `project_id`, `file_path`
- Returns: summary, description, file_role, key_symbols, architectural_notes

The existing `search_code` tool can optionally include file descriptions in search results when descriptions are available, enriching the context returned to AI agents.

## 10. Qdrant Integration

### Embedding File Descriptions

File descriptions are embedded and stored in Qdrant to enable semantic search over file purposes and roles, complementing the existing code chunk search. Embedding uses the project's embedding provider (not the LLM provider) to generate vectors from the description text. This ensures descriptions and code chunks live in the same vector space.

### What Gets Embedded

For each described file, embed a single text composed of:

```
File: {file_path}
Language: {language}
Role: {file_role}
Summary: {summary}
Description: {description}
Key symbols: {comma-separated symbol names and roles}
```

### Qdrant Payload

```json
{
  "project_id": "uuid",
  "index_snapshot_id": "uuid",
  "file_path": "internal/handler/auth.go",
  "language": "go",
  "file_role": "http_handler",
  "chunk_type": "file_description",
  "branch": "main",
  "git_commit": "a1b2c3d4",
  "embedding_version": "ollama-jina-embeddings-v2-base-en-768",
  "raw_text_ref": "db://file_descriptions/{description_id}"
}
```

The `chunk_type: "file_description"` field distinguishes description vectors from code chunk vectors (which use `chunk_type` values like `function`, `class`, `module_context`). The `search_code` query pipeline can filter by `chunk_type` to include or exclude descriptions.

### Lifecycle

Description vectors follow the same lifecycle as code chunk vectors:

- Upserted after description generation (same collection as code chunks)
- Updated when descriptions are regenerated
- Cleaned up when snapshots are superseded

## 11. Error Handling

### Partial Failure Semantics

The description pipeline uses partial-failure semantics, mirroring the parser's approach:

- Individual file description failures do not fail the entire job
- Failed files are recorded with error details in the job's `error_details` JSONB
- The job completes successfully if at least one file was described
- The job fails only if zero files could be described (total failure)

### Failure Categories

| Category | Examples | Retry? |
|---|---|---|
| `llm_unavailable` | LLM endpoint unreachable, connection refused | Yes (job-level retry via asynq) |
| `llm_rate_limit` | 429 response from LLM provider | Yes (with backoff) |
| `llm_parse_error` | LLM returned invalid JSON | Yes (per-file, up to 2 retries with adjusted prompt) |
| `llm_timeout` | LLM call exceeded per-file timeout | No (skip file, log diagnostic) |
| `context_load` | Failed to load file content or artifacts | No (skip file, log error) |
| `validation` | Description does not match schema | Yes (per-file, 1 retry) |
| `persistence` | PostgreSQL write failure | Yes (job-level retry) |

### LLM Parse Retry Strategy

When the LLM returns malformed JSON:

1. First retry: append "Your previous response was not valid JSON. Please respond with only a JSON object matching the schema." to the prompt
2. Second retry: simplify the prompt to request only summary and file_role (reduced schema)
3. After two retries: skip the file and record a diagnostic

### Job-Level Error Reporting

The `error_details` JSONB on `indexing_jobs` captures per-file errors:

```json
[
  {
    "category": "llm_parse_error",
    "message": "invalid JSON after 2 retries",
    "step": "generate_description",
    "file_path": "src/utils/legacy.js"
  }
]
```

## 12. Relationship to Indexing

### Dependency on Completed Indexing

The description pipeline requires a completed, active index snapshot. It reads from:

- `files` (file records)
- `file_contents` (raw source text)
- `symbols` (declarations)
- `dependencies` (imports)
- `exports` (file-level exports)
- `symbol_references` (call sites)

A description job validates that an active snapshot exists for the project branch before proceeding. If no active snapshot exists, the job fails immediately with category `precondition_failed`.

### Staleness When Indexing Reruns

When indexing produces a new snapshot:

1. The old snapshot's descriptions remain valid and queryable by explicit snapshot ID
2. The new snapshot has no descriptions until a description job runs
3. A `describe-incremental` job against the new snapshot can copy forward unchanged descriptions and regenerate only those for changed files

### Automatic Chaining (Optional)

The API can be configured to automatically trigger description generation after indexing:

| Setting | Default | Description |
|---|---|---|
| `AUTO_DESCRIBE_AFTER_INDEX` | `false` | Automatically enqueue a description job after indexing |
| `AUTO_DESCRIBE_SCOPE` | `incremental` | Scope for auto-triggered description jobs |

When enabled, the worker publishes a Redis event after successful indexing. The API listens for this event and creates the follow-up description job. This keeps the chaining logic in the API (job creator) rather than the worker (executor), consistent with ADR-005.

### Snapshot Association

```
index_snapshots (1) ──── (N) files
                    ──── (N) file_descriptions
                    ──── (N) code_chunks
                    ──── (N) symbols
```

Descriptions are first-class snapshot artifacts alongside files, symbols, and chunks. The snapshot is the consistency boundary: all descriptions within a snapshot were generated from the same indexed state.

## 13. Configuration

### New Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DESCRIBE_CONCURRENCY` | `4` | Concurrent LLM calls per description job |
| `DESCRIBE_RATE_LIMIT_MS` | `0` | Minimum delay between LLM calls (ms) |
| `DESCRIBE_PER_FILE_TIMEOUT` | `60s` | Timeout for a single LLM call |
| `DESCRIBE_MAX_CONTENT_CHARS` | `12000` | Max file content characters in prompt |
| `DESCRIBE_MAX_SYMBOLS` | `30` | Max symbols included in prompt context |
| `DESCRIBE_JOB_TIMEOUT` | `60m` | Overall job timeout (asynq) |
| `AUTO_DESCRIBE_AFTER_INDEX` | `false` | Auto-chain description after indexing |
| `AUTO_DESCRIBE_SCOPE` | `incremental` | Scope for auto-triggered descriptions |

### LLM Provider Config

No new provider configuration tables. The pipeline uses the existing `llm_provider_configs` table and the project's resolved LLM config. The `indexing_jobs.llm_provider_config_id` column (already present) records which config was used.

## 14. Service Layout

```text
backend-worker/
  internal/
    workflow/
      describe/
        handler.go              # Description workflow handler (all 3 scopes)
        context.go              # Per-file context assembly
        prompt.go               # Prompt template and construction
        generate.go             # LLM call + retry logic + JSON parse
        schema.go               # Output schema definition and validation
        incremental.go          # Change detection and copy-forward
    llmclient/
      client.go                 # Completer interface
      ollama.go                 # Ollama /api/chat implementation
    description/
      writer.go                 # PostgreSQL persistence for file_descriptions
```

The handler follows the same `workflow.Handler` interface as `fullindex.Handler` and `incremental.Handler`. It is registered with the dispatcher under three workflow names: `describe-full`, `describe-incremental`, `describe-file`.

## 15. ADR References

| ADR | Relevance |
|---|---|
| ADR-005 | API creates description jobs; worker executes them |
| ADR-006 | Queue payload is minimal; LLM config loaded at runtime |
| ADR-008 | Description is a retrieval-oriented workflow (no repo clone needed) |
| ADR-009 | Reuse `indexing_jobs` table for description job types |
| ADR-020 | Advisory locks prevent concurrent description + indexing on same project |
| ADR-022 | LLM provider resolved via same two-tier pattern as embedding |
