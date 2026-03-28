# 16 — File Description Card & Detail Modal

## Status
Pending

## Goal
Replace the mocked `AiDescriptionCard` with a real implementation that fetches file descriptions from the backend API, triggers single-file description generation, and provides a modal dialog for viewing the full description with metadata.

## Depends On
15-actions-tab, backend-api/18-unified-action-endpoint, backend-worker/16-description-schema

## Scope

### tRPC Procedure (`project-files.ts`)

Add a `fileDescription` query procedure:

```typescript
fileDescription: publicProcedure
  .input(z.object({
    projectId: z.string(),
    filePath: z.string(),
  }))
  .query(async ({ input, ctx }) => {
    const res = await apiCall<FileDescription>(
      `v1/projects/${input.projectId}/files/description?file_path=${encodeURIComponent(input.filePath)}`,
      { headers: ctx.headers },
    );
    return res;
  }),
```

**`FileDescription` type:**

```typescript
const FileDescriptionSchema = z.object({
  id: z.string(),
  file_path: z.string(),
  language: z.string().nullable(),
  file_role: z.string().nullable(),
  summary: z.string(),
  description: z.string(),
  key_symbols: z.array(z.object({
    name: z.string(),
    kind: z.string(),
    role: z.string(),
  })),
  imports_summary: z.string().nullable(),
  consumers_summary: z.string().nullable(),
  architectural_notes: z.string().nullable(),
  confidence: z.string(),
  uncertainty_notes: z.string().nullable(),
  generation_metadata: z.object({
    model: z.string().optional(),
    prompt_version: z.string().optional(),
    input_tokens: z.number().optional(),
    output_tokens: z.number().optional(),
    latency_ms: z.number().optional(),
    generated_at: z.string().optional(),
    job_id: z.string().optional(),
  }),
  content_hash: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});
```

The query returns 404 when no description exists for the file. The card handles this gracefully (shows the "Request" button state).

### Card Component Rewrite (`file-viewer-content.tsx`)

Replace the mocked `AiDescriptionCard` with a real implementation. Extract to `src/components/project-detail/file-description-card.tsx`.

**Props:**

```typescript
interface FileDescriptionCardProps {
  projectId: string;
  filePath: string;
}
```

**States:**

1. **Loading**: Skeleton placeholder while the query is in flight
2. **No description**: File has no description yet (404 response). Show prompt text and "Request AI Description" button
3. **Generating**: A `describe-file` job was just triggered and is pending. Show spinner with "Generating description…"
4. **Has description**: Description exists. Show summary, file role badge, and "View Details" / "Regenerate" buttons
5. **Error**: Query failed (non-404). Show inline error with retry

### Card Layout (Has Description State)

```
┌─────────────────────────────────────┐
│ ✨ AI Description          [high]   │  ← confidence badge
├─────────────────────────────────────┤
│ ┌─────────┐                         │
│ │ handler │  ← file_role badge      │
│ └─────────┘                         │
│                                     │
│ HTTP handler for user auth...       │  ← summary text
│                                     │
│ ┌───────────────┐ ┌────────────┐    │
│ │ View Details  │ │ Regenerate │    │
│ └───────────────┘ └────────────┘    │
│                                     │
│ Generated 2h ago · llama3.1         │  ← metadata footer
└─────────────────────────────────────┘
```

- **Confidence badge**: `high` (green), `medium` (amber), `low` (red) — shown in card header
- **File role badge**: outline badge with the `file_role` value
- **Summary**: the `summary` field, shown in full (typically under 50 words)
- **View Details button**: opens the description detail modal
- **Regenerate button**: ghost button, triggers `triggerAction` with `type: "describe-file"`
- **Metadata footer**: relative time from `generation_metadata.generated_at` + model name, muted text

### Request Button (No Description State)

The "Request AI Description" button triggers a single-file description job:

```typescript
triggerAction.mutate({
  projectId,
  type: "describe-file",
  file_path: filePath,
});
```

On success:
- Toast: "Description requested"
- Switch to "Generating" state
- Poll the `fileDescription` query (set `refetchInterval: 3000` while in generating state)
- When the description appears, transition to "Has description" state

### Description Detail Modal

A `Dialog` component that shows the full description with all structured fields. Opened by the "View Details" button.

