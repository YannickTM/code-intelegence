# 09 — Indexing Pipeline & Vector Persistence

## Status
Done

## Goal
Build the three-phase storage pipeline that turns parser output into durable PostgreSQL artifacts, vector embeddings via Ollama, and Qdrant vectors, culminating in atomic snapshot activation. This closes the gap between parsing and searchable index data.

## Depends On
03-job-lifecycle, 05-parser-engine

## Scope

### Pipeline Architecture (`internal/indexing/pipeline.go`)

The `Pipeline` struct coordinates storage across three backends. It receives `PipelineInput` containing parsed files, file contents, snapshot metadata, and embedding config, then executes three sequential phases:

**Phase 1 -- Artifact Persistence (PostgreSQL):**

The `artifact.Writer` persists each parsed file result through a nine-step per-file write:

1. **File content deduplication:** SHA-256 content hash -> `UpsertFileContent` into `file_contents` table (content-addressable, unique on `(project_id, content_hash)`)
2. **File row:** `InsertFile` with file facts, parser metadata, extractor statuses, issues, and `file_content_id` link
3. **Symbols:** `InsertSymbol` with parent linking via running `symbolMap` (parser emits AST order), v2 flags (JSONB), qualified names, modifiers, return types, parameter types
4. **Code chunks:** `InsertCodeChunk` with v2 semantic context (`owner_qualified_name`, `owner_kind`, `is_exported_context`, `semantic_role`)
5. **Dependencies:** `InsertDependency` with resolved `target_file_path` from import resolution
6. **Exports:** `InsertExport` with `linked_symbol_id` from symbol map
7. **Symbol references:** `InsertSymbolReference` with resolution scope and confidence
8. **JSX usages:** `InsertJsxUsage` with component name and intrinsic/fragment flags
9. **Network calls:** `InsertNetworkCall` with client kind, method, URL patterns

Progress is reported to PostgreSQL every 500 files via `SetIndexingJobProgress`.

**Phase 2 -- Embedding Generation (Ollama):**

The `embedding.OllamaClient` calls `POST /api/embed` on the Ollama endpoint:

- Batch size: 50 texts per HTTP request (`DefaultEmbedBatchSize`)
- Per-request timeout: 5 minutes (`DefaultEmbedRequestTimeout`)
- Input truncation: texts exceeding `maxInputChars` (default 7500, configurable per embedding config via `max_tokens`) are truncated with a warning log
- Dimension validation: each returned vector is checked against the expected dimensions; mismatches produce an immediate error
- Cross-file batching: all chunks across all files are collected into a single slice and embedded together, reducing Ollama round-trips

**Phase 3 -- Vector Upsert (Qdrant):**

The `vectorstore.QdrantClient` is a REST client using the Qdrant HTTP API:

- Collection naming: `project_{hex}__emb_{versionLabel}` via `CollectionName()`
- Lazy collection creation: `EnsureCollection` checks existence (GET), creates on 404 (PUT) with Cosine distance and configured dimensions; 409 Conflict is handled for concurrent creation
- Batch upsert: points are written in batches of 100 (`vectorUpsertBatchSize`)
- Payload metadata per vector: `project_id`, `index_snapshot_id`, `file_path`, `language`, `symbol_name`, `symbol_type`, `chunk_type`, `chunk_hash`, `start_line`, `end_line`, `branch`, `git_commit`, `embedding_version`, `raw_text_ref` (back-reference to `code_chunks` table)

### Embedding Version Resolution

Each snapshot points to an `embedding_version_id`. The worker derives a deterministic `version_label` from `"{provider}-{model}-{dimensions}"` and looks up or creates the `embedding_versions` row (idempotent via UNIQUE constraint on `version_label`).

### Snapshot Activation (`ActivateSnapshotTx`)

Activation runs inside a database transaction via `TxActivator`:
1. `DeactivateActiveSnapshot` for the `(project_id, branch)` pair
2. `ActivateSnapshot` sets `is_active = TRUE, status = 'active'`

If activation fails, the transaction rolls back so the branch is never left without an active snapshot. The previous active snapshot remains untouched.

### Import Resolution (`resolve.go`)

`ResolveImportTargets` runs as a post-processing step after parsing, before artifact persistence. It builds lookup maps from all project file paths (stem map, directory index map, exact match set, Go directory map) and resolves each INTERNAL import to a concrete `target_file_path`. Post-hoc reclassification converts EXTERNAL imports to INTERNAL for Go, Java, Kotlin, C#, and PHP when the import specifier matches a known project file.

`GoModulePath` is extracted from `go.mod` and propagated through `PipelineInput` so it is always available, including during incremental indexing when `go.mod` may not be in `FileContents`.

### Pipeline Variants

- `Run()`: full pipeline including snapshot activation (used by full-index workflow)
- `RunWithoutActivation()`: pipeline without activation (used by incremental workflow, which activates after copy-forward)

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/indexing/pipeline.go` | Pipeline orchestration, three-phase execution, snapshot activation |
| `internal/indexing/resolve.go` | Import target resolution, stem/directory maps, post-hoc reclassification |
| `internal/artifact/writer.go` | Nine-step per-file artifact persistence to PostgreSQL |
| `internal/embedding/client.go` | Ollama `/api/embed` client, batching, truncation, dimension validation |
| `internal/vectorstore/client.go` | Qdrant REST client: collection creation, point upsert, point retrieval |
| `internal/vectorstore/collection.go` | `CollectionName()` naming convention |

## Acceptance Criteria
- [x] Artifact writer persists files, symbols, chunks, dependencies, exports, references, JSX usages, and network calls
- [x] File content deduplicated via `file_contents` table with SHA-256 content hash
- [x] Symbol parent linking resolved via running symbol map in AST order
- [x] v2 fields populated on symbols (flags, modifiers, return type) and chunks (semantic role, owner context)
- [x] Ollama embedding client batches requests (50 per call) with per-request timeout
- [x] Oversized embedding inputs truncated at configurable character limit with warning
- [x] Vector dimension mismatches detected and surfaced as errors
- [x] Qdrant collection created lazily on first use with Cosine distance
- [x] Vectors include metadata payload with `raw_text_ref` back to PostgreSQL `code_chunks`
- [x] Batch vector upsert (100 points per Qdrant call)
- [x] Snapshot activation atomic: deactivate-old + activate-new in single transaction
- [x] Activation failure rolls back; previous active snapshot preserved
- [x] Import targets resolved against full project file set (including unchanged files in incremental)
- [x] Go module path extracted and used for import reclassification
- [x] Job progress counters updated during pipeline execution
- [x] `RunWithoutActivation` variant available for incremental workflow
