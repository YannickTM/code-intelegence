# 11 — Dependency Graph Visualization

## Status
Done

## Goal
Built an interactive dependency graph visualization using `@xyflow/react` with dagre auto-layout, accessible from the file detail sidebar. The dependencies card shows bidirectional import relationships (what a file imports and what imports it), and a dialog-based graph explorer lets users visually traverse the dependency neighbourhood at configurable depth.

## Depends On
09-file-browser, 00-overview

## Scope

### Dependencies Card (File Viewer Sidebar)
Replaces the original "Coming soon" dependencies stub. Renders as a card in the file detail right sidebar.

**Header**: "Dependencies" with `Network` icon and count badge (total imports + imported-by).

**Body -- two collapsible sections**:
- **Imports (N)**: forward dependencies. Each entry is:
  - Internal files: file path as link navigating to `/projects/:id/file?path=<target_file_path>`
  - External packages: `package_name` with `Badge variant="outline"` labelled "external"
  - Import name in muted monospace below the path
- **Imported By (N)**: reverse dependencies. Each entry: source file path as link with import name below.
- Both sections default to 5 visible entries with "Show more" to expand

**Graph button**: Full-width outline button at bottom: "View dependency graph" opens the graph dialog.

**States**:
- Empty: muted italic "No dependencies found for this file."
- Loading: 3 skeleton rows of varying width
- Error: Alert with retry button

### Dependency Graph Dialog
Opens from the "View dependency graph" button. Uses shadcn `Dialog` sized at `max-w-5xl`.

**Header**:
- Title: "Dependency Graph"
- Subtitle: file path in muted monospace
- Depth selector: `Select` with options 1 through 5 (default 2)
- Truncation warning: muted text "Graph capped at 200 nodes" when `truncated === true`

**Graph area** (fills dialog body, `min-h-[60vh]`):
- Uses `ReactFlow` from `@xyflow/react` with `@dagrejs/dagre` for hierarchical top-down layout
- Interactive: pan, zoom, drag nodes
- Built-in `Controls` component for zoom in/out, fit view
- `Background` component for dot-grid pattern

**Node rendering** (custom `DependencyGraphNode` component, registered as `nodeTypes.dependency`):
- Internal files: rounded rectangle, file name as label, full path in tooltip, language badge
- External packages: dashed border, muted background, package name as label
- Root node: highlighted with primary border color
- Depth indicated by opacity (deeper nodes more muted)

**Edge rendering**:
- Directed edges from source to target
- Default edge styling with animated markers

**Node interaction**: Clicking a node navigates to that file's detail view (closes dialog, triggers navigation via `useRouter`).

**States**:
- Loading: Skeleton placeholder in dialog body
- Error: Alert with retry button inside dialog body

### Layout Algorithm
`buildLayout` function uses `@dagrejs/dagre` to compute node positions:
- Graph direction: top-to-bottom (`rankdir: "TB"`)
- Fixed node dimensions: 180px wide, 40px tall
- Graph data transformed from `DependencyGraphResponse` into `@xyflow/react` Node and Edge arrays

### Data Fetching
Graph query only fires when dialog is open (`enabled: graphOpen`). Depth change triggers a new query.

### tRPC Procedures
Added to `src/server/api/routers/project-files.ts`:

| Procedure | Input | Backend |
|---|---|---|
| `projectFiles.fileDependencies` | `{ projectId, filePath }` | `GET /v1/projects/{id}/files/dependencies?file_path=...` |
| `projectFiles.dependencyGraph` | `{ projectId, root, depth? }` | `GET /v1/projects/{id}/dependencies/graph?root=...&depth=...` |

### Response Types
- `DependencyEdge`: `{ id, source_file_path, target_file_path, import_name, import_type, package_name?, package_version? }`
- `FileDependenciesResponse`: `{ file_path, imports[], imported_by[], snapshot_id }`
- `GraphNode`: `{ file_path, language?, is_external, depth }`
- `GraphEdge`: `{ source, target, import_name, import_type, package_name? }`
- `DependencyGraphResponse`: `{ nodes[], edges[], root, depth, truncated, snapshot_id }`

## Key Files

| File | Purpose |
|---|---|
| `src/components/project-detail/file-dependencies-card.tsx` | Sidebar card: imports/imported-by lists, "Show more" expansion, graph button |
| `src/components/project-detail/dependency-graph-dialog.tsx` | Dialog with ReactFlow graph, dagre layout, depth selector, truncation warning |
| `src/components/project-detail/dependency-graph-node.tsx` | Custom @xyflow node: internal file vs external package styling, root highlight |
| `src/server/api/routers/project-files.ts` | `fileDependencies` and `dependencyGraph` procedures with exported types |
| `package.json` | `@xyflow/react` and `@dagrejs/dagre` dependencies |

## Acceptance Criteria
- [x] Dependencies card replaces the stub in file detail sidebar
- [x] Card shows "Imports (N)" section with internal file links and external package badges
- [x] Card shows "Imported By (N)" section with source file links
- [x] Clicking an internal file link navigates to that file's detail view
- [x] Lists default to 5 entries with "Show more" to expand
- [x] Empty state shows muted italic message
- [x] Loading and error states render correctly
- [x] "View dependency graph" button opens dialog
- [x] Graph data is only fetched when dialog opens (lazy loading)
- [x] Nodes render with custom component distinguishing internal files from external packages
- [x] Root node is visually highlighted with primary border color
- [x] Depth selector (1-5) re-fetches graph with new depth
- [x] Truncation warning shown when graph is capped at 200 nodes
- [x] Clicking a graph node navigates to that file's detail view
- [x] Graph supports pan, zoom, and fit-to-view controls
- [x] Dagre layout positions nodes in hierarchical top-down arrangement
