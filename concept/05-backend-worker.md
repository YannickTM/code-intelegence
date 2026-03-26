# Backend Worker

## 1. Role

`backend-worker` is the asynchronous workflow execution engine for the MYJUNGLE Code Intelligence Platform. It is a single Go binary that:

- Dequeues jobs from Redis/asynq
- Loads the project-scoped execution context from PostgreSQL
- Runs workflow handlers (full-index, incremental-index, commits)
- Persists results to PostgreSQL and Qdrant
- Contains the embedded parser (tree-sitter + extractors)

The worker does not expose public HTTP endpoints. Its operational state is surfaced through PostgreSQL job state, Redis pub/sub events, and the worker registry.

## 2. Embedded Parser

### Why embedded

- **Simpler deployment**: one container instead of two; no gRPC service discovery or health-check wiring.
- **No network boundary**: parsed results stay in-process; no serialization overhead.
- **Parser scales with workers**: adding a worker replica automatically adds parser capacity.
- **Shared filesystem**: the parser reads files from the same container filesystem as the workspace, requiring no file-sharing mechanism.

### Technology

The parser uses `smacker/go-tree-sitter`, which provides Go bindings to the tree-sitter C runtime. Each supported language has a compiled grammar loaded via the registry. The engine processes files through a pipeline: normalize content, detect language, parse with tree-sitter, run extractors by tier, compute file facts, and emit metadata.

### Parser pool

`sitter.Parser` is not goroutine-safe. The pool (`internal/parser/pool.go`) maintains a bounded set of pre-allocated parsers behind a buffered channel. Callers acquire a parser, set the language, parse, and return it.

- Pool size: `PARSER_POOL_SIZE` environment variable (default: `runtime.NumCPU()`)
- Parsers are created at startup and reused across requests
- Acquire respects context cancellation and pool shutdown
- After each parse, `parser.Reset()` clears incremental-parse state

### Timeouts and limits

| Setting | Environment variable | Default |
|---|---|---|
| Per-file parse timeout | `PARSER_TIMEOUT_PER_FILE` | 30s |
| Batch timeout | `PARSER_TIMEOUT` | 5m |
| Max file size | `PARSER_MAX_FILE_SIZE` | 10 MB |

Files exceeding the max size are skipped with an oversized-file diagnostic. The per-file timeout uses `context.WithTimeout` around the tree-sitter `ParseCtx` call; timed-out files produce a parse-timeout diagnostic and proceed without an AST (chunks and file-meta extractors still run in text-only mode).

### Concurrency model

`ParseFilesBatched` processes a batch of files concurrently using `errgroup` bounded by the pool size. Individual file errors are captured as `Issues` in the result (partial-failure semantics); the batch never returns an error for a single file failure.

### Determinism

Given identical `(file_path, language, content)`, the parser produces stable output: same symbol ordering, same chunk boundaries, same hash values. Determinism is critical for incremental indexing so unchanged files do not churn vectors or metadata rows.

## 3. Language Support

The parser supports 28 languages in 3 tiers, defined in `internal/parser/registry/languages.go`.

### Tier 1 -- Full extraction

Symbols + imports + exports + references + chunks + diagnostics + file facts.

| Language | ID | Extensions |
|---|---|---|
| JavaScript | `javascript` | `.js`, `.mjs`, `.cjs` |
| TypeScript | `typescript` | `.ts` |
| JSX | `jsx` | `.jsx` |
| TSX | `tsx` | `.tsx` |
| Python | `python` | `.py`, `.pyw`, `.pyi` |
| Go | `go` | `.go` |
| Rust | `rust` | `.rs` |
| Java | `java` | `.java` |
| Kotlin | `kotlin` | `.kt`, `.kts` |
| C | `c` | `.c` |
| C++ | `cpp` | `.cpp`, `.cc`, `.cxx`, `.hpp`, `.hxx`, `.h` |
| C# | `csharp` | `.cs` |
| Swift | `swift` | `.swift` |
| Ruby | `ruby` | `.rb`, `.rake`, `.gemspec` |
| PHP | `php` | `.php` |

### Tier 2 -- Partial extraction

Symbols + imports + chunks + diagnostics.

