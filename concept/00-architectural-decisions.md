# Architectural Decisions v8

This file captures the active architectural decisions for the MYJUNGLE Code Intelligence Platform.

Related detail lives in [`01-system-overview.md`](./01-system-overview.md).

## ADR-001: Service split

Decision:

- Keep `backend-api`, `backend-worker`, `mcp-server`, and `backoffice` as separate services.

Rationale:

- Clear execution boundaries
- Independent scaling paths
- Smaller deployable units with simpler responsibilities

Note: The parser is now embedded in `backend-worker` rather than running as a separate service. See ADR-017.

## ADR-002: Deployment baseline

Decision:

- Docker Compose remains the default baseline for local development and the first self-hosted deployments.

Baseline runtime (7 services):

- `postgres`
- `qdrant`
- `redis`
- `backend-api`
- `backend-worker`
- `mcp-server`
- `backoffice`

Embedding provider (Ollama) runs externally or as an optional compose profile.

Rationale:

- Reproducible integration environment
- Fast local onboarding

## ADR-003: Durable truth vs async transport

Decision:

- PostgreSQL is the source of truth for projects, jobs, snapshots, and indexed artifacts.
- Redis/asynq is transport for async execution and optional transient event fanout.

Rationale:

- Queue state and event delivery are not sufficient as business truth
- Job and health reads must work even when pub/sub or SSE is unavailable
- Retries and recovery are simpler when state transitions are durable

## ADR-004: Dual-store retrieval model

Decision:

- Use PostgreSQL for structure, state, and raw indexed artifacts.
- Use Qdrant for vector retrieval.

Rationale:

- Better separation of concerns
- Easier operational reasoning
- Retrieval can evolve without collapsing all concerns into one store

## ADR-005: Backend-owned job creation, worker-owned execution

Decision:

- `backend-api` creates jobs and publishes queue tasks.
- `backend-worker` consumes tasks and executes workflows.

Rationale:

- Keeps request handling separate from long-running execution
- Makes worker scaling independent from API scaling
- Avoids a larger planner/coordinator layer before it is needed

## ADR-006: Minimal queue payload, runtime context loading

Decision:

- Queue messages should stay small and contain a durable job reference plus minimal routing or observability hints.
- The worker loads the project-scoped execution context at runtime from durable state.

That execution context can include:

- project metadata
- repo URL and branch defaults
- active SSH key assignment
- embedding configuration
- LLM configuration

Rationale:

- Avoids copying secrets into Redis
- Retries use current project configuration
- Reduces queue payload size and drift

## ADR-007: Project-scoped isolation and fairness

Decision:

- Treat `project_id` as the primary isolation boundary for job execution.
- Preserve fairness across projects in queueing and worker scheduling.

Rationale:

- The system is multi-project by design
- One noisy project must not block every other project indefinitely
- Retries and failures must not cause cross-project state corruption

## ADR-008: Repo-required workflows vs retrieval-oriented workflows

Decision:

- Distinguish workflows by their data-access pattern.

Repo-required workflows:

- `full-index`
- `incremental-index`
- `commits`
- `code-analysis`
- `agent-run`

Retrieval-oriented workflows:

- `rag-file`
- `rag-repo`

Rationale:

- Not every workflow should pay the cost of cloning a repository
- Cost, scaling, and dependency profiles differ significantly between these workflow classes

## ADR-009: Keep indexing-specific job storage until a second workflow is real

Decision:

- Keep `indexing_jobs` and `index_snapshots` as the primary workflow state model while indexing is the only real implemented workflow.
- Map worker workflow names such as `full-index` and `incremental-index` onto existing indexing job types until broader workflow support is real.

Rationale:

- Avoids premature schema generalization
- Uses the existing durable model already present in the repo
- Keeps migration cost tied to real product need

## ADR-010: Embedded parser 

Decision:

- The parser is embedded directly in `backend-worker` using `smacker/go-tree-sitter` bindings.

Rationale:

- Removes the gRPC boundary and its associated serialization overhead
- Simplifies deployment from 8 services to 7
- Parser capacity scales automatically with worker replicas
- `go-tree-sitter` bindings are mature and support all 28 target languages
- No separate container image, health checks, or version coordination required
- Eliminates a class of cross-process failure modes

## ADR-011: Git authentication via reusable SSH key assignments

Decision:

- Projects use an active assigned SSH key for repository access.
- SSH keys are reusable across projects when desired.
- Private keys remain encrypted at rest.

Rationale:

- Works across common Git hosts
- Keeps access explicit at the project boundary
- Supports rotation and reuse without user-scoped token sprawl

## ADR-012: Embedding provider strategy

Decision:

- The system uses a pluggable embedding provider abstraction with a common interface.
- Phase 1 default is Ollama.
- The provider registry supports multiple backends (Ollama, OpenAI, custom) selectable per project or globally.

Rationale:

- Keeps phase 1 self-hostable
- Allows later provider expansion without changing the worker model
- Makes embedding concerns separate from queue and parsing concerns

## ADR-013: Incremental indexing follows a stable full-index path

Decision:

- Deliver `full-index` first.
- Add `incremental-index` only on the same execution runtime after the full path is stable.

Rationale:

- Incremental correctness depends on a known-good full snapshot path
- Reduces moving parts during the first end-to-end implementation

## ADR-014: SSE is useful but not authoritative

Decision:

- SSE remains the preferred live update channel for backoffice status.
- Durable job and project status must remain readable from PostgreSQL without SSE.

Rationale:

