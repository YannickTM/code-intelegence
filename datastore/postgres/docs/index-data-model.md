# Index Data Model

## Purpose

Extends the datastore with three capabilities:

1. **File content storage** — raw source text and tree-sitter AST, deduplicated across snapshots
2. **Git commit history** — commits, parent relationships, and per-file diffs
3. **Commit/diff embeddings** — Qdrant vectors for semantic search over change history

## Tables

### `file_contents`

Content-addressable file storage, deduplicated by `(project_id, content_hash)`.

| Column | Type | Notes |
|--------|------|-------|
| id | UUID PK | |
| project_id | UUID FK → projects | CASCADE delete |
| content_hash | TEXT | SHA-256, matches the parser engine `file_hash` |
| content | TEXT | Raw source text |
| tree_sitter_ast | JSONB | Raw AST from tree-sitter, nullable (set after parse) |
| size_bytes | BIGINT | |
| line_count | INT | |
| created_at | TIMESTAMPTZ | |

**Unique constraint:** `(project_id, content_hash)`

Multiple `files` rows across different snapshots point to the same `file_contents` row when the file hasn't changed. This avoids storing duplicate content for a 10k-file repo across 100 snapshots.

The `files` table gains a `file_content_id UUID` FK column linking to this table.

### `commits`

Git commits per project.

| Column | Type | Notes |
|--------|------|-------|
| id | UUID PK | |
| project_id | UUID FK → projects | CASCADE delete |
| commit_hash | TEXT | Full SHA |
| author_name | TEXT | |
| author_email | TEXT | |
| author_date | TIMESTAMPTZ | |
| committer_name | TEXT | |
| committer_email | TEXT | |
| committer_date | TIMESTAMPTZ | |
| message | TEXT | Full commit message |
| created_at | TIMESTAMPTZ | |

**Unique constraint:** `(project_id, commit_hash)`

**Indexes:**
- `(project_id, committer_date DESC)` — chronological listing
- `(project_id, commit_hash)` — direct lookup

### `commit_parents`

Junction table for commit parent relationships. Supports merge commits (multiple parents).

| Column | Type | Notes |
|--------|------|-------|
| project_id | UUID FK → projects | CASCADE delete, part of PK |
| commit_id | UUID | Composite FK → commits(project_id, id), CASCADE delete |
| parent_commit_id | UUID | Composite FK → commits(project_id, id), CASCADE delete |
| ordinal | INT | 0 = first parent, 1 = merge parent, etc. |

**Primary key:** `(project_id, commit_id, parent_commit_id)`
**Unique:** `(project_id, commit_id, ordinal)`

Uses a junction table instead of `TEXT[]` for referential integrity and efficient "find children of commit X" queries.

### `commit_file_diffs`

Per-file diffs between a commit and its parent.

| Column | Type | Notes |
|--------|------|-------|
| id | UUID PK | |
| project_id | UUID FK → projects | CASCADE delete |
| commit_id | UUID | Composite FK → commits(project_id, id), CASCADE delete |
| parent_commit_id | UUID | Composite FK → commits(project_id, id), SET NULL on delete; NULL for root commit |
| old_file_path | TEXT | NULL for added files |
| new_file_path | TEXT | NULL for deleted files |
| change_type | TEXT | `added`, `modified`, `deleted`, `renamed`, `copied` |
| patch | TEXT | Unified diff text; NULL for binary or omitted diffs |
| additions | INT | Line count |
| deletions | INT | Line count |
| created_at | TIMESTAMPTZ | |

**Check constraint:** `change_type IN ('added', 'modified', 'deleted', 'renamed', 'copied')`

### Altered Tables

**`files`** — new column:
- `file_content_id UUID` — Composite FK → `file_contents(project_id, id)`, ON DELETE SET NULL

**`index_snapshots`** — new column:
- `commit_id UUID` — Composite FK → `commits(project_id, id)`, ON DELETE SET NULL

The existing `git_commit TEXT` column on `index_snapshots` remains for backward compatibility. The `commit_id` FK provides a join path to full commit metadata.

## Entity Relationships

```text
projects
 ├── 1:N → file_contents (content-addressable)
 ├── 1:N → commits
 │           ├── N:M → commit_parents (self-join)
 │           └── 1:N → commit_file_diffs
 │
 └── 1:N → index_snapshots
              ├── 0:1 → commits (via commit_id FK)
              └── 1:N → files
                          └── 0:1 → file_contents (via file_content_id FK)
```

## Qdrant Collection

Commit and diff embeddings live in a **separate collection** from code chunk vectors:

```text
project_{project_id}__commits__emb_{embedding_version}
```

Two embed types per collection:

| embed_type | Source | Granularity |
|------------|--------|-------------|
| `commit_message` | `commits.message` | One vector per commit |
| `file_diff` | `commit_file_diffs.patch` | One vector per file diff |

Payload schema:

