# 09 — File Browser & Viewer

## Status
Done

## Goal
Built the file browser and viewer at `/projects/:id/file` providing a file tree panel, breadcrumb navigation, and a syntax-highlighted code viewer with Shiki. The URL reflects the selected file path so views are deep-linkable, and the right sidebar hosts file metadata and analysis cards (covered in tickets 10 and 11).

## Depends On
01-app-shell, 00-overview

## Scope

### Route
`/projects/:id/file` with query parameter `?path=<file_path>&line=<n>`. Server page at `src/app/(app)/project/[id]/file/page.tsx` renders the `<FileViewerContent>` client component.

### File Tree Panel (Left)
- `FileBrowser` component fetches project structure via `api.projects.structure.useQuery({ id: projectId })`
- Tree data is of type `FileNode` (recursive: `{ name, path, type: "file" | "directory", children? }`)
- Tree rendered by `FileTreeView` which delegates to `FileTreeDirectory` and `FileTreeFile`
- Directories are expandable/collapsible with chevron icons
- Files show language-appropriate icons via `FileIcon` component
- Clicking a file calls `onFileSelect(path)` which updates URL query param
- Tree auto-clears selection when the selected path no longer exists in the structure
- Loading state: `FileTreeSkeleton` with animated placeholder rows
- Empty state: `FileTreeEmpty` when no files are indexed

### Breadcrumb Navigation
- `FilePathBreadcrumb` component renders each path segment as a clickable link
- Navigates to the corresponding directory or file on click
- Displayed above the code viewer

### Code Viewer
- `CodeViewerWithPreview` component wraps `CodeBlock` with optional preview tab for HTML/Markdown/Mermaid files
- `CodeBlock` uses Shiki (`codeToHtml`) for syntax highlighting with `github-light` / `github-dark` themes (auto-switches via `next-themes`)
- Line numbers rendered via CSS counters (`counter-reset: line` / `counter-increment: line`)
- Each line has an ID attribute (e.g., `line-42`) for scroll-to-line support via URL `?line=N`
- Large files render with vertical scroll (`overflow-y-auto`)
- Preview tab available for HTML, Markdown, and Mermaid file types:
  - `HtmlPreview`: sandboxed iframe rendering
  - `MarkdownPreview`: react-markdown with remark-gfm and rehype-raw
  - `MermaidPreview`: mermaid.js diagram rendering

### File Metadata Header
Displayed above the code viewer, showing:
- Language badge
- Line count
- File size (formatted via `formatBytes`)
- Content hash (monospace, truncated with copy button)
- Copy path button

### File Context Data
Fetched via `projectSearch.fileContext` tRPC procedure:
- Returns `file_path`, `language`, `size_bytes`, `line_count`, `content_hash`, `content`, `snapshot_id`, `last_indexed_at`
- Extended with v2 data: `file_facts`, `issues`, `parser_meta`, `extractor_statuses`

### Right Sidebar
The file viewer page renders a two-column layout: code viewer (left, flex-grow) and sidebar cards (right, fixed width). Sidebar cards include File Info, File Facts, Exports, Diagnostics, History, Dependencies, References, JSX Usages, and Network Calls (detailed in tickets 10 and 11).

### States
- **Loading**: Skeleton in tree panel; skeleton blocks in code viewer area
- **No file selected**: Prompt message to select a file from the tree
- **File not found**: Alert with error message
- **Error**: Alert with retry button

### tRPC Procedures

| Procedure | Input | Backend |
|---|---|---|
| `projects.structure` | `{ id }` | `GET /v1/projects/{id}/structure` |
| `projectSearch.fileContext` | `{ projectId, filePath }` | `GET /v1/projects/{id}/files/context?file_path=...` |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/project/[id]/file/page.tsx` | Server page rendering FileViewerContent |
| `src/components/project-detail/file-viewer-content.tsx` | Main layout: tree panel + code viewer + sidebar cards, file context query, breadcrumb |
| `src/components/project-detail/file-browser.tsx` | File tree panel with structure query, selection management |
| `src/components/project-detail/file-tree-view.tsx` | Recursive tree renderer |
| `src/components/project-detail/file-tree-directory.tsx` | Expandable directory node |
| `src/components/project-detail/file-tree-file.tsx` | Clickable file leaf node |
| `src/components/project-detail/file-tree-types.ts` | `FileNode` type definition |
| `src/components/project-detail/file-tree-skeleton.tsx` | Loading skeleton for tree |
| `src/components/project-detail/file-tree-empty.tsx` | Empty state for tree |
| `src/components/project-detail/file-icon.tsx` | Language-based file icon |
| `src/components/project-detail/code-viewer-with-preview.tsx` | Tabs wrapper: Code tab (Shiki) + Preview tab (HTML/Markdown/Mermaid) |
| `src/components/project-detail/code-block.tsx` | Shiki syntax highlighter with line numbers and theme support |
| `src/components/project-detail/html-preview.tsx` | Sandboxed HTML preview |
| `src/components/project-detail/markdown-preview.tsx` | react-markdown preview |
| `src/components/project-detail/mermaid-preview.tsx` | Mermaid diagram preview |
| `src/server/api/routers/project-search.ts` | `fileContext` tRPC procedure |
| `src/server/api/routers/projects.ts` | `structure` tRPC procedure |

## Acceptance Criteria
- [x] File tree renders from project structure endpoint with expand/collapse directories
- [x] Clicking a file in the tree updates URL query param and loads file content
- [x] Breadcrumb navigation renders path segments as clickable links
- [x] Code viewer uses Shiki with `github-light`/`github-dark` themes
- [x] Line numbers rendered with CSS counters and line IDs for scroll-to-line
- [x] URL `?line=N` scrolls to the specified line on load
- [x] File metadata header shows language, line count, size, content hash
- [x] Preview tab available for HTML, Markdown, and Mermaid files
- [x] Tree auto-clears selection when file no longer exists in structure
- [x] Loading, empty, and error states render correctly for both tree and viewer
- [x] Right sidebar renders file analysis cards (integration point for tickets 10, 11)
- [x] URL reflects selected file path for deep-linkable views
