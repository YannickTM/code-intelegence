# Qdrant

## Responsibility

Qdrant stores semantic embeddings for code chunks and supports low-latency similarity search with metadata filters.

It answers: "What code is semantically similar to this query?"

## Collection Strategy

Use one collection per project and embedding version.

```text
project_{project_id}__emb_{embedding_version}
example: project_5f2c__emb_nomic-embed-code_2026-03
```

Benefits:

- No mixed vector spaces across model changes
- Safe rollout and rollback for embedding upgrades
- Straightforward lifecycle operations per project/version

## Embedded Units

- Function and method chunks
- Class declaration chunks
- Module context chunks
- Relevant config/test snippets

## Payload Schema (Recommended)

```json
{
  "project_id": "uuid",
  "index_snapshot_id": "uuid",
  "file_path": "services/auth/handler.ts",
  "language": "typescript",
  "symbol_name": "validateToken",
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

Store full raw code in PostgreSQL `code_chunks`. Keep Qdrant payload lean.

## Index Configuration Baseline

```yaml
vectors:
  size: 768
  distance: Cosine
hnsw_config:
  m: 16
  ef_construct: 128
optimizers:
  indexing_threshold: 20000
```

`size` must match the active embedding model dimensions from PostgreSQL `embedding_versions`.

## Query Patterns

### Semantic Search

- Input: query embedding + project scope + optional filters
- Output: top-k IDs with similarity scores

### Filtered Search

Typical filters:

- `language`
- `symbol_type`
- `file_path`
- `branch`
- `index_snapshot_id`

### Snapshot Cleanup

Delete stale vectors by `index_snapshot_id` or `file_path` during reindexing.

## Lifecycle Operations

- Create collection on first index/new embedding version
- Upsert vectors during indexing
- Delete vectors for removed files/symbols
- Snapshot before major migrations
- Drop collection when project is deleted

## Capacity Heuristics

- 1k files: about 80k to 250k vectors
- 10k files: about 0.8M to 2.5M vectors

Memory usage is driven mainly by vector dimensions and payload size.

## Docker Notes

- Dedicated container with persistent storage volume
- Internal port `6333`
- External exposure optional for debug/admin access
