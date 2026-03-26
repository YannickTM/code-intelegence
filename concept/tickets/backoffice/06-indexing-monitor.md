# 06 — Indexing Jobs Monitor

## Status
Done

## Goal
Built the Indexing (Jobs) tab for the project detail view. Shows job history in a table with status badges, auto-refresh while jobs are active, expandable error details for failed jobs, and SSE-driven live updates. Also fixed the tRPC router and header trigger button to align with the backend API contract.

## Depends On
03-projects, 02-dashboard (SSE infrastructure)

## Scope

### Jobs Page (`/projects/[id]/jobs`)

Replaces the placeholder "Indexing" tab in the project detail view.

**Jobs Table** (shadcn Table):

| Column | Source | Render |
|---|---|---|
| Type | `job_type` | Outline badge: `Full` / `Incremental` |
| Status | `status` | Badge: queued (grey), running (blue + pulse animation), completed (green), failed (red/destructive) |
| Files | `files_processed` | Number |
| Chunks | `chunks_upserted` | Number |
| Deleted | `vectors_deleted` | Number |
| Started | `started_at` | Relative time; tooltip full date; "--" if null |
| Duration | computed | `finished_at - started_at` formatted as "Xm Ys"; "Running..." if in progress; "--" if not started |
| Created | `created_at` | Relative time; tooltip full date |

**Status Badge Styles**:
- `queued`: outline variant (grey/default)
- `running`: blue background with `animate-pulse`
- `completed`: green background
- `failed`: destructive variant (red)

**Expandable Error Details**: failed rows with `error_details.length > 0` are clickable. Expanding reveals a muted background section with monospace text, each error on a separate line. Click again to collapse. Only failed rows are expandable.

### Auto-Refresh and Live Updates

**Polling Fallback**: `refetchInterval: 3000` enabled when any job has status `queued` or `running`. Disabled when all jobs are completed or failed.

```typescript
const hasActiveJobs = data?.items.some(
  (job) => job.status === "queued" || job.status === "running"
);
refetchInterval: hasActiveJobs ? 3000 : false
```

**SSE-Driven Updates**: job events (`job:queued`, `job:started`, `job:progress`, `job:completed`, `job:failed`) trigger `utils.projectIndexing.listJobs.invalidate()` via the global SSE hook (implemented in ticket 02). Both mechanisms coexist; SSE provides faster reactivity, polling is the fallback.

### Trigger Buttons

The "Trigger Index" split button lives in the project header (persistent across tabs). This ticket fixed its behavior:
- Default button: `triggerIndex.mutate({ projectId, job_type: "incremental" })`
- Dropdown "Full Index": `triggerIndex.mutate({ projectId, job_type: "full" })`
- Dropdown "Incremental Index": `triggerIndex.mutate({ projectId, job_type: "incremental" })`
- Entire split button disabled when `healthStatus === "indexing"` (job already active)

On trigger success, `projectIndexing.listJobs` query is invalidated to show the new job immediately.

### Pagination

- Default page size: 20 items
- Total <= 20: no pagination controls
- Total <= 120 (up to 6 pages): "Load more" button at bottom (append mode)
- Total > 120: page-number navigation (replace mode)

### tRPC Router Fixes

Fixed `src/server/api/routers/project-indexing.ts`:
1. Removed `"rebuild"` from `IndexJob.job_type` union (backend only accepts `"full"` / `"incremental"`)
2. Added `embedding_provider_config_id` and `llm_provider_config_id` to `IndexJob` type
3. Changed `triggerIndex` input from `force_full: boolean` to `job_type: enum("full", "incremental")`
4. Added `total`, `limit`, `offset` to `IndexJobListResponse` type
5. Added `limit` and `offset` to `listJobs` input schema (passed as query params)

### tRPC Procedures

| Procedure | Backend Endpoint | Purpose |
|---|---|---|
| `projectIndexing.listJobs` | `GET /v1/projects/{id}/jobs` | Paginated job list |
| `projectIndexing.triggerIndex` | `POST /v1/projects/{id}/index` | Trigger full or incremental index |

### States

- **Loading**: skeleton table rows
- **Empty**: "No indexing jobs yet" with subtext referencing the header trigger button
- **Error**: inline alert with retry button

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/projects/[id]/jobs/page.tsx` | Server page rendering JobsContent |
| `src/app/(app)/projects/[id]/components/jobs-content.tsx` | Jobs table, auto-refresh, expandable errors, pagination |
| `src/app/(app)/projects/[id]/components/project-detail-header.tsx` | Fixed trigger mutations to use `job_type` |
| `src/app/(app)/projects/[id]/lib/use-project-detail-mutations.ts` | Added listJobs invalidation on trigger success |
| `src/server/api/routers/project-indexing.ts` | Fixed types and input schemas |

## Acceptance Criteria
- [x] Jobs table renders with type, status, files, chunks, deleted, started, duration, created columns
- [x] Status badges: queued (grey), running (blue pulse), completed (green), failed (red)
- [x] Duration calculated from `started_at`/`finished_at`; shows "Running..." for active jobs
- [x] Failed rows expandable to show `error_details` in monospace text
- [x] Auto-refresh polling at 3s intervals when active jobs exist
- [x] SSE events trigger query invalidation for immediate updates
- [x] Trigger button sends `job_type: "incremental"` by default, `"full"` via dropdown
- [x] Split button disabled when a job is already running
- [x] Pagination: load-more for small histories, page numbers for large
- [x] `IndexJob` type corrected: no "rebuild", includes config ID fields
- [x] `triggerIndex` input uses `job_type` instead of `force_full`
- [x] `listJobs` supports `limit`/`offset` query params
- [x] Empty state and loading skeleton rendered correctly