```json
{
  "project_id": "uuid",
  "commit_id": "uuid",
  "commit_hash": "abc123",
  "embed_type": "commit_message | file_diff",
  "committer_date": "2026-03-10T12:00:00Z",
  "message_preview": "fix: auth token expiry",
  "old_file_path": "src/auth/token.ts",
  "new_file_path": "src/auth/token.ts",
  "change_type": "modified",
  "additions": 12,
  "deletions": 5,
  "diff_id": "uuid",
  "raw_text_ref": "db://commits/{id} | db://commit_file_diffs/{id}"
}
```

Full diff/message text stays in PostgreSQL. Qdrant payload is lean, following the existing code chunk collection pattern.

Vector config matches existing collections: 768 dimensions, Cosine distance, HNSW m=16, ef_construct=128.

Filterable payload fields: `project_id`, `embed_type`, `commit_id`, `old_file_path`, `new_file_path`, `change_type`, `committer_date`.

> **Note:** Branch is a project-level concept (stored on `projects.default_branch` and `index_snapshots.branch`). It is not included in per-commit/per-diff Qdrant payloads since each project covers a single branch.

## Query Reference

Queries are split across two files:
- `datastore/postgres/queries/commits.sql` — commits, parents, diffs, snapshot link
- `datastore/postgres/queries/files.sql` — file content queries (task 22: `GetActiveSnapshotForProject`, `ListSnapshotFiles`, `GetFileByPath`, `GetFileWithContent`, `GetFileWithContentAndAST`)

### File Contents

| Query | Type | Purpose |
|-------|------|---------|
| `UpsertFileContent` | :one | Insert or return existing by `(project_id, content_hash)` |
| `GetFileContentByHash` | :one | Lookup by project + hash |
| `GetFileContentByID` | :one | Lookup by PK |
| `SetFileContentAST` | :exec | Set `tree_sitter_ast` JSONB after parse |
| `UpdateFileContentRef` | :exec | Set `file_content_id` on a `files` row |
| `DeleteProjectFileContents` | :exec | Cascade cleanup |

### Commits

| Query | Type | Purpose |
|-------|------|---------|
| `InsertCommit` | :one | Upsert by `(project_id, commit_hash)` |
| `GetCommitByHash` | :one | Lookup by project + hash |
| `GetCommitByID` | :one | Lookup by PK |
| `ListProjectCommits` | :many | Paginated, ordered by `committer_date DESC` |
| `CountProjectCommits` | :one | Total count for a project |
| `ListCommitsBetween` | :many | Range query by committer date |
| `DeleteProjectCommits` | :exec | Cascade cleanup (parents, diffs follow) |

### Commit Parents

| Query | Type | Purpose |
|-------|------|---------|
| `InsertCommitParent` | :exec | Idempotent via `ON CONFLICT DO NOTHING` |
| `GetCommitParents` | :many | Parents of a commit, ordered by ordinal |
| `GetCommitChildren` | :many | Children of a commit |

### Commit File Diffs

| Query | Type | Purpose |
|-------|------|---------|
| `InsertCommitFileDiff` | :one | Idempotent upsert by `(project_id, commit_id, old_file_path, new_file_path)` |
| `ListCommitFileDiffs` | :many | Paginated diffs for a commit, ordered by file path |
| `CountCommitFileDiffs` | :one | Total diff count for pagination |
| `GetCommitDiffStats` | :one | Aggregate: files_changed, total_additions, total_deletions |
| `ListFileDiffsByPath` | :many | Paginated history of changes to a specific file path |

### Snapshot Link

| Query | Type | Purpose |
|-------|------|---------|
| `UpdateSnapshotCommitID` | :exec | Set `commit_id` FK on an index snapshot |

## Data Flow

### Commit Ingestion (new worker step, runs after repo fetch)

1. `git log --format=...` extracts commit metadata since last known commit
2. `InsertCommit` per commit (idempotent upsert)
3. `InsertCommitParent` for each parent relationship (ordinal 0 = first parent)
4. Compute file diffs:
   - **Root commit** (no parents): `git diff-tree --root -r <commit>` — diff against empty tree; set `parent_commit_id = NULL`
   - **Normal/merge commit**: `git diff first_parent..commit` — diff against first parent only; set `parent_commit_id` to first parent
5. `InsertCommitFileDiff` per changed file (idempotent upsert)

> **Merge commits:** Only first-parent diffs are stored. This matches `git log --first-parent` semantics and avoids duplicate diff entries. The full parent list is preserved in `commit_parents` for graph traversal.

### File Content Storage (enhancement to existing index flow)

1. The embedded parser engine computes SHA-256 `file_hash` during parsing
2. `UpsertFileContent` stores raw content (deduplicated by hash)
3. After tree-sitter parse: `SetFileContentAST` stores raw AST JSONB
4. `UpdateFileContentRef` links `files.file_content_id` to the content row

### Commit/Diff Embedding (new worker step, after commit ingestion)

1. Batch embed `commits.message` text, upsert to Qdrant with `embed_type=commit_message`
2. Batch embed `commit_file_diffs.patch` text (truncated to token budget), upsert with `embed_type=file_diff`
