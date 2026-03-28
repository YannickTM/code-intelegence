# 17 — Description Workflow Handler (Full & Single-File)

## Status
Pending

## Goal
Implement the core description workflow handler for `describe-full` and `describe-file` scopes: per-file context assembly from PostgreSQL, prompt construction, LLM generation with structured JSON parsing and retry, Qdrant embedding, and persistence. Register handler with dispatcher.

## Depends On
15-llm-client, 16-description-schema

## Scope

### Handler Structure (`internal/workflow/describe/handler.go`)

The `Handler` implements `workflow.Handler` and is registered under three workflow names: `describe-full`, `describe-incremental`, and `describe-file`. This ticket covers the `describe-full` and `describe-file` scopes; incremental is handled in ticket 18.

```go
type Handler struct {
    repo       jobRepo
    queries    db.Querier
    vectorDB   vectorstore.VectorStore
    descWriter *description.Writer
    cfg        config.DescribeConfig
}
```

The handler does NOT receive a pre-built `Completer` or `Embedder` at construction time. Instead, it builds per-job instances from the `execution.Context` config (same pattern as `fullindex.Handler` building `embedding.NewOllamaClient`).

### Handle Flow

**Pre-claim phase** (errors returned for queue retry):

1. `LoadExecutionContext(jobID)` — now includes `LLMConfig` and `TargetFilePath`
2. Validate LLM config is present (fail if `LLM.ID` is zero — LLM is required for describe jobs)
3. `TryProjectLock(projectID)` — advisory lock
4. `ClaimJob(jobID, workerID)` — queued → running

**Post-claim phase** (errors mark job failed, return nil):

5. Resolve active snapshot for project branch; fail with `precondition_failed` if none
6. Build per-job LLM completer: `llmclient.NewOllamaCompleter(execCtx.LLM.EndpointURL, execCtx.LLM.Model, cfg.PerFileTimeout)`
7. Build per-job embedder: `embedding.NewOllamaClient(execCtx.Embedding.EndpointURL, ...)`
8. **File selection:**
   - `describe-full`: all files in the active snapshot
   - `describe-file`: single file by `execCtx.TargetFilePath`
9. Process files with bounded concurrency
10. Mark job completed with detached context

### Per-File Context Assembly (`internal/workflow/describe/context.go`)

For each file, assemble the context bundle from PostgreSQL:

```go
type FileContext struct {
    FilePath     string
    Content      string   // truncated to cfg.MaxContentChars
    Language     string
    Symbols      []SymbolContext
    Imports      []ImportContext
    Exports      []ExportContext
    References   []ReferenceContext
    Consumers    []string
    FileFacts    map[string]interface{}
    LineCount    int32
    SizeBytes    int64
    FolderFiles  []string // sibling file paths
    PreviousDesc *string  // prior description if available
}
```

Queries used per file:
- File content + metadata from `files` / `file_contents`
- Symbols limited to `cfg.MaxSymbols` (exported first, then by line count)
- Imports from `dependencies`
- Exports from `exports`
- References from `symbol_references`
- Consumers via `ListConsumersByTargetPath` (reverse dependency lookup)

### Prompt Construction (`internal/workflow/describe/prompt.go`)

System message defines the role and output format. User message contains the assembled file context. The prompt version is tracked as a constant (e.g., `"v1"`) and stored in `generation_metadata`.

Prompt design:
- System message: "You are a code analysis assistant that produces structured file descriptions."
- User message: file context in structured format (path, language, content excerpt, symbols, imports, exports, consumers, file facts)
- Response format: JSON matching the description schema
- Includes the `file_role` taxonomy for consistent classification
- Token budget guidance: summaries under 50 words, descriptions under 200 words

### LLM Generation with Retry (`internal/workflow/describe/generate.go`)

```go
func (h *Handler) generateDescription(
    ctx context.Context,
    fileCtx FileContext,
    completer llmclient.Completer,
) (*description.FileDescription, *llmclient.CompletionMeta, error)
```