- SSE matches the one-way live-update use case well
- Operational status should not depend on a live event stream being healthy

## ADR-015: API versioning

Decision:

- HTTP endpoints remain versioned under `/v1`.
- MCP tool schemas remain explicit contracts.

Rationale:

- Preserves compatibility for clients and agents
- Makes iterative changes safer

## ADR-016: Scope boundary for phase 1

Decision:

- Phase 1 delivers a broad language-coverage indexing platform:
  - multi-project support with project-scoped isolation
  - project CRUD and SSH key assignment
  - `full-index`, `incremental-index`, and `commits` workflows
  - 28-language parsing (13 Tier 1, 15 Tier 2) via embedded go-tree-sitter
  - extractors: symbols, imports, exports, references, JSX usages, network calls, chunks, diagnostics, file metadata/facts
  - cross-language import resolution
  - PostgreSQL + Qdrant-backed reads
  - 6 MCP tools: `search_code`, `get_symbol_info`, `get_dependencies`, `get_project_structure`, `get_file_context`, `get_conventions`
  - GitHub OAuth for backoffice, API keys for MCP/programmatic access
  - basic job monitoring and project health

Out of scope for phase 1:

- workflow coordinators and chain engines
- maintenance pipelines
- global cross-project search
- direct backend code-mutation APIs
- enterprise auth complexity beyond GitHub OAuth

Rationale:

- Broad language support from the start maximizes the value of the indexing core
- GitHub OAuth provides a practical auth baseline without enterprise complexity

## ADR-017: Embedded parser using go-tree-sitter

Decision:

- All source-code parsing runs inside `backend-worker` using the `smacker/go-tree-sitter` library with precompiled grammar bindings for each supported language.
- Each extractor (symbols, imports, exports, references, JSX usages, network calls, chunks, diagnostics, file metadata) operates as a Go function that receives a tree-sitter syntax tree and returns structured artifacts.

Rationale:

- Single-binary deployment per worker replica
- No IPC latency; parsing is an in-process function call
- Tree-sitter grammars are battle-tested across editors (Neovim, Helix, Zed)
- Go CGo overhead for tree-sitter is negligible relative to file I/O and embedding costs
- Extractor logic is testable with standard Go unit tests

## ADR-018: Backoffice auth via GitHub OAuth

Decision:

- The backoffice uses GitHub OAuth for user authentication, implemented via the `better-auth` library.
- `backend-api` exposes auth endpoints consumed by the Next.js backoffice.
- API keys provide programmatic and MCP access independently of OAuth.

Rationale:

- GitHub accounts are the natural identity for the target user base (developers, AI agents)
- `better-auth` provides session management, CSRF protection, and token refresh out of the box
- Separating OAuth (interactive) from API keys (programmatic) keeps auth flows clean

## ADR-019: Cross-language import resolution with stem-based matching

Decision:

- Import paths are resolved to indexed files using a multi-strategy resolver:
  1. Exact path match against the file tree
  2. Extension completion (try known extensions for the source language)
  3. Index-file resolution (append `/index.*` for directory imports)
  4. Stem-based fuzzy match (strip extensions, normalize separators, match against indexed file stems)
- Resolution results are stored as edges in the dependency graph for downstream queries.

Rationale:

- Different languages have different import conventions (bare specifiers, relative paths, package names)
- Stem-based matching handles cross-language references (e.g., a TypeScript file importing a generated `.js` file)
- A layered strategy keeps exact matches fast while falling back gracefully

## ADR-020: PostgreSQL advisory locks for per-project serialized indexing

Decision:

- Use PostgreSQL advisory locks keyed on `project_id` to serialize concurrent indexing jobs for the same project.
- Workers acquire the lock before starting a workflow and release it on completion or failure.

Rationale:

- Prevents snapshot corruption from overlapping full-index and incremental-index runs on the same project
- Advisory locks are lightweight and do not block unrelated table operations
- No external coordination service required; PostgreSQL is already the durable store

## ADR-021: Tiered language support

Decision:

- Languages are grouped into tiers based on extraction depth:

Tier 1 (13 languages) -- full extraction (symbols, imports, exports, references, chunks, diagnostics, file metadata):
  - JavaScript, TypeScript, JSX, TSX, Python, Go, Rust, Java, Kotlin, C#, Swift, Ruby, PHP

Tier 2 (15 languages) -- partial extraction (symbols, chunks, file metadata; no import/export resolution):
  - C, C++, HTML, CSS, SCSS, JSON, YAML, TOML, XML, Markdown, Bash, Dockerfile, SQL, GraphQL, HCL

Tier 3 (any other file) -- structural fallback:
  - Line-based chunking, file metadata only

Rationale:

- Full extractor coverage for every grammar is expensive to build and maintain
- Tier 1 covers the languages where import graphs and symbol navigation provide the highest agent value
- Tier 2 still benefits from tree-sitter structure without requiring full semantic extractors
- Tier 3 ensures no file is silently dropped from the index

## ADR-022: Pluggable embedding and LLM provider architecture

Decision:

- Embedding and LLM providers are registered through a common provider interface.
- Each provider implements `Embed(texts) -> vectors` and optionally `Complete(prompt) -> text`.
- Provider selection is configurable per project or at the system level.
- Phase 1 ships Ollama as the default provider.

Rationale:

- Self-hosted users need Ollama; cloud users may prefer OpenAI, Anthropic, or Cohere
- A pluggable interface avoids hard coupling to any single vendor
- Provider-level configuration per project supports mixed environments (e.g., sensitive repos use on-prem, others use cloud)
