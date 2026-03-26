# 10 — File Detail V2 Analysis Cards

## Status
Done

## Goal
Added a suite of sidebar cards to the file detail view that surface v2 parser analysis data: file facts, exports, diagnostics, references, JSX component usages, network/API calls, and file edit history. Each card is self-contained, fetches its own data, and hides itself when no data exists.

## Depends On
09-file-browser, 00-overview

## Scope

### Sidebar Layout (Right Column of File Viewer)
Cards render in the file viewer right sidebar in this order:
1. **File Info** (existing, unchanged)
2. **File Facts** (from `fileContext` response `file_facts`)
3. **Exports** (from `projectFiles.fileExports`)
4. **Diagnostics** (from `fileContext` response `issues`)
5. **File History** (from `projectFiles.fileHistory`)
6. **Dependencies** (from `projectFiles.fileDependencies` -- covered in ticket 11)
7. **References** (from `projectFiles.fileReferences`)
8. **JSX Components** (from `projectFiles.fileJsxUsages`)
9. **Network Calls** (from `projectFiles.fileNetworkCalls`)

All new cards hidden entirely when their data is empty or null.

### Card: File Facts
- Header: "File Facts" with `Tags` icon and count badge of true facts
- Body: horizontal flex-wrap of small colored badges for each true boolean property:
  - `has_jsx` -> "JSX" (cyan), `has_default_export` -> "Default Export" (green), `has_named_exports` -> "Named Exports" (green outline), `has_top_level_side_effects` -> "Side Effects" (amber), `has_react_hook_calls` -> "React Hooks" (cyan outline), `has_fetch_calls` -> "Fetch Calls" (blue), `has_class_declarations` -> "Classes" (purple), `has_tests` -> "Tests" (emerald), `has_config_patterns` -> "Config" (gray)
- `jsx_runtime` shown as "React" / "Preact" badge when non-empty
- Hidden when no facts are true

### Card: Exports
- Header: "Exports" with `Upload` icon and count badge
- Body: compact list with each entry showing:
  - Export kind badge: `NAMED` (blue), `DEFAULT` (green), `REEXPORT` (amber), `EXPORT_ALL` (purple), `TYPE_ONLY` (gray)
  - Exported name in monospace
  - Re-exports show `from {source_module}` in muted text
  - Line number as clickable link scrolling code viewer to that line
- First 10 items shown; expandable "Show all N exports" button for overflow

### Card: Diagnostics
- Header: "Diagnostics" with `AlertTriangle` icon and severity-colored count badge (red if any errors, amber if warnings only, gray if info only)
- Body: list sorted by severity (errors first, then warnings, then info):
  - Severity icon: `XCircle` (red/error), `AlertTriangle` (amber/warning), `Info` (blue/info)
  - Issue code in monospace bold (e.g. `LONG_FUNCTION`)
  - Message text
  - Line number link
- Hidden when issues array is empty

### Card: File History
- Header: "History" with `GitCommitHorizontal` icon and count badge
- Body: compact commit list, each entry showing:
  - Short hash in monospace
  - Author name
  - Relative date with tooltip for absolute
  - Change type badge (added/modified/deleted/renamed/copied)
  - `+additions / -deletions` stats
  - Row links to commit detail page `/projects/:id/commits/:hash`
- Initially shows 3 commits with "Load more" button to show all
- Line navigation buttons with aria-labels for accessibility

### Card: References
- Header: "References" with `Link` icon and count badge
- Body: grouped by `reference_kind` into collapsible sections:
  - **CALL** -> "Function Calls": target_name with line
  - **TYPE_REF** -> "Type References": target_name with line
  - **JSX_RENDER** -> "JSX Renders": target_name with line
  - **EXTENDS / IMPLEMENTS** -> "Inheritance": target_name with line
  - Other kinds grouped as "Other"
- Each group shows first 5 items with "Show all" expansion
- Hidden when no references exist

### Card: JSX Components
- Header: "JSX Components" with `Blocks` icon and count badge
- Body in two sections:
  - **Custom Components**: non-intrinsic, non-fragment usages listed with component_name and line number
  - **Intrinsic Elements**: summarized as count (e.g. "12 intrinsic elements (div, span, button, ...)")