Retry strategy on parse failure:
1. First attempt: full prompt
2. First retry: append correction hint ("Your previous response was not valid JSON. Please respond with only a JSON object matching the schema.")
3. Second retry: simplified prompt requesting only `summary` and `file_role`
4. After 2 retries: skip file, record diagnostic with category `llm_parse_error`

### Concurrency Model

Worker pool with `cfg.Concurrency` goroutines processing files from a channel. Rate limiting via `time.Ticker` when `cfg.RateLimitMs > 0`. Each goroutine: assemble context → generate description → validate → persist → embed.

Progress reporting via `SetDescriptionsGenerated` every 50 files.

### Qdrant Embedding

For each described file, embed the description text using the project's embedding provider. The embedded text:

```
File: {file_path}
Language: {language}
Role: {file_role}
Summary: {summary}
Description: {description}
Key symbols: {comma-separated symbol names and roles}
```

Qdrant payload includes `chunk_type: "file_description"` to distinguish from code chunk vectors (which use values like `function`, `class`, `module_context`). Point ID is the `file_descriptions.id` UUID.

### Partial Failure Semantics

Individual file failures do not fail the job. Failed files are collected and persisted as structured JSONB in `error_details`. The job fails only if zero files were successfully described.

Error categories: `llm_unavailable`, `llm_rate_limit`, `llm_parse_error`, `llm_timeout`, `context_load`, `validation`, `persistence`.

### Dispatcher Registration (`internal/app/app.go`)

Register the handler under all three workflow names (`describe-full`, `describe-incremental`, `describe-file`). Add to `supportedWorkflows`. Build the handler with the same dependency injection pattern as `fullindex.NewHandler`.

## Key Files

| File/Package | Purpose |
|---|---|
| `backend-worker/internal/workflow/describe/handler.go` | Handler struct, Handle method, concurrency orchestration |
| `backend-worker/internal/workflow/describe/context.go` | Per-file context assembly from PostgreSQL |
| `backend-worker/internal/workflow/describe/prompt.go` | Prompt template construction, version tracking |
| `backend-worker/internal/workflow/describe/generate.go` | LLM call + retry logic + JSON parse |
| `backend-worker/internal/workflow/describe/handler_test.go` | Unit tests with mock dependencies |
| `backend-worker/internal/app/app.go` | Handler registration, dependency wiring |
| `backend-worker/internal/workflow/fullindex/handler.go` | Reference pattern for handler structure |

## Acceptance Criteria
- [ ] Handler implements `workflow.Handler` interface
- [ ] Pre-claim phase: load context, validate LLM config present, advisory lock, claim job
- [ ] Post-claim phase: errors mark job failed (return nil to prevent queue retry)
- [ ] `describe-full` processes all files in the active snapshot
- [ ] `describe-file` processes the single file identified by `target_file_path`
- [ ] Active snapshot existence validated; `precondition_failed` error if missing
- [ ] Per-file context assembled from PostgreSQL (content, symbols, imports, exports, references, consumers)
- [ ] File content truncated to `DESCRIBE_MAX_CONTENT_CHARS`; symbols limited to `DESCRIBE_MAX_SYMBOLS`
- [ ] Prompt includes file role taxonomy and JSON output format specification
- [ ] LLM parse retry: up to 2 retries with adjusted prompts on JSON parse failure
- [ ] Bounded concurrency via `DESCRIBE_CONCURRENCY` worker pool
- [ ] Rate limiting respected when `DESCRIBE_RATE_LIMIT_MS > 0`
- [ ] Descriptions persisted via `description.Writer.WriteDescription`
- [ ] Description text embedded and upserted to Qdrant with `chunk_type: "file_description"`
- [ ] Partial failure: individual file errors collected, job succeeds if at least one file described
- [ ] Progress updated via `SetDescriptionsGenerated` every 50 files
- [ ] Job completed/failed with detached context (10s timeout)
- [ ] Handler registered under `describe-full`, `describe-incremental`, `describe-file` in dispatcher
- [ ] `describe-incremental` falls back to full-describe behavior until ticket 18 adds incremental file selection
- [ ] `supportedWorkflows` updated to include the three describe workflow names
