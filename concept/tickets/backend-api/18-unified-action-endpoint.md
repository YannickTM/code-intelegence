# 18 — Unified Action Endpoint & Description Read Endpoints

## Status
Pending

## Goal
Replace `POST /v1/projects/{id}/index` with a unified `POST /v1/projects/{id}/action` endpoint that supports all job types (`full`, `incremental`, `describe-full`, `describe-incremental`, `describe-file`) via a `type` field. Add read endpoints for file descriptions. Extend the queue publisher to support describe workflows.

## Depends On
- Backend-Worker 16-description-schema (migration must be applied)

## Scope

### Unified Endpoint (`POST /v1/projects/{id}/action`)

Replace `HandleIndex` with `HandleAction`. The endpoint accepts a `type` field instead of `job_type`:

```json
{
  "type": "describe-full",
  "file_path": "internal/handler/auth.go"
}
```

`file_path` is required when `type` is `describe-file`, ignored otherwise.

Valid types:
- `full` — full indexing (existing behavior)
- `incremental` — incremental indexing (existing behavior)
- `describe-full` — describe all files
- `describe-incremental` — describe changed files
- `describe-file` — describe single file

### HandleAction Flow

```go
func (h *ProjectHandler) HandleAction(w http.ResponseWriter, r *http.Request) {
    // 1. Parse project ID
    // 2. Parse request body: { "type": "...", "file_path": "..." }
    //    Default type: "full" (backward compat for empty body)
    // 3. Validate type via MapJobTypeToWorkflow
    // 4. If type is "describe-file":
    //    a. Validate file_path is present (400 if missing)
    //    b. Validate file exists in the active snapshot (404 if not found)
    //
    // --- Prerequisites (type-dependent) ---
    //
    // 5. isDescribe := strings.HasPrefix(type, "describe-")
    //    isIndex := !isDescribe
    //
    // 6. SSH key check: required for index types only
    //    Describe jobs don't clone repos — they read from PostgreSQL
    //
    // 7. Resolve embedding config (required for all types):
    //    Index needs it for chunk embedding; describe needs it for description embedding
    //
    // 8. Resolve LLM config:
    //    - Index types: optional (NULL if not configured)
    //    - Describe types: required (409 if not resolved)
    //
    // 9. Describe types only: validate active snapshot exists (409 if not)
    //    Can't describe without indexed data
    //
    // --- Job creation (same for all types) ---
    //
    // 10. Dedup: FindActiveIndexingJobForProjectAndType(projectID, type, targetFilePath)
    //     For describe-file: dedup includes target_file_path so different files can be described concurrently
    // 11. Create indexing_jobs row with:
    //     - job_type = type
    //     - embedding_provider_config_id = resolved
    //     - llm_provider_config_id = resolved (or NULL for index types)
    //     - target_file_path = file_path (only for describe-file, NULL otherwise)
    // 12. Enqueue via publisher
    // 13. Handle enqueue failure (mark job failed)
    // 14. Return 202 Accepted with job record
}
```

### Route Change (`internal/app/routes.go`)

```go
// Before:
r.With(adminAccess).Post("/index", a.project.HandleIndex)

// After:
r.With(adminAccess).Post("/action", a.project.HandleAction)
```

### Active Snapshot Validation Query

```sql
-- name: HasActiveSnapshotForProject :one
SELECT EXISTS (
  SELECT 1 FROM index_snapshots
  WHERE project_id = $1 AND is_active = TRUE
) AS has_active;
```

### Queue Publisher Extension (`internal/queue/publisher.go`)

Extend `MapJobTypeToWorkflow`:

```go
func MapJobTypeToWorkflow(jobType string) (string, error) {
    switch jobType {
    case "full":
        return "full-index", nil
    case "incremental":
        return "incremental-index", nil
    case "describe-full":
        return "describe-full", nil
    case "describe-incremental":
        return "describe-incremental", nil
    case "describe-file":
        return "describe-file", nil
    default:
        return "", fmt.Errorf("queue: unknown job type: %q", jobType)
    }
}
```

Add `DescribeTimeout` to `PublisherConfig`. Select timeout based on workflow prefix in `EnqueueIndexJob`:

```go
timeout := p.cfg.IndexFullTimeout
switch {
case workflow == "incremental-index":
    timeout = p.cfg.IndexIncrementalTimeout
case strings.HasPrefix(workflow, "describe-"):
    timeout = p.cfg.DescribeTimeout
}
```

### CreateIndexingJob Extension

The existing `CreateIndexingJob` query needs to accept the new `target_file_path` column:

```sql
-- name: CreateIndexingJob :one
INSERT INTO indexing_jobs (
  project_id, index_snapshot_id, job_type,
  embedding_provider_config_id, llm_provider_config_id,
  target_file_path
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
```

### Description Read Endpoints

