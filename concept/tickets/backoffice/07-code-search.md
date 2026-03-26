# 07 — Full-Text Code Search

## Status
Done

## Goal
Built a full-text code search page at `/projects/:id/search` that lets developers search indexed code with case-sensitive/regex toggles, a language filter dropdown (17 languages), collapsible advanced filters (file pattern, include/exclude directory), and paginated results rendered as collapsible cards with Shiki syntax highlighting and yellow match decorations.

## Depends On
01-app-shell, 03-projects

## Scope

### Route
`/projects/:id/search` — replaces the original placeholder page. Server page at `src/app/(app)/project/[id]/search/page.tsx` renders the `<CodeSearch>` client component.

### Search & Filter Bar
- **Query input** with `Search` icon prefix. Fires on Enter key or submit button (not debounced, to avoid partial regex/multiword queries hitting the API).
- **Search mode toggle buttons** inline with search input:
  - `Aa` (Match Case) — toggles `searchMode = "sensitive"`
  - `.*` (Use Regular Expression) — toggles `searchMode = "regex"`
  - Active state: `bg-accent text-accent-foreground`; inactive: `text-muted-foreground`
  - Both wrapped in `Tooltip` for accessibility
- **Language filter** `Select` dropdown with "All Languages" default and 17 options: Go, TypeScript, JavaScript, Python, Rust, Java, C, C++, Ruby, PHP, CSS, HTML, SQL, Shell, YAML, JSON, Markdown. Values match backend file extension convention (`"go"`, `"typescript"`, `"javascript"`, `"py"`, `"rs"`, etc.).
- **Total count badge** (`Badge variant="secondary"`) showing filtered result count
- **Collapsible filters section** (triggered by "Filters" label with chevron toggle):
  - File pattern input (`FileCode` icon, placeholder: `File pattern (e.g. *.go, **/*.test.ts)`)
  - Include directories input (`FolderOpen` icon, glob patterns)
  - Exclude directories input (`FolderMinus` icon, glob patterns)
  - All three debounced at 300 ms via `useDebounce`
  - Dot indicator on collapsed trigger when any filter is active

### Inline Regex Error
When the backend returns 422 for an invalid regex, a `text-destructive text-xs` message appears inline below the search input. Previous result data is retained via `keepPreviousData`.

### Results List
Results display as collapsible cards (not a table). Each card has:
- **Header row**: file icon (`FileCode`), file path as link to file viewer (`/projects/:id/file?path=...&line=<start_line>`), language badge, line range, match count, expand/collapse chevron, "View file" link (`ExternalLink` icon)
- **Code block** (expanded): Shiki syntax highlighting (`github-light`/`github-dark` themes) with line numbers. Match occurrences highlighted using Shiki decorations with a `search-match` CSS class rendering `bg-yellow-200/50 dark:bg-yellow-700/30`.
- Cards are collapsed by default; clicking the header or chevron expands to show the highlighted code snippet

### Match Highlighting
Client-side decoration computation using `computeMatchDecorations()`:
- For insensitive mode: case-insensitive string matching
- For sensitive mode: case-sensitive string matching
- For regex mode: `RegExp.exec` loop with zero-length match protection
- Decorations passed to Shiki `codeToHtml` as `decorations` option

### Pagination
Bottom of results list: `<Pagination>` component with "Showing X-Y of Z results" and Previous/Next buttons. Page size: 20. Resets to page 0 on any filter or search mode change.

### States
- **Loading**: 4 skeleton cards with shimmer animation for header + code block area
- **Empty (no index)**: `Code` icon, "No index available. Trigger an indexing job to search code."
- **Empty (no query)**: `Search` icon, "Enter a search query to find code across the project."
- **Empty (no results)**: `Search` icon, "No code found matching your search."
- **Error (non-regex)**: Alert with retry button
- **Error (regex 422)**: Inline below search input, previous results retained

### tRPC Procedures
In `src/server/api/routers/project-search.ts`:

| Procedure | Input | Backend |
|---|---|---|
| `projectSearch.search` | `{ projectId, query, searchMode?, language?, filePattern?, includeDir?, excludeDir?, limit?, offset? }` | `POST /v1/projects/{id}/query/search` |

The procedure is `.query` (idempotent from React Query perspective) calling the backend via `POST` method. Input maps camelCase to snake_case for the backend request body.

### Hooks
- `useDebounce` from `~/hooks/use-debounce` (file pattern, include dir, exclude dir)
- `keepPreviousData` from `@tanstack/react-query` for filter transitions

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/project/[id]/search/page.tsx` | Server page rendering CodeSearch |
| `src/components/project-detail/code-search.tsx` | Main search component: filter bar, search mode toggles, language dropdown, collapsible filters, skeleton cards, state management, results list, pagination |
| `src/components/project-detail/code-search-result.tsx` | Individual result card: collapsible header, Shiki-highlighted code with line numbers, match decorations via `computeMatchDecorations()` |
| `src/server/api/routers/project-search.ts` | `search` tRPC procedure with `CodeSearchMatch` and `CodeSearchResponse` types |

## Acceptance Criteria
- [x] Code search renders at `/projects/:id/search` replacing the placeholder
- [x] Search fires on Enter key (not debounced on the main query input)
- [x] Case-sensitive (`Aa`) and regex (`.*`) toggle buttons set `searchMode` correctly
- [x] Language dropdown filters by 17 language options
- [x] Collapsible filters section with file pattern, include dir, exclude dir inputs
- [x] Active filter dot indicator on collapsed "Filters" trigger
- [x] All filter changes debounced at 300 ms and reset pagination to page 0
- [x] Results render as collapsible cards with file path, language badge, line range, match count
- [x] Expanding a card shows Shiki syntax-highlighted code with yellow match decorations
- [x] File path links navigate to file viewer at the match start line
- [x] Invalid regex (422) shows inline error below search input without replacing result data
- [x] `keepPreviousData` prevents layout flash on filter/page changes
- [x] Pagination with Previous/Next and total count (page size 20)
- [x] Empty, loading, and error states render correctly
- [x] tRPC `search` procedure maps camelCase input to snake_case backend request
