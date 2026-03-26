# 12 — Commit Browser & Diff Viewer

## Status
Done

## Goal
Built a commit history browser at `/projects/:id/commits` with a searchable, filterable commit list and a drill-down commit detail page at `/projects/:id/commits/:hash` showing full commit metadata, file change summary, and per-file unified diffs with lazy patch loading. Includes debounced text search, collapsible date range filters, and pagination with filter-aware reset.

## Depends On
01-app-shell, 03-projects

## Scope

### Commit List Page (`/projects/:id/commits`)
Table view of commit history ordered newest-first.

**Table columns**:

| Column | Content |
|---|---|
| Hash | `short_hash` in monospace muted text |
| Message | `message_subject`, bold, truncated to ~72 chars |
| Author | `author_name` |
| Date | Relative time via `formatRelativeTime`; tooltip shows full ISO date |

Rows are clickable `<Link>` elements navigating to `/projects/:id/commits/:hash`.

### Search & Date Range Filter Bar
Above the commit table, matching the Symbol Browser filter pattern:
- **Search input** with `Search` icon prefix, placeholder "Search commits...", debounced at 300 ms via `useDebounce`. Filters commits by message text (case-insensitive via backend).
- **Total count badge** (`Badge variant="secondary"`) showing filtered commit count (e.g. "123 commits")
- **Collapsible "Filters" section** with chevron toggle:
  - Two native `<input type="date">` elements with `Calendar` icon prefix and "From" / "To" labels
  - Date values converted to ISO 8601 (start of day / end of day) before sending to API
  - Dot indicator on collapsed trigger when any date filter is active

### Filter Behavior
- Search debounced at 300 ms; date changes trigger immediately
- All filter changes reset pagination to page 0 and clear loaded pages
- `keepPreviousData: true` prevents layout flash during filter transitions
- Filters combine: search + from date + to date applied together

### Pagination
Hybrid pattern matching the jobs table:
- Page size: 20 items
- Up to 6 pages (120 items): "Load more" button accumulates rows
- Beyond 6 pages: page-number navigation with ellipsis
- `getVisiblePages()` helper computes visible page numbers

### Empty States
- **No commits at all**: `GitCommitHorizontal` icon, "No commits indexed yet", "Commit history will appear after the next index run."
- **Filtered to zero**: `Search` icon, "No commits match your filters", "Try adjusting your search or date range."

### Commit Detail Page (`/projects/:id/commits/:hash`)
Route: `/projects/:id/commits/[hash]`

**Header section**:
- Back button linking to commit list
- Full commit message (first line bold, rest in muted block)
- Author name, committer date (absolute + relative)
- Full commit hash in monospace with copy button (clipboard + toast feedback)
- Parent commit hashes as clickable links navigating to their detail pages
- Diff stats summary: "N files changed, +A additions, -D deletions"

**File diff list**:
Each changed file shown as an expandable entry:
- File path (renames show `old_path -> new_path` via unicode arrow)
- Change type badge via `<ChangeTypeBadge>`: Added (green), Modified (blue), Deleted (red), Renamed (yellow), Copied (gray)
- Per-file `+additions / -deletions` stats
- Language badge when detectable

**Expandable diff view**:
- Initial `getCommitDiffs` call omits `includePatch` (metadata only)
- Expanding a file triggers a follow-up call with `includePatch: true` for that file
- Unified diff rendered as monospace text with line-level coloring:
  - Additions: green background
  - Deletions: red background
  - Hunk headers (`@@`): muted blue
  - Context lines: default
- Binary or omitted patches: muted italic "No diff available"
- Max height with scroll for large diffs (`max-h-[600px] overflow-y-auto`)
- Collapse via toggle button or header click

### tRPC Procedures
In `src/server/api/routers/project-commits.ts`:

| Procedure | Input | Backend |
|---|---|---|
| `projectCommits.listCommits` | `{ projectId, search?, fromDate?, toDate?, limit?, offset? }` | `GET /v1/projects/{id}/commits` |
| `projectCommits.getCommit` | `{ projectId, commitHash }` | `GET /v1/projects/{id}/commits/{hash}` |
| `projectCommits.getCommitDiffs` | `{ projectId, commitHash, limit?, offset?, includePatch? }` | `GET /v1/projects/{id}/commits/{hash}/diffs` |

Exported types: `CommitSummary`, `CommitParent`, `CommitDiffStats`, `CommitDetail`, `FileDiff`, `CommitDiffsResponse`.

### Hooks
- `useDebounce` from `~/hooks/use-debounce` (search input)
- `keepPreviousData` from `@tanstack/react-query`
- `formatRelativeTime` from `~/lib/format`

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/project/[id]/commits/page.tsx` | Server page rendering CommitsContent |
| `src/app/(app)/project/[id]/commits/[hash]/page.tsx` | Server page rendering CommitDetailContent |
| `src/components/project-detail/commits-content.tsx` | Commit list: table, search input, date range filters, pagination, empty/loading/error states |
| `src/components/project-detail/commit-detail-content.tsx` | Commit detail: header with copy hash, parent links, diff stats, expandable file diffs with lazy patch loading, diff line rendering |
| `src/components/project-detail/change-type-badge.tsx` | Color-coded badge for file change types (added/modified/deleted/renamed/copied) |
| `src/server/api/routers/project-commits.ts` | tRPC router with `listCommits`, `getCommit`, `getCommitDiffs` procedures and exported types |

## Acceptance Criteria
- [x] Commit list renders at `/projects/:id/commits` with Hash, Message, Author, Date columns
- [x] Rows are clickable links navigating to commit detail page
- [x] Search input debounced at 300 ms filters by commit message
- [x] Collapsible date range filters with From/To native date inputs
- [x] Active date filter dot indicator on collapsed "Filters" trigger
- [x] Total count badge updates to reflect filtered count
- [x] All filter changes reset pagination to page 0
- [x] `keepPreviousData` prevents layout flash during filter transitions
- [x] Hybrid pagination: load-more for up to 6 pages, page numbers beyond
- [x] Empty state distinguishes "no commits indexed" from "no commits match filters"
- [x] Commit detail shows full hash with copy button, parent links, diff stats summary
- [x] File diff list shows change type badge, path, and +/- stats per file
- [x] Expanding a file lazily loads the patch via `includePatch: true`
- [x] Unified diff renders with line-level coloring (green/red/blue)
- [x] Binary or omitted patches show fallback message
- [x] Loading and error states render correctly on both list and detail pages
