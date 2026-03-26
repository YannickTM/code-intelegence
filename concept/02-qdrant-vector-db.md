# Qdrant Vector DB v8

## Responsibility

Qdrant stores semantic embeddings for code chunks and enables natural-language retrieval with metadata filtering.

Qdrant answers: "What code is semantically similar to this query?"

## What Gets Embedded

- Function and method bodies (e.g., Go handlers, Python class methods, Rust impl blocks, Java service methods)
- Class and struct declarations (signature + doc + key members summary)
- Module-level context (imports, constants, entrypoint patterns)
- Config, infrastructure, and test snippets when useful for intent retrieval
- Tier 2 and Tier 3 file chunks (line-based or tree-sitter-structural) for languages without full symbol extraction

Embedding input is language-agnostic at the vector level. The parser embedded in `backend-worker` produces chunks for all 28 supported languages (13 Tier 1, 15 Tier 2) plus line-based fallback for unrecognized files.

## Collection Strategy

Use one collection per project + embedding model version to avoid mixed-vector spaces.

```text
collection_name = project_{project_id}__emb_{embedding_version}
example: project_5f2c__emb_nomic-embed-code_2026-03
```

Benefits:

- Safe model upgrades (new collection, then switch active pointer)
- No accidental cross-model similarity scoring
- Simple rollback to prior embedding version

## Payload Schema (Recommended)

```json
{
  "project_id": "uuid",
  "index_snapshot_id": "uuid",
  "file_path": "internal/handler/auth.go",
  "language": "go",
  "symbol_name": "ValidateToken",
  "symbol_type": "function",
  "chunk_type": "function",
  "chunk_hash": "sha256:...",
  "start_line": 41,
  "end_line": 96,
  "branch": "main",
  "git_commit": "a1b2c3d4",
  "embedding_version": "nomic-embed-code_2026-03",
  "raw_text_ref": "db://code_chunks/{chunk_id}"
}
```

The `language` field covers all 28 supported languages:

- Tier 1: `javascript`, `typescript`, `jsx`, `tsx`, `python`, `go`, `rust`, `java`, `kotlin`, `csharp`, `swift`, `ruby`, `php`
- Tier 2: `c`, `cpp`, `html`, `css`, `scss`, `json`, `yaml`, `toml`, `xml`, `markdown`, `bash`, `dockerfile`, `sql`, `graphql`, `hcl`
- Fallback: `unknown` for files parsed with line-based chunking only

Note: Full raw code is stored in the PostgreSQL `code_chunks` table (see doc 03). The `raw_text_ref` field references the chunk by UUID. Keep Qdrant payload lean -- include short snippets only when needed for latency.

## Index Configuration Baseline

Vector dimensions depend on the embedding model selected via the pluggable provider architecture. The configuration below assumes a typical code embedding model; adjust `size` to match the actual model output.

```yaml
vectors:
  size: 768          # adjust to match embedding model dimensions
  distance: Cosine
hnsw_config:
  m: 16
  ef_construct: 128
optimizers:
  indexing_threshold: 20000
```

Tune after real workload measurements. The `size` value is read from the `embedding_versions` table (see doc 03) and applied when creating each collection.

## Query Patterns

### Semantic Search

- Input: query text + project scope + optional filters
- Output: top-k chunk ids + scores

### Filtered Semantic Search

With 28 languages indexed, filtered search becomes meaningful for cross-language projects. Typical filters:

- `language in (...)` -- narrow to specific languages (e.g., only backend Go code, only frontend TypeScript)
- `symbol_type in (...)` -- functions, classes, interfaces, structs
- `file_path like ...` -- directory scoping (e.g., `internal/...`, `src/components/...`)
- `chunk_type in (...)` -- function, class, module_context, config, test
- `branch = active_branch`

### Cleanup by Snapshot

On re-index, remove stale vectors by `index_snapshot_id` or `file_path` filter to keep collection consistent.

## Lifecycle Operations

- `create_collection` on first index or new embedding version
- `upsert` chunks during indexing
- `delete` removed symbols/files
- `snapshot` before migrations or large model transitions
- `drop_collection` when project is deleted

## Capacity Heuristics

- 1k files: roughly 80k to 250k vectors
- 10k files: roughly 0.8M to 2.5M vectors
- Memory depends on dim, HNSW params, payload size; payload bloat is the main avoidable cost

Polyglot projects with many small files (e.g., config, Dockerfiles, SQL migrations) may skew toward the higher end of these ranges due to additional Tier 2/3 chunks.

## Docker Notes

- Run Qdrant as its own container with persisted volume
- Expose 6333 internally; public exposure optional and usually unnecessary
- Add healthcheck and startup dependency from backend