- Only shown for JSX/TSX files with usages

### Card: Network Calls
- Header: "API Calls" with `Globe` icon and count badge
- Body: each entry shows:
  - Method badge: `GET` (green), `POST` (blue), `PUT` (amber), `DELETE` (red), `PATCH` (purple), `UNKNOWN` (gray)
  - Client kind in muted text: `fetch`, `axios`, `ky`, `graphql`
  - URL (`url_literal` or `url_template`) in monospace, truncated with tooltip for full text
  - `is_relative` shown as small "relative" badge
  - Line number link
- Hidden when no network calls exist

### Data Fetching
All per-file endpoints fetched in parallel using `useQuery` with `enabled: !!filePath`. File facts and issues come from the existing `fileContext` response (no additional request). The four new endpoints each have their own query:
- `projectFiles.fileExports`
- `projectFiles.fileReferences`
- `projectFiles.fileJsxUsages`
- `projectFiles.fileNetworkCalls`

### tRPC Procedures
Added to `src/server/api/routers/project-files.ts`:

| Procedure | Input | Backend |
|---|---|---|
| `projectFiles.fileExports` | `{ projectId, filePath }` | `GET /v1/projects/{id}/files/exports?file_path=...` |
| `projectFiles.fileReferences` | `{ projectId, filePath }` | `GET /v1/projects/{id}/files/references?file_path=...` |
| `projectFiles.fileJsxUsages` | `{ projectId, filePath }` | `GET /v1/projects/{id}/files/jsx-usages?file_path=...` |
| `projectFiles.fileNetworkCalls` | `{ projectId, filePath }` | `GET /v1/projects/{id}/files/network-calls?file_path=...` |
| `projectFiles.fileHistory` | `{ projectId, filePath, limit?, offset? }` | `GET /v1/projects/{id}/files/history?file_path=...` |

## Key Files

| File | Purpose |
|---|---|
| `src/components/project-detail/file-viewer-content.tsx` | Orchestrates all card queries; renders sidebar card stack |
| `src/components/project-detail/file-facts-card.tsx` | File facts boolean badge grid |
| `src/components/project-detail/file-exports-card.tsx` | Exports list with kind badges and line links |
| `src/components/project-detail/file-diagnostics-card.tsx` | Severity-sorted diagnostics with icons and codes |
| `src/components/project-detail/file-history-card.tsx` | Commit history with change stats and load-more |
| `src/components/project-detail/file-references-card.tsx` | Grouped references with collapsible sections |
| `src/components/project-detail/file-jsx-usages-card.tsx` | Custom vs intrinsic JSX component usages |
| `src/components/project-detail/file-network-calls-card.tsx` | Method-badged API call list with URLs |
| `src/server/api/routers/project-files.ts` | tRPC procedures and exported response types (`FileFacts`, `FileIssue`, `FileExport`, `SymbolReference`, `JsxUsage`, `NetworkCall`, `FileHistoryEntry`) |

## Acceptance Criteria
- [x] File Facts card renders boolean badges for each true fact; hidden when no facts are true
- [x] Exports card shows export kind badge, name, re-export source, line links; expandable beyond 10 items
- [x] Diagnostics card shows severity-sorted issues with icons, codes, messages, and line links
- [x] Diagnostics header badge color reflects highest severity present
- [x] File History card shows commit list with change stats; initially 3 items with load-more
- [x] History card line navigation buttons have aria-labels for accessibility
- [x] References card groups by kind (function calls, type refs, JSX renders, inheritance, other) with collapsible sections
- [x] JSX Components card separates custom components from intrinsic elements; only shown for JSX/TSX files
- [x] Network Calls card shows method badge, client kind, URL (truncated with tooltip), relative indicator
- [x] All cards hidden when their data is empty or null (no empty card shells)
- [x] All cards use skeleton loading states matching existing card patterns
- [x] All four new tRPC procedures handle errors consistently
- [x] No regressions on existing File Info and Dependencies cards