**Component**: `src/components/project-detail/file-description-dialog.tsx`

**Layout:**

```
┌──────────────────────────────────────────────┐
│  AI Description — auth.go              [×]   │
├──────────────────────────────────────────────┤
│                                              │
│  Role: http_handler    Confidence: high      │
│                                              │
│  Summary                                     │
│  ─────────                                   │
│  HTTP handler for user authentication...     │
│                                              │
│  Description                                 │
│  ───────────                                 │
│  This file implements the auth HTTP handler  │
│  for the backend API. It exposes three...    │
│                                              │
│  Key Symbols                                 │
│  ───────────                                 │
│  • HandleLogin (function) — Processes login  │
│  • HandleLogout (function) — Invalidates...  │
│                                              │
│  Imports                                     │
│  ───────                                     │
│  Depends on auth service, session store...   │
│                                              │
│  Consumers                                   │
│  ─────────                                   │
│  Consumed by route registration in routes.go │
│                                              │
│  Architectural Notes                         │
│  ────────────────────                        │
│  Part of the handler layer in the backend... │
│                                              │
│  ┌─────────────────────────────────────────┐ │
│  │ Generation Info                         │ │
│  │ Model: llama3.1  Tokens: 2400→350      │ │
│  │ Latency: 1.2s  Generated: 2h ago       │ │
│  │ Prompt version: v1                      │ │
│  └─────────────────────────────────────────┘ │
│                                              │
│                            [ Regenerate ]    │
└──────────────────────────────────────────────┘
```

- Dialog uses `sm:max-w-2xl` for comfortable reading width
- Sections only shown when their data is non-null/non-empty
- Key symbols rendered as a compact list with monospace names
- Generation info in a muted collapsible section at the bottom
- "Regenerate" button in the dialog footer

### Card Placement

The `FileDescriptionCard` replaces the inline `AiDescriptionCard()` function in `file-viewer-content.tsx`. It stays in the same position: between "File Info" and "File Facts" cards in the right sidebar.

### Polling Strategy

When a description job is triggered from this card:
1. Set a local `isGenerating` state flag
2. Enable `refetchInterval: 3000` on the `fileDescription` query
3. When the query returns data (not 404), clear `isGenerating` and disable polling
4. Timeout after 5 minutes: clear `isGenerating`, show a "Generation timed out" message with retry

This avoids requiring SSE integration for a single-file operation — simple polling is sufficient.

## Key Files

| File | Purpose |
|---|---|
| `src/components/project-detail/file-description-card.tsx` | Real AI description card with all states |
| `src/components/project-detail/file-description-dialog.tsx` | Full description modal dialog |
| `src/components/project-detail/file-viewer-content.tsx` | Replace mocked `AiDescriptionCard` with real component |
| `src/server/api/routers/project-files.ts` | `fileDescription` query procedure and `FileDescription` type |
| `src/hooks/use-project-detail-mutations.ts` | `triggerAction` mutation (from ticket 15) |

## Acceptance Criteria
- [ ] Mocked `AiDescriptionCard` replaced with real implementation
- [ ] `projectFiles.fileDescription` query fetches from `GET /v1/projects/{id}/files/description`
- [ ] Card shows skeleton loading state while query is in flight
- [ ] Card shows "Request AI Description" button when no description exists (404)
- [ ] Request button triggers `describe-file` job via `triggerAction` mutation
- [ ] Card shows "Generating description…" spinner after job is triggered
- [ ] Polling (`refetchInterval: 3000`) activates during generation
- [ ] Card transitions to "has description" state when description becomes available
- [ ] Summary, file role badge, and confidence badge displayed in card
- [ ] "View Details" button opens the description dialog
- [ ] Dialog shows all structured fields: summary, description, key symbols, imports, consumers, architectural notes
- [ ] Dialog sections hidden when data is null/empty
- [ ] Generation metadata shown in collapsible footer section
- [ ] "Regenerate" button available in both card and dialog, triggers new `describe-file` job
- [ ] Confidence badge color: high (green), medium (amber), low (red)
- [ ] Generation timeout (5 min) handled gracefully with retry option
- [ ] `FileDescription` Zod schema matches backend response
- [ ] No regressions on other sidebar cards
