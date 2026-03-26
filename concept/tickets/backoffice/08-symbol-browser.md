# 08 — Symbol Browser (V2)

## Status
Done

## Goal
Built a full-featured symbol browser at `/projects/:id/symbols` that lets developers search, filter, and inspect all symbols extracted during indexing. Combines name search with case-sensitive/regex toggles, kind and directory filters, a results table with v2 metadata badges, and an expandable detail panel per symbol.

## Depends On
01-app-shell, 00-overview

## Scope

### Route
`/projects/:id/symbols` — replaces the original placeholder page. Server page at `src/app/(app)/project/[id]/symbols/page.tsx` renders the `<SymbolList>` client component.

### Search & Filter Bar
- **Name search input** with `Search` icon prefix, debounced at 300 ms via `useDebounce`
- **Search mode toggle buttons** inline with search input:
  - `Aa` (Match Case) — toggles `searchMode = "sensitive"`
  - `.*` (Use Regular Expression) — toggles `searchMode = "regex"`
  - Active state: `bg-accent text-accent-foreground`; inactive: `text-muted-foreground`
  - Both wrapped in `Tooltip` for accessibility
- **Kind filter** `Select` dropdown with options: All Kinds, Function, Class, Interface, Type Alias, Variable, Enum, Method, Namespace
- **Total count badge** (`Badge variant="secondary"`) showing filtered total
- **Collapsible directory filters** (triggered by "Filters" label with chevron toggle):
  - Include directories input (`FolderOpen` icon, glob patterns, comma-separated)
  - Exclude directories input (`FolderMinus` icon, glob patterns, comma-separated)
  - Dot indicator on collapsed trigger when either filter is active
  - Both debounced at 300 ms

### Inline Regex Error
When the backend returns 422 for an invalid regex, a `text-destructive text-xs` message appears inline below the search input. Previous table data is retained via `keepPreviousData`.

### Symbol Results Table
Columns: Name, Kind, Type, File, Lines.

| Column | Content |
|---|---|
| **Name** | Symbol name in monospace. Qualified name as muted subtitle when it differs. V2 badges after name: `Export` (green), `Default` (green filled), `async` (blue), `static` (gray), `abstract` (purple), `Component` (cyan), `Hook` (cyan outline). Max 3 visible badges; overflow shows "+N" tooltip. |
| **Kind** | Color-coded badge per kind (function=blue, class=purple, interface=green, type_alias=orange, variable=gray, enum=yellow, method=blue-outline, namespace=teal) |
| **Type** | `return_type` in muted monospace (e.g. `-> Promise<User[]>`). Truncated with tooltip for full text. Empty cell if absent. |
| **File** | `file_path` as link navigating to `/projects/:id/file?path=...&line=<start_line>` |
| **Lines** | `start_line-end_line` range in muted text |

Row click expands the inline detail panel.

### Symbol Detail Panel
Accordion-style expansion below the clicked row. Content:
- **Type signature**: return type and parameter types in monospace (when present)
- **Signature**: full function/method signature in syntax-highlighted monospace block
- **Qualified name**: full qualified path (e.g. `ClassName.methodName`)
- **Modifiers**: inline badges for `modifiers[]` (export, async, static, abstract, readonly, etc.)
- **Flags summary**: collapsible "Symbol properties" section with two-column grid of boolean flags (check/x icons)
- **Documentation**: `doc_text` rendered as preformatted text
- **View source**: button linking to file viewer at the symbol's location

### Pagination
Bottom of table: "Showing X-Y of Z symbols". Previous/Next navigation. Page size: 50. Resets to page 0 on any filter change.

### States
- **Loading**: 8 skeleton rows with shimmer animation
- **Empty (no index)**: "No index available. Trigger an indexing job to extract symbols."
- **Empty (filtered)**: "No symbols found matching your search."
- **Error**: Alert with retry button

### tRPC Procedures
Extended in `src/server/api/routers/project-search.ts`:

| Procedure | Input | Backend |
|---|---|---|
| `projectSearch.listSymbols` | `{ projectId, name?, kind?, searchMode?, includeDir?, excludeDir?, limit?, offset? }` | `GET /v1/projects/{id}/symbols` |
| `projectSearch.getSymbol` | `{ projectId, symbolId }` | `GET /v1/projects/{id}/symbols/{symbol_id}` |

The `Symbol` type includes v2 fields: `flags` (boolean properties), `modifiers` (string array), `return_type`, `parameter_types`.

### Hooks
- `useDebounce` from `~/hooks/use-debounce` (name, include dir, exclude dir)
- `keepPreviousData` from `@tanstack/react-query` for pagination and filter transitions

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/project/[id]/symbols/page.tsx` | Server page rendering SymbolList |
| `src/components/project-detail/symbol-list.tsx` | Main table with search/filter bar, kind dropdown, search mode toggles, collapsible directory filters, pagination, row expansion |
| `src/components/project-detail/symbol-detail-panel.tsx` | Expanded detail: signature, doc_text, type info, modifiers, flags grid, source link |
| `src/server/api/routers/project-search.ts` | `listSymbols` and `getSymbol` tRPC procedures with v2 Symbol type |

## Acceptance Criteria
- [x] Symbol list renders at `/projects/:id/symbols` replacing the placeholder
- [x] Table shows Name, Kind, Type, File, Lines columns
- [x] Name search input filters symbols with debounced queries
- [x] Case-sensitive (`Aa`) and regex (`.*`) toggle buttons work with correct `searchMode` values
- [x] Kind dropdown filters by symbol kind
- [x] Collapsible directory include/exclude filters with glob support
- [x] Active directory filters show dot indicator on collapsed trigger
- [x] All filters combine and reset pagination to page 0
- [x] Invalid regex (422) shows inline error below search input without replacing table data
- [x] V2 badges (exported, async, static, abstract, component, hook) render on symbol names
- [x] Badge overflow limited to 3 visible with "+N" tooltip
- [x] Return type column shows `return_type` in muted monospace
- [x] Row click expands inline detail panel with signature, doc_text, type info, modifiers, flags
- [x] Detail panel shows "View source" button linking to file viewer
- [x] Pagination with Previous/Next and total count
- [x] `keepPreviousData` prevents layout flash on filter/page changes
- [x] Empty, loading, and error states render correctly
- [x] File path links navigate to file viewer at the symbol's line