| Language | ID | Extensions |
|---|---|---|
| Bash | `bash` | `.sh`, `.bash`, `.zsh` |
| SQL | `sql` | `.sql` |
| GraphQL | `graphql` | `.graphql`, `.gql` |
| Dockerfile | `dockerfile` | `.dockerfile` |
| HCL (Terraform) | `hcl` | `.tf`, `.tfvars`, `.tofu` |

### Tier 3 -- Structural only

Chunks + diagnostics, minimal symbols.

| Language | ID | Extensions |
|---|---|---|
| HTML | `html` | `.html`, `.htm` |
| CSS | `css` | `.css` |
| SCSS | `scss` | `.scss` |
| JSON | `json` | `.json`, `.jsonc` |
| YAML | `yaml` | `.yaml`, `.yml` |
| TOML | `toml` | `.toml` |
| Markdown | `markdown` | `.md`, `.markdown`, `.mdx` |
| XML | `xml` | `.xml`, `.svg`, `.xsl`, `.xsd`, `.plist` |

### Extension and basename mapping

Language detection (`registry.DetectLanguage`) uses three strategies in order:

1. **Exact basename**: `Dockerfile`, `Gemfile`, `Rakefile`, `Makefile`, `.bashrc`, etc.
2. **Prefix pattern**: `Dockerfile.*` and `dockerfile.*` match the `dockerfile` language.
3. **Extension map**: 50+ extension-to-language-ID mappings covering all tiers.

## 4. Extractor Inventory

Extractors are implemented in `internal/parser/extractors/` and run by the engine based on the language tier. Each extractor operates on the tree-sitter AST (or raw content for chunks) and produces typed output defined in `internal/parser/domain.go`.

### Symbols (`extractors/symbols.go`)

Extracts declarations from the AST based on per-language `SymbolNodeTypes` configuration.

Output per symbol:

- `Name`, `QualifiedName`, `Kind` (function, class, method, variable, interface, type_alias, enum, namespace)
- `Signature`, `StartLine`, `EndLine`, `DocText` (from language-specific doc-comment conventions)
- `SymbolHash` (stable content hash)
- `ParentSymbolID` (for nested symbols)
- v2 extensions: `Modifiers` (async, static, abstract, etc.), `ReturnType`, `ParameterTypes`
- `SymbolFlags`: `IsExported`, `IsDefaultExport`, `IsAsync`, `IsGenerator`, `IsStatic`, `IsAbstract`, `IsReadonly`, `IsOptional`, `IsArrowFunction`, `IsReactComponentLike`, `IsHookLike`

Runs at: all tiers.

### Imports (`extractors/imports.go`)

Walks import-related AST nodes and classifies each import.

Output per import:

- `SourceFilePath`, `TargetFilePath`, `ImportName`
- `ImportType`: `STDLIB` (known standard library module), `INTERNAL` (relative path or internal import pattern), `EXTERNAL` (third-party package)
- `PackageName`, `PackageVersion`

Classification uses per-language stdlib module lists, stdlib prefixes (e.g. `node:`, `std::`, `java.`, `kotlin.`), and internal import patterns (e.g. `./`, `../`, `crate::`, `self::`).

Runs at: Tier 1 and Tier 2.

### Exports (`extractors/exports.go`)

Extracts file-level export declarations.

Output per export:

- `ExportKind`: `NAMED`, `DEFAULT`, `REEXPORT`, `EXPORT_ALL`, `TYPE_ONLY`
- `ExportedName`, `LocalName`, `SymbolID`, `SourceModule`
- `Line`, `Column`

Runs at: Tier 1 only.

### References (`extractors/references.go`)

Finds call sites, type references, and other symbol usages in the AST.

Output per reference:

- `ReferenceKind`: `CALL`, `JSX_RENDER`, `TYPE_REF`, `HOOK_USE`, and others
- `RawText`, `TargetName`, `QualifiedTargetHint`
- `StartLine`, `StartColumn`, `EndLine`, `EndColumn`
- `ResolutionScope`: `LOCAL`, `IMPORTED`, `MEMBER`, `GLOBAL`, `UNKNOWN`
- `Confidence`: `HIGH`, `MEDIUM`, `LOW`

Runs at: Tier 1 only.

