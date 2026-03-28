# 15 — Actions Tab & Unified Job Trigger

## Status
Pending

## Goal
Rename the "Indexing" tab to "Actions" and add a job trigger interface inside the tab that supports all job types (indexing and description). Update the tRPC router and header trigger button to use the new unified `POST /v1/projects/{id}/action` endpoint. Extend the job table to display description job types with appropriate badges.

## Depends On
06-indexing-monitor, backend-api/18-unified-action-endpoint

## Scope

### Tab Rename

Update the tab label in the project detail layout from "Indexing" to "Actions". The route stays at `/project/{id}/jobs` (no URL change needed — "jobs" is still accurate as the data model).

**File**: `src/app/(app)/project/[id]/layout.tsx`

```typescript
// Before:
{ label: "Indexing", href: `/project/${id}/jobs` }

// After:
{ label: "Actions", href: `/project/${id}/jobs` }
```

### Action Trigger Section

Add a card at the top of the jobs page (above the jobs table) with grouped action buttons. This replaces the need to solely rely on the header split button for triggering jobs.

**Layout**: A card with two sections separated by a subtle divider:

**Indexing section:**
- "Incremental Index" button (primary, default)
- "Full Index" button (outline)

**Description section:**
- "Describe All Files" button (outline)
- "Describe Changed Files" button (outline)

All buttons:
- Call `triggerAction.mutate({ projectId, type: "..." })`
- Disabled when a job of the same type is already active (queued/running)
- Show loading spinner during mutation
- On success: toast + invalidate `listJobs` query

The section is collapsible (default expanded) with a "New Action" header and a `Plus` icon. This keeps the table visible without scrolling on smaller screens.

**Component**: Add action trigger UI directly in `jobs-content.tsx` or extract to a new `action-trigger-card.tsx` component.

### Header Split Button Update

Update the project detail header split button to use the new `/action` endpoint:

**Before:**
```typescript
triggerIndex.mutate({ projectId, job_type: "incremental" })
// Calls POST /v1/projects/{id}/index
```

**After:**
```typescript
triggerAction.mutate({ projectId, type: "incremental" })
// Calls POST /v1/projects/{id}/action
```

The split button keeps its current behavior (default = incremental index, dropdown = full index). Description triggers are only available in the Actions tab interface, not in the header — the header stays focused on the most common action.

### tRPC Router Update (`project-indexing.ts`)

**Rename procedure**: `triggerIndex` → `triggerAction`

```typescript
triggerAction: publicProcedure
  .input(z.object({
    projectId: z.string(),
    type: z.enum([
      "full", "incremental",
      "describe-full", "describe-incremental", "describe-file",
    ]),
    file_path: z.string().optional(),
  }))
  .mutation(async ({ input, ctx }) => {
    const res = await apiCall<IndexJob>(
      `v1/projects/${input.projectId}/action`,
      {
        method: "POST",
        body: JSON.stringify({
          type: input.type,
          file_path: input.file_path,
        }),
        headers: ctx.headers,
      },
    );
    return res;
  }),
```

### Extended `IndexJob` Type

Add new job types and columns to match the backend changes:

```typescript
const IndexJobSchema = z.object({
  id: z.string(),
  project_id: z.string(),
  index_snapshot_id: z.string().nullable(),
  job_type: z.enum([
    "full", "incremental",
    "describe-full", "describe-incremental", "describe-file",
  ]),
  status: z.enum(["queued", "running", "completed", "failed"]),
  files_processed: z.number(),
  chunks_upserted: z.number(),
  vectors_deleted: z.number(),
  descriptions_generated: z.number(),
  target_file_path: z.string().nullable(),
  error_details: z.array(ErrorDetailSchema),
  worker_id: z.string().nullable(),
  embedding_provider_config_id: z.string().nullable(),
  llm_provider_config_id: z.string().nullable(),
  started_at: z.string().nullable(),
  finished_at: z.string().nullable(),
  created_at: z.string(),
});
```

### Job Table Updates (`jobs-content.tsx`)

**Type column badges**: Extend the type badge rendering to support description job types:

| `job_type` | Badge Label | Badge Style |
|---|---|---|
| `full` | Full Index | outline (existing) |
| `incremental` | Incremental Index | outline (existing) |
| `describe-full` | Describe All | outline, purple tint |
| `describe-incremental` | Describe Changed | outline, purple tint |
| `describe-file` | Describe File | outline, purple tint |

**Descriptions column**: Add a "Described" column after "Deleted" showing `descriptions_generated` (only visible when any job in the list has `descriptions_generated > 0`, to avoid clutter for projects that haven't used description yet).

**Target file path**: For `describe-file` jobs, show the `target_file_path` in a tooltip on the type badge or as a subtitle below the badge.

### Mutations Hook Update (`use-project-detail-mutations.ts`)

Rename `triggerIndex` to `triggerAction` and update the mutation call:

```typescript
const triggerAction = api.projectIndexing.triggerAction.useMutation({
  onSuccess: () => {
    toast.success("Action started");
    void utils.projectIndexing.listJobs.invalidate();
  },
  onError: (error) => {
    toast.error(`Failed to start action: ${error.message}`);
  },
});
```

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/project/[id]/layout.tsx` | Rename tab "Indexing" → "Actions" |
| `src/components/project-detail/jobs-content.tsx` | Action trigger card, extended job table, description badges |
| `src/components/project-detail/project-detail-header.tsx` | Update split button to use `triggerAction` |
| `src/server/api/routers/project-indexing.ts` | `triggerAction` procedure, extended `IndexJob` type |
| `src/hooks/use-project-detail-mutations.ts` | Rename mutation, update invalidation |

## Acceptance Criteria
- [ ] Tab label changed from "Indexing" to "Actions"
- [ ] Action trigger card shown at top of actions tab with grouped buttons
- [ ] Indexing group: "Incremental Index" and "Full Index" buttons
- [ ] Description group: "Describe All Files" and "Describe Changed Files" buttons
- [ ] All trigger buttons call `POST /v1/projects/{id}/action` with correct `type`
- [ ] Buttons disabled when a job of the same type is already active
- [ ] Header split button updated to use `/action` endpoint (default: incremental, dropdown: full)
- [ ] `triggerIndex` renamed to `triggerAction` in tRPC router and mutations hook
- [ ] `IndexJob` type extended with `describe-*` job types, `descriptions_generated`, `target_file_path`
- [ ] Job table shows description job types with purple-tinted badges
- [ ] "Described" column shown conditionally when description jobs exist
- [ ] `describe-file` jobs show `target_file_path` in tooltip
- [ ] Success/error toasts for all action triggers
- [ ] Auto-refresh polling still works for describe jobs (queued/running status)
- [ ] SSE-driven updates still invalidate the jobs query
- [ ] No regressions on existing indexing job display