**`GET /v1/projects/{id}/files/description?file_path=...`** (member+ access)

Returns the file description for the specified path from the active snapshot.

```go
func (h *ProjectHandler) HandleFileDescription(w http.ResponseWriter, r *http.Request) {
    // 1. Parse project ID
    // 2. Get active snapshot for project (404 if none)
    // 3. Read file_path query param (required, 400 if missing)
    // 4. GetFileDescription(projectID, snapshotID, filePath)
    // 5. Return description JSON or 404
}
```

**`GET /v1/projects/{id}/descriptions`** (member+ access)

Paginated list of file descriptions for the active snapshot. Query params: `limit` (default 20, max 100), `offset`, `file_role`, `language`, `confidence`.

**`GET /v1/projects/{id}/descriptions/summary`** (member+ access)

Aggregate statistics: total files described, role distribution, confidence distribution, generation cost summary (total input/output tokens, total and average latency).

### Route Registration

```go
// Member-level read (within existing member access group):
r.Get("/files/description", a.project.HandleFileDescription)
r.Get("/descriptions", a.project.HandleListDescriptions)
r.Get("/descriptions/summary", a.project.HandleDescriptionsSummary)
```

### MCP Tool Extension

Add `get_file_description` tool mapping to the MCP tool schema:
- Backend: `GET /v1/projects/{project_id}/files/description?file_path={path}`
- Required: `project_id`, `file_path`
- Returns: `summary`, `description`, `file_role`, `key_symbols`, `architectural_notes`

### Error Responses

| Condition | Status | Error |
|---|---|---|
| Invalid `type` | 400 | Validation error: must be one of the valid types |
| `describe-file` without `file_path` | 400 | Validation error: `file_path` required |
| `describe-file` with unknown `file_path` | 404 | `"file not found in active snapshot"` |
| No active SSH key (index types) | 409 | `"project has no active SSH key assignment"` |
| No embedding config | 409 | `"project has no embedding provider configured"` |
| No LLM config (describe types) | 409 | `"project has no LLM provider configured"` |
| No active snapshot (describe types) | 409 | `"project has no active index snapshot — run indexing first"` |
| Enqueue failure | 500 | Job marked failed with `"stage": "enqueue"` in `error_details` |

## Key Files

| File | Purpose |
|---|---|
| `backend-api/internal/handler/project.go` | `HandleAction` (replaces `HandleIndex`), `HandleFileDescription`, `HandleListDescriptions`, `HandleDescriptionsSummary` |
| `backend-api/internal/queue/publisher.go` | Extend `MapJobTypeToWorkflow`, add `DescribeTimeout` to config, update timeout selection |
| `backend-api/internal/app/routes.go` | Replace `/index` route with `/action`, add description read routes |
| `datastore/postgres/queries/indexing.sql` | `HasActiveSnapshotForProject`, extend `CreateIndexingJob` with `target_file_path` |
| `datastore/postgres/queries/descriptions.sql` | Read queries (from worker ticket 16) |
| `backend-api/internal/handler/project.go` (`HandleIndex`) | Reference pattern being replaced |

## Acceptance Criteria
- [ ] `POST /v1/projects/{id}/action` replaces `POST /v1/projects/{id}/index`
- [ ] Request body accepts `type` field with all 5 valid values
- [ ] Default type is `"full"` when body is empty or type is omitted (backward compat)
- [ ] `"describe-file"` type requires `file_path`; rejected with 400 if missing
- [ ] `"describe-file"` validates file exists in the active snapshot; rejected with 404 if not found
- [ ] SSH key check runs only for index types (`full`, `incremental`), not describe types
- [ ] Embedding config required for all types
- [ ] LLM config required for describe types (409 if not resolved); optional for index types
- [ ] Active snapshot validated for describe types (409 if missing)
- [ ] `target_file_path` stored on job row for `describe-file`
- [ ] Dedup works per `(project_id, type)` for most types; `describe-file` dedup includes `target_file_path` so different files can be described concurrently
- [ ] Description jobs are blocked at the worker level (advisory lock) while an indexing job is running on the same project — descriptions depend on indexed data
- [ ] `MapJobTypeToWorkflow` maps all 5 types to correct workflow names
- [ ] Describe workflow timeout configurable via `DescribeTimeout` in `PublisherConfig`
- [ ] `GET /v1/projects/{id}/files/description?file_path=X` returns single description or 404
- [ ] `GET /v1/projects/{id}/descriptions` returns paginated list with optional filtering by `file_role`, `language`, `confidence`
- [ ] `GET /v1/projects/{id}/descriptions/summary` returns aggregate statistics
- [ ] All read endpoints use the active snapshot; 404 if no active snapshot
- [ ] Read routes registered with member+ access; action route with admin+ access
- [ ] Response format consistent with existing project endpoint patterns
- [ ] `CreateIndexingJob` accepts `target_file_path` parameter