### Chunks (`extractors/chunks.go`)

Produces embedding-ready code segments. Unlike other extractors, chunks are text-based and do not require an AST -- they use symbol and import data when available but fall back to raw content.

Output per chunk:

- `ChunkType`: `function`, `class`, `module_context`, `config`, `test`, `raw`
- `ChunkHash` (stable content hash for dedup/upsert decisions)
- `Content`, `ContextBefore`, `ContextAfter`
- `StartLine`, `EndLine`, `EstimatedTokens`
- v2 extensions: `OwnerQualifiedName`, `OwnerKind`, `IsExportedContext`, `SemanticRole` (`implementation`, `api_surface`, `config`, `test`, `ui_component`, `hook`)

Chunking rules:

- One module-context chunk per file (imports, exports, top-level constants, patterns)
- Function/method chunks for declarations with meaningful bodies
- Class-level chunks for declaration + key members summary
- Config/test chunks for high-signal files
- Preserve line ranges exactly for UI links and MCP citations
- Deterministic boundaries for stable chunk hashes
- Oversized chunks (exceeding `embedding.DefaultMaxInputChars`) are split on line boundaries post-extraction

Runs at: all tiers.

### JSX Usages (`extractors/jsx.go`)

Detects JSX component rendering in JSX/TSX files.

Output per usage:

- `ComponentName`, `IsIntrinsic` (HTML elements), `IsFragment`
- `SourceSymbolID`, `Line`, `Column`
- `ResolvedTargetSymbolID`, `Confidence`

Runs at: JSX and TSX files only (any tier, if the language is `jsx` or `tsx`).

### Network Calls (`extractors/network.go`)

Detects HTTP client calls (fetch, axios, ky, GraphQL).

Output per call:

- `ClientKind`: `FETCH`, `AXIOS`, `KY`, `GRAPHQL`, `UNKNOWN`
- `Method`: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `UNKNOWN`
- `URLLiteral`, `URLTemplate`, `IsRelative`
- `StartLine`, `StartColumn`, `Confidence`

Runs at: Tier 1 only.

### Diagnostics (`extractors/diagnostics.go`)

Reports parser warnings and errors from the tree-sitter parse.

Output per issue:

- `Code`, `Message`, `Line`, `Column`
- `Severity`: `info`, `warning`, `error`

Runs at: all tiers.

### File Metadata (`extractors/file_meta.go`)

Computes per-file metadata:

- `FileHash` (SHA-256 content hash)
- `LineCount`, `SizeBytes`

Always runs, even for oversized or unparseable files.

### File Facts (computed in engine)

Boolean facts derived from extractor results:

- `HasJsx`, `HasDefaultExport`, `HasNamedExports`
- `HasTopLevelSideEffects` (side-effect imports like CSS)
- `HasReactHookCalls`, `HasFetchCalls`
- `HasClassDeclarations`, `HasTests`, `HasConfigPatterns`
- `JsxRuntime`: `react`, `preact`, `unknown`

### Extractor Status

Each extractor reports its outcome: `OK`, `PARTIAL`, or `FAILED`. Failures are captured via panic recovery in `runExtractor` -- a panicking extractor does not crash the pipeline. The status array and enabled-extractor list are included in the `ParserMeta` on each file result.

## 5. Cross-Language Import Resolution

Import resolution turns raw import specifiers into resolved file paths within the project. It runs as a post-processing step in the indexing pipeline (`internal/indexing/resolve.go`), after parsing and before artifact persistence.

### Phase 1: Extraction

During parsing, the imports extractor walks import-related AST nodes and classifies each import:

- `STDLIB`: the import target matches a known standard library module or prefix for the language.
- `INTERNAL`: the import path starts with a relative prefix (`./`, `../`) or a language-specific internal pattern (`crate::`, `super::`, `self::` for Rust).
- `EXTERNAL`: everything else (third-party packages).

The `TargetFilePath` is set to the resolved relative path for internal imports (e.g. `./utils/helpers` becomes `src/utils/helpers`).

### Phase 2: Resolution

`ResolveImportTargets` resolves extensionless import targets against the known file set. It builds three lookup maps from all project file paths:

