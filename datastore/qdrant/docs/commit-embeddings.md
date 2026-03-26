# Commit & Diff Embeddings (Qdrant)

## Purpose

Enables semantic search over git history: "when did we fix the auth bug?", "what changed around the caching layer?", "find commits that touched error handling".

Stored in a **separate collection** from code chunk vectors because they serve a different query purpose (understanding change history vs. understanding current code state).

## Collection Naming

```
project_{project_id}__commits__emb_{embedding_version}
```

Follows the existing code chunk pattern (`project_{id}__emb_{version}`) with `__commits__` inserted to distinguish the two collection families.

Examples:
```
project_5f2c__commits__emb_nomic-embed-code_2026-03
project_a1b3__commits__emb_nomic-embed-code_2026-03
```

## Embed Types

Each collection contains two types of vectors, distinguished by the `embed_type` payload field.

### `commit_message`

- **Source text:** `commits.message` (full commit message from PostgreSQL)
- **Granularity:** One vector per commit
- **Use case:** Semantic search by intent — "when did we add rate limiting?"

### `file_diff`

- **Source text:** `commit_file_diffs.patch` (unified diff, truncated to token budget)
- **Granularity:** One vector per file diff (a single commit may produce many)
- **Use case:** Find specific file-level changes — "changes to the auth middleware"

## Vector Configuration

Matches the existing code chunk collection baseline:

```yaml
vectors:
  size: 768
  distance: Cosine
hnsw_config:
  m: 16
  ef_construct: 128
optimizers:
  indexing_threshold: 10000
```

`size` must match the active embedding model dimensions from PostgreSQL `embedding_versions`.

## Point ID Strategy

- `commit_message` vectors: use `commits.id` (UUID) as the Qdrant point ID
- `file_diff` vectors: use `commit_file_diffs.id` (UUID) as the Qdrant point ID

This ensures 1:1 mapping between PostgreSQL rows and Qdrant points for upsert idempotency.

## Payload Schema

```json
{
  "project_id": "uuid",
  "commit_id": "uuid",
  "commit_hash": "a1b2c3d4e5f6",
  "embed_type": "commit_message",
  "committer_date": "2026-03-10T12:00:00Z",
  "message_preview": "fix: resolve auth token expiry issue",
  "old_file_path": null,
  "new_file_path": null,
  "change_type": null,
  "additions": null,
  "deletions": null,
  "diff_id": null,
  "raw_text_ref": "db://commits/{commit_id}"
}
```

For `file_diff` vectors, the file-specific fields are populated:

```json
{
  "project_id": "uuid",
  "commit_id": "uuid",
  "commit_hash": "a1b2c3d4e5f6",
  "embed_type": "file_diff",
  "committer_date": "2026-03-10T12:00:00Z",
  "message_preview": "fix: resolve auth token expiry issue",
  "old_file_path": "src/auth/token.ts",
  "new_file_path": "src/auth/token.ts",
  "change_type": "modified",
  "additions": 12,
  "deletions": 5,
  "diff_id": "uuid",
  "raw_text_ref": "db://commit_file_diffs/{diff_id}"
}
```

Full text (commit messages and diff patches) stays in PostgreSQL. The `raw_text_ref` field follows the existing code chunk collection convention for resolving full content.

## Payload Indexes

Create Qdrant payload indexes on these fields for filtered search:

| Field | Index Type | Filter Use Case |
|-------|-----------|----------------|
| `project_id` | keyword | Always filtered (required scope) |
| `embed_type` | keyword | Search only messages or only diffs |
| `commit_id` | keyword | Delete by commit, fetch related diffs |
| `old_file_path` | keyword | Find changes by original file path (renames, deletes) |
| `new_file_path` | keyword | Find changes by current file path |
| `change_type` | keyword | Filter by added/modified/deleted |
| `committer_date` | datetime | Time range queries |

## Query Patterns

### "When did we fix X?"

Embed the natural language query, search with `embed_type=commit_message`, return top-k commit messages with metadata.

### "What changed in file Y?"

Embed the query, search with `embed_type=file_diff` and filter on `new_file_path` (catches adds, modifies, renames-to) OR `old_file_path` (catches deletes, renames-from). Use a Qdrant `should` filter to match either path.

### "Recent changes related to Z"

Embed the query, filter by `committer_date` range, search across both embed types.

### Hybrid: commit + diff

Search `commit_message` for intent, then fetch related `file_diff` vectors by `commit_id` for detail.

## Lifecycle Operations

| Operation | When |
|-----------|------|
| Create collection | First commit ingestion for a project/embedding version combo |
| Upsert vectors | During commit ingestion worker step |
| Delete by commit_id | If commits are re-ingested or corrected |
| Drop collection | When project is deleted or embedding version retired |

## Capacity Heuristics

- 1k commits with avg 5 files changed each = ~6k vectors (1k message + 5k diff)
- 10k commits with avg 5 files = ~60k vectors
- 50k commits with avg 5 files = ~300k vectors

Significantly smaller than code chunk collections. Memory impact is modest.

## Truncation Strategy

Large diffs (generated files, lock files, vendor code) should be truncated before embedding:

- **Patch text budget:** ~4000 tokens (~16KB) per file diff
- **Skip embedding** for diffs exceeding a configurable max (e.g., 100KB raw patch)
- **Skip embedding** for binary files or files matching ignore patterns (e.g., `*.lock`, `vendor/`)

The truncation is applied at embedding time, not at storage time. Full patches remain in PostgreSQL `commit_file_diffs.patch`.