1. **Stem map**: extensionless path to full file path (`src/foo` to `src/foo.ts`). Implementation files (`.ts`) overwrite declaration files (`.d.ts`).
2. **Directory index map**: directory path to index file (`src/lib` to `src/lib/index.ts`).
3. **Exact match set**: full file paths for fast lookups.
4. **Go directory map**: directory path to first `.go` file (for package-level imports).

Resolution order for each `INTERNAL` import:

1. Exact match (already resolved)
2. Stem match (`src/foo` to `src/foo.ts`)
3. Directory index (`src/lib` to `src/lib/index.ts`)

Language-specific index files:

| Language | Index files |
|---|---|
| JS/TS | `index.ts`, `index.tsx`, `index.js`, `index.jsx`, `index.mjs`, `index.cjs` |
| Python | `__init__.py` |
| Rust | `mod.rs` |

### Post-hoc reclassification

After stem-based resolution, a second pass reclassifies `EXTERNAL` imports as `INTERNAL` when the import specifier can be converted to a project-relative path that matches a known file:

| Language | Conversion rule |
|---|---|
| Go | Strip the `go.mod` module path prefix, match against project directories. E.g. `github.com/user/repo/internal/handler` becomes `internal/handler`. |
| Java / Kotlin | Replace dots with slashes. E.g. `com.example.models.User` becomes `com/example/models/User`. |
| C# | Replace dots with slashes. |
| PHP | Trim leading `\`, replace `\` with `/`. E.g. `\App\Models\User` becomes `App/Models/User`. |
| Python | Replace dots with slashes (absolute imports only; relative imports are already handled). |

The Go module path is extracted from `go.mod` at the repository root and passed through the pipeline as `GoModulePath`.

### Incremental indexing

During incremental indexing, the `AllFilePaths` field in the pipeline input includes both changed and unchanged file paths. This allows import resolution to resolve targets that point to unchanged files outside the current parse batch.

## 6. Workflow System

### Dispatcher

The queue consumer receives a generic workflow message:

```json
{
  "job_id": "uuid",
  "workflow": "full-index",
  "enqueued_at": "2026-03-08T11:00:00Z",
  "project_id": "uuid"
}
```

The dispatcher routes the job to the matching workflow handler by `workflow` name. Workflow handlers implement a common `Handler` interface with a `Handle(ctx, task)` method.

### Full-index workflow (`internal/workflow/fullindex/`)

End-to-end indexing of all project source files.

Steps:

1. Load execution context from PostgreSQL (project metadata, repo URL, SSH key, embedding config)
2. Acquire `pg_advisory_xact_lock` for the project
3. Claim the job atomically (`queued` to `running`)
4. Resolve embedding version
5. Prepare workspace: clone or fetch the repository, create worktree, collect file list
6. Index commit history (non-fatal; walks git log into `commits` + `commit_file_diffs` tables)
7. Read file contents from disk (skipping binary files)
8. Split files into parser-supported and raw (non-parseable)
9. Parse supported files via `ParseFilesBatched`
10. Build synthetic results for raw files (hash, line count, content-only chunks)
11. Split oversized chunks on line boundaries
12. Create a new `index_snapshot`
13. Run the storage pipeline: persist artifacts, embed chunks, upsert vectors
14. Activate snapshot (transactional: deactivate old, activate new)
15. Link snapshot to HEAD commit
16. Mark job `completed`

If any step after claiming fails, the job is marked `failed` with structured error details (category, message, step). The handler returns `nil` to prevent queue retry for post-claim failures.

### Incremental-index workflow (`internal/workflow/incremental/`)

Narrows the indexing scope to only changed files.

Steps:

1. Load execution context and claim job (same as full-index)
2. Resolve incremental base: load the active snapshot for the branch, check embedding version compatibility, compute git diff
3. If no valid base is found (no active snapshot, embedding version mismatch, diff error), fall back to full-index semantics
4. Classify diff entries: added, modified, deleted
5. Index new commits since base (non-fatal)
6. Read and parse only changed files
7. Create new snapshot
8. Run pipeline for changed files without activation
9. Copy forward unchanged files: artifacts from the base snapshot are duplicated to the new snapshot with updated snapshot IDs
10. Copy forward vectors for unchanged chunks (update snapshot metadata in Qdrant)
11. Activate snapshot
12. Mark job `completed`

The incremental workflow falls back to full-index semantics when:

- No active snapshot exists for the branch
- The embedding version changed (requires re-embedding all chunks)
- Git diff computation fails

### Commits workflow (`internal/workflow/commits/`)

Indexes git commit history into `commits` and `commit_file_diffs` tables. Runs as a sub-step of both full-index (`IndexAll`) and incremental-index (`IndexSince`). Commit indexing is non-fatal; a failure does not fail the parent indexing job.

## 7. Indexing Pipeline

The storage pipeline (`internal/indexing/pipeline.go`) coordinates the transformation of parser output into durable storage.

### Pipeline phases

The pipeline runs in three sequential phases to maximize throughput:

**Phase 1 -- Artifact persistence (PostgreSQL)**

For each parsed file, the artifact writer (`internal/artifact/`) persists:

- File record (path, language, hash, size, line count)
- Symbols with all v2 extensions
- Imports with resolved targets
- Exports
- References
- JSX usages
- Network calls
- Code chunks (content, metadata, hashes)
- Diagnostics

Progress is updated every 500 files via `SetIndexingJobProgress`.

**Phase 2 -- Embedding generation**

All chunks across all files are collected into a single batch. Chunk content is sent to the embedding provider (Ollama via `internal/embedding/`) for vector generation. This cross-file batching is more efficient than per-file embedding calls.

**Phase 3 -- Vector upsert (Qdrant)**

Vectors are upserted to Qdrant in batches of 100 points. Each point carries a payload with:

- `project_id`, `index_snapshot_id`, `file_path`, `language`
- `symbol_name`, `symbol_type`, `chunk_type`, `chunk_hash`
- `start_line`, `end_line`, `branch`, `git_commit`
- `embedding_version`, `raw_text_ref`

### Snapshot activation

After all writes succeed, the pipeline activates the new snapshot inside a database transaction:

1. Deactivate the currently active snapshot for the `(project_id, branch)` pair
2. Activate the new snapshot

This is atomic: if activation fails, deactivation is rolled back so the branch is never left without an active snapshot.

The incremental workflow uses `RunWithoutActivation` for changed files, then activates separately after copy-forward.

## 8. Advisory Locks

Each workflow handler acquires a PostgreSQL advisory lock before claiming the job:

```go
projectUnlock, err := h.repo.TryProjectLock(ctx, execCtx.ProjectID)
```

This uses `pg_advisory_xact_lock` scoped to the `project_id`. The lock serializes indexing within a project, preventing:

- Concurrent full-index and incremental-index for the same project
- Snapshot activation races
- Duplicate artifact writes

The lock is acquired before `ClaimJob` (so failure allows queue retry) and released via `defer projectUnlock(ctx)` when the handler returns.

Different projects can be indexed concurrently by different workers without contention.

## 9. Workspace and Cache Model

### Project-scoped workspaces

For repo-required workflows, each job uses a project-scoped local workspace under `REPO_CACHE_DIR` (default: `/var/lib/myjungle/repos`). The workspace contains:

- Cloned repository data (bare or full checkout)
- Fetched refs and commit state
- Temporary worktree for the target branch

### Cache semantics

The repo cache is:

- Helpful: avoids full re-clone on every job
- Replaceable: can be deleted without data loss
- Rebuildable: worker re-clones from the Git remote if cache is missing

The cache is not the source of truth. Truth lives in:

- Git remote for repository content
- PostgreSQL for job and snapshot state
- Qdrant for vector state

### Multi-worker behavior

In a scaled deployment with multiple workers:

- Each worker has its own local workspace or cache
- Workers clone or fetch as needed
- Cache reuse is opportunistic, not required
- Correctness does not depend on shared storage

A shared `repo_cache` volume is acceptable for local development (single worker) but should not be assumed for production multi-worker deployments.

### Embedded parser implication

Because the parser runs inside `backend-worker`, file access is straightforward:

- The worker prepares the workspace
- The parser reads from the same container filesystem

No file transfer or shared volume between separate containers is needed.

### Project isolation

- Project A and project B never share the same mutable workspace
- Retries do not corrupt another project's files
- Cleanup only touches the intended project workspace

## 10. Deployment and Scaling

### Horizontal scaling

Scaling `backend-worker` means adding more identical replicas. All replicas:

- Compete for tasks from the same Redis/asynq queue
- One job is processed by one worker at a time
- Retries may be handled by a different worker

Worker logic must remain:

- Idempotent
- Project-scoped
- Safe under retries
- Independent from local process memory

### Parser capacity scales with workers

Because the parser is embedded, adding a worker replica automatically adds parser capacity. There is no separate parser service to scale independently.

Tradeoff: every worker replica carries parser runtime overhead. This is acceptable while parsing is tightly coupled to repo-required workflows.

### Queue model

`backend-worker` uses Redis/asynq as the queue backend.

- Queue name: `QUEUE_NAME` (default: `default`)
- Concurrency: `WORKER_CONCURRENCY` (default: 4 concurrent handlers per worker)
- Shutdown timeout: `QUEUE_SHUTDOWN_TIMEOUT` (default: 30s)
- Per-project fairness: prevents one hot project from starving others
- Deduplication: uniqueness window per project and job type

Workers are long-lived consumers. There is not one container per queue message.

### Worker registry

Each running worker publishes an ephemeral status entry to Redis for observability.

- Key pattern: `worker:status:{worker_id}`
- Heartbeat interval: 10 seconds
- Key TTL: 30 seconds (auto-expires if worker stops publishing)
- Worker ID: `WORKER_ID` env var, falling back to `os.Hostname()`

Status lifecycle:

1. `starting` -- on boot, before queue consumer is ready
2. `idle` -- ready and waiting for tasks
3. `busy` -- executing a job (includes `current_job_id` and `current_project_id`)
4. `draining` -- graceful shutdown in progress

The registry is advisory only. PostgreSQL remains the source of truth for job ownership, completion, and failure.

### Draining and replacement

During rollout or shutdown:

- Worker stops taking new tasks before exit
- Publishes `draining` while in-flight work finishes or times out
- Other workers continue consuming from the shared queue

### Autoscaling (future)

Manual replica counts are sufficient initially. Later, autoscaling can target:

- CPU or memory usage
- Redis queue depth
- Queue latency

## 11. Service Layout

```
backend-worker/
  cmd/
    worker/
      main.go                       # Entrypoint
  internal/
    app/                            # Application bootstrap and wiring
    artifact/                       # PostgreSQL artifact writer (files, symbols, chunks, etc.)
    config/                         # Environment-driven configuration + defaults
    embedding/                      # Embedding provider client (Ollama)
    execution/                      # Execution context (project metadata loaded at job start)
    gitclient/                      # Git operations (clone, fetch, diff)
    indexing/                       # Storage pipeline + import resolution
    logger/                         # Structured logging (slog)
    notify/                         # Redis pub/sub event publisher
    parser/
      domain.go                     # Domain types (ParsedFileResult, Symbol, Import, Chunk, etc.)
      pool.go                       # Bounded tree-sitter parser pool
      grammars.go                   # Grammar loading via smacker/go-tree-sitter
      normalize.go                  # Content normalization (CRLF, BOM)
      sort.go                       # Deterministic sorting for all result types
      dedup.go                      # Import deduplication
      engine/
        engine.go                   # Parser engine: pipeline orchestration per file
      extractors/
        symbols.go                  # Symbol declaration extraction
        imports.go                  # Import/dependency extraction
        exports.go                  # Export detection
        references.go               # Call sites and type references
        chunks.go                   # Embedding-ready code chunking
        jsx.go                      # JSX component usage detection
        network.go                  # HTTP client call detection
        diagnostics.go              # Parser warnings and errors
        file_meta.go                # File hash, line count, size
        utils.go                    # Shared extractor utilities
      registry/
        registry.go                 # Language detection and config lookup
        languages.go                # 28 language configurations (tiers, AST mappings, stdlib)
    queue/                          # Asynq queue consumer and task definitions
    registry/                       # Worker heartbeat registry (Redis)
    repository/                     # Job repository (claim, complete, fail, lock)
    sshenv/                         # SSH environment setup for git operations
    sshkey/                         # SSH key decryption
    storage/                        # Storage utilities
    vectorstore/                    # Qdrant client (ensure collection, upsert, delete)
    workflow/
      handler.go                    # Workflow handler interface
      task.go                       # WorkflowTask type
      fullindex/
        handler.go                  # Full-index workflow handler
      incremental/
        handler.go                  # Incremental-index workflow handler
        copyforward.go              # Copy-forward logic for unchanged files/vectors
      commits/
        indexer.go                  # Git commit history indexer
    workspace/                      # Workspace preparation (clone/fetch, worktree, file list)
  Dockerfile
  go.mod
  go.sum
```

## 12. Configuration

All configuration is read from environment variables at startup (`internal/config/config.go`). Required variables cause a panic if missing.

### Required variables

| Variable | Description |
|---|---|
| `POSTGRES_DSN` | PostgreSQL connection string |
| `REDIS_URL` | Redis connection URL |
| `SSH_KEY_ENCRYPTION_SECRET` | Encryption key for SSH private keys stored in PostgreSQL |

### Optional variables with defaults

| Variable | Default | Description |
|---|---|---|
| `LOG_LEVEL` | `info` | Minimum log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | Log output format: `json` or `text` |
| `POSTGRES_MAX_CONNS` | `10` | Maximum PostgreSQL connections |
| `POSTGRES_MIN_CONNS` | `2` | Minimum PostgreSQL connections |
| `POSTGRES_MAX_CONN_LIFE` | `30m` | Maximum connection lifetime |
| `QUEUE_NAME` | `default` | Asynq queue name |
| `WORKER_CONCURRENCY` | `4` | Number of concurrent workflow handlers per worker |
| `QUEUE_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `PARSER_POOL_SIZE` | `0` (= `runtime.NumCPU()`) | Tree-sitter parser pool size |
| `PARSER_TIMEOUT` | `5m` | Overall batch parse timeout |
| `PARSER_TIMEOUT_PER_FILE` | `30s` | Per-file parse timeout |
| `PARSER_MAX_FILE_SIZE` | `10485760` (10 MB) | Maximum file size in bytes |
| `OLLAMA_URL` | `http://host.docker.internal:11434` | Embedding provider URL |
| `OLLAMA_MODEL` | `jina/jina-embeddings-v2-base-en` | Embedding model name |
| `QDRANT_URL` | `http://qdrant:6333` | Qdrant connection URL |
| `REPO_CACHE_DIR` | `/var/lib/myjungle/repos` | Local repository cache directory |
| `WORKER_ID` | `os.Hostname()` | Worker identifier for the Redis registry |
| `PROVIDER_ENCRYPTION_SECRET` | (none) | Encryption key for provider credentials |

## 13. Docker Notes

The worker builds a single Go binary from `./cmd/worker` and runs as a non-root container.

### Compose configuration

```yaml
backend-worker:
  build:
    context: .
    dockerfile: backend-worker/Dockerfile
  profiles: ["app"]
  depends_on:
    postgres:
      condition: service_healthy
    qdrant:
      condition: service_healthy
    redis:
      condition: service_healthy
  volumes:
    - repo_cache:/var/lib/myjungle/repos
  networks:
    - internal
```

Key properties:

- **No public ports**: the worker communicates only with internal services.
- **Depends on**: PostgreSQL, Qdrant, and Redis must be healthy before the worker starts.
- **Shared volume**: `repo_cache` is mounted at `REPO_CACHE_DIR` for repository workspace persistence across restarts.
- **Host gateway**: `host.docker.internal` is mapped via `extra_hosts` for Ollama access when it runs outside Docker.
- **Profile**: the worker runs under the `app` profile, separate from infrastructure services.

### Stopping the worker

If `backend-worker` is not running:

- `backend-api` can still create jobs and enqueue tasks
- Queued work waits in Redis until a worker starts
- No data is lost

### Local development

```bash
# Start infrastructure
docker compose up -d postgres redis qdrant

# Build and start app services (backend-api + backend-worker)
docker compose --profile app up --build

# Check worker heartbeat
docker compose exec redis redis-cli GET "worker:status:myjungle-backend-worker"

# Follow worker logs
docker compose logs -f backend-worker
```
