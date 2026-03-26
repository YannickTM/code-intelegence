# Changelog

All notable changes to the MYJUNGLE Backoffice will be documented in this file.

## [Unreleased]

### Added

- **File Detail View: V2 Analysis Cards (030)** — Added 6 new sidebar cards to the File Detail View displaying v2 parser analysis data. Replaced the "AI Description" placeholder with real analysis information.
  - **tRPC procedures** — 4 new procedures in `projectFiles` router: `fileExports`, `fileReferences`, `fileJsxUsages`, `fileNetworkCalls`. Extended `FileContextResponse` with `file_facts`, `issues`, `parser_meta`, `extractor_statuses` fields.
  - **File Facts card** — Horizontal badge grid showing boolean file properties (JSX, Default Export, Named Exports, Side Effects, React Hooks, Fetch Calls, Classes, Tests, Config) with semantic colors. Hidden when no facts are true.
  - **Exports card** — Compact list of file exports with kind badges (Named/Default/Reexport/ExportAll/TypeOnly), monospace names, re-export source modules, and line number links. Expandable beyond 10 items.
  - **Diagnostics card** — Severity-sorted issue list with colored icons (error/warning/info), issue codes, messages, and line links. Severity-colored count badge in header.
  - **References card** — References grouped by kind (Function Calls, Type References, JSX Renders, Inheritance, Other) with collapsible sections. 5 items per group, expandable.
  - **JSX Components card** — Separates custom components (as clickable badges) from intrinsic elements (count summary). Only shown for JSX/TSX files.
  - **API Calls card** — Network call list with HTTP method badges, client kind, truncated URLs with tooltips, relative indicator, and line links. Expandable beyond 10 items.
  - **General** — All new cards hidden when data is empty. Loading skeletons match existing card patterns. AI Description placeholder removed. No regressions on existing cards.

- **Symbol Browser V2: Type Info & Flags (029)** — Enhanced the Symbol Browser with v2 parser metadata: export/async/static/component/hook badges, return type column, and enriched detail panel.
  - **tRPC type** — Extended `Symbol` type in `projectSearch` router with `flags`, `modifiers`, `return_type`, `parameter_types` fields matching the backend v2 response.
  - **Flag badges** — Data-driven badge system on symbol names showing export (green outline), async (blue), static (gray), abstract (purple), Component (cyan), Hook (cyan outline). Mutually exclusive default/named export handling. Max 3 visible badges with "+N" overflow tooltip.
  - **Type column** — New "Type" column between Kind and File showing `→ {return_type}` in muted monospace with truncation and full-text tooltip.
  - **Detail panel — Type signature** — New section above Signature showing parameter types as `(type1, type2, ...)` and return type as `→ type` in a monospace block.
  - **Detail panel — Modifiers** — Inline badges for each modifier (export, async, static, etc.) below Qualified Name.
  - **Detail panel — Flags summary** — Collapsible "Symbol properties" section with two-column grid of boolean flags with check/x icons.
  - **Backward compatible** — All new sections gracefully hidden when v2 data is absent (pre-v2 indexed projects).

- **Codebase Search (018)** — VS Code-like full-text code search UI replacing the Search tab placeholder.
  - **tRPC router** — Replaced `search` stub in `projectSearch` with real `.query` procedure calling `POST /v1/projects/{id}/query/search`. Input: `query` (required), `searchMode` (insensitive/sensitive/regex), `language`, `filePattern`, `includeDir`, `excludeDir`, `limit`, `offset`. Replaced old `SearchResult`/`_SearchResponse` types with `CodeSearchMatch`/`CodeSearchResponse`.
  - **Search input** — Text input fires on Enter key (not debounced). `Aa` (Match Case) and `.*` (Use Regular Expression) toggle buttons with tooltips, matching symbol browser pattern.
  - **Language filter** — Dropdown with "All Languages" default plus 17 common languages. Values match backend-stored identifiers (file extensions for most languages, e.g. `py`, `rs`, `md`, `sh`; full names `typescript`/`javascript` for parser-handled languages).
  - **Collapsible filters** — File pattern, include dirs, and exclude dirs inputs with debounced (300ms) re-search. Dot indicator shows when filters are active.
  - **Collapsible result cards** — Results render as collapsed header bars by default (file path, language badge, line range, match count). Click to expand and view the code snippet.
  - **Syntax highlighting** — Expanded code blocks use Shiki (`github-light`/`github-dark` themes) with match decorations overlaid as yellow highlights. Line numbers offset to match actual file positions. Falls back to plain-text rendering with `<mark>` match highlighting when Shiki fails.
  - **Pagination** — "Showing X–Y of Z results" with Previous/Next buttons. PAGE_SIZE = 20.
  - **Empty states** — No index, no query (initial), no results. Loading state with 4 skeleton cards. FilterBar preserved in all states.
  - **Inline regex errors** — 422 errors displayed inline below search input, previous data retained via `keepPreviousData`.

- **Commit Search & Date Range Filters (028)** — Search input and date range filter bar for the Commit Browser.
  - **tRPC router** — Extended `projectCommits.listCommits` with `search` (max 500), `fromDate`, and `toDate` optional params forwarded to the backend as `search`, `from_date`, `to_date` URL params.
  - **Search input** — Debounced (300ms) text input with magnifying glass icon for case-insensitive commit message search.
  - **Date range filters** — Collapsible "Filters" section with labeled From/To native date inputs for filtering commits by committer date range, with start-of-day/end-of-day ISO conversion. Dot indicator shows when date filters are active.
  - **Total count badge** — Muted badge showing filtered total count, updating as filters change.
  - **Pagination reset** — All filter changes reset pagination to page 0 and clear loaded pages cache.
  - **Filtered empty state** — Distinct "No commits match your filters" message when filters produce zero results, separate from "No commits indexed yet".
  - **`placeholderData: keepPreviousData`** — Prevents layout flash during filter transitions.

- **Symbol Browser Advanced Search (027)** — VS Code-like search controls for the Symbol Browser with search mode toggles and directory filters.
  - **tRPC router** — Extended `projectSearch.listSymbols` with `searchMode` (insensitive/sensitive/regex), `includeDir`, and `excludeDir` params forwarded to the backend.
  - **Search mode toggles** — Inline `Aa` (Match Case) and `.*` (Use Regular Expression) toggle buttons next to the search input with tooltips and active state styling.
  - **Directory filters** — Collapsible "Filters" section with include/exclude directory inputs supporting comma-separated glob patterns (e.g. `src/api, **/test/**`). Dot indicator shows when filters are active.
  - **Inline regex errors** — Invalid regex patterns (422) display inline below the search input instead of replacing the table. Previous data is retained via `placeholderData: keepPreviousData`.
  - **Debounced inputs** — Directory filter inputs debounced at 300ms, matching existing name search behavior. All filter changes reset pagination.

- **Platform Worker Status View (026)** — Read-only `/platform-settings/workers` page for platform admins to view active worker instances.
  - **tRPC router** — New `platformWorkers` router with `list` procedure proxying to `GET /v1/platform-management/workers`.
  - **Workers table** — Displays worker ID (monospace), color-coded status badge, hostname, version, supported workflow badges, current job (truncated UUID), uptime (relative duration), and last heartbeat (relative time).
  - **Status badges** — Color-coded by status: starting (amber), idle (green), busy (blue), draining (orange), stopped (gray). Draining workers show drain reason as tooltip.
  - **Workflow badges** — Each supported workflow rendered as a labeled badge (Full Index, Incremental, Code Analysis, RAG File, RAG Repo, Agent).
  - **Manual refresh** — Refresh button with spinning animation triggers query invalidation. No auto-polling.
  - **Loading/empty/error states** — Skeleton rows, centered empty state with icon, error alert with retry button. Redis-specific error message for 502 responses.
  - **Navigation** — "Workers" entry with `Activity` icon added to platform settings sub-navigation.

- **Platform LLM Provider Settings — Multi-Provider (025)** — Cards-based multi-provider management UI at `/platform-settings/llm` for platform admins, mirroring the embedding view.
  - **tRPC router** — New `platformLLM` router with 8 procedures: `list`, `update`, `test`, `create` (POST), `updateById` (PATCH), `deleteById` (DELETE), `promote` (POST), `testById` (POST) proxying to backend multi-provider LLM endpoints.
  - **Provider cards** — Each active LLM provider displayed as a card with name, Default/Active badges, provider, model (or "Not specified"), endpoint URL, availability, credentials status, and last updated.
  - **Add Provider** — Inline form to create additional non-default LLM providers with full validation.
  - **Edit Provider** — Inline edit form per card with pre-populated values, saves via PATCH.
  - **Delete Provider** — Confirmation dialog via overflow menu; disabled for default provider.
  - **Promote to Default** — Overflow menu action to atomically promote any provider to default.
  - **Test Connection** — Per-provider connectivity test with inline result on each card.
  - **Shared component reuse** — Reuses `ProviderCard` component from `platform-settings/components/` with `capability="llm"`.
  - **Loading/empty/error states** — Card skeletons, empty state prompt, error alert with retry.

- **Platform Embedding Provider Settings — Multi-Provider (024)** — Cards-based multi-provider management UI at `/platform-settings/embedding` for platform admins.
  - **tRPC router** — Extended `platformEmbedding` router with 5 new procedures: `create` (POST), `updateById` (PATCH), `deleteById` (DELETE), `promote` (POST), `testById` (POST) proxying to backend multi-provider endpoints.
  - **Provider cards** — Each active provider displayed as a card with name, Default/Active badges, provider, model, dimensions, max tokens, endpoint URL, availability, credentials status, and last updated.
  - **Add Provider** — Inline form to create additional non-default providers with full validation.
  - **Edit Provider** — Inline edit form per card with pre-populated values, saves via PATCH.
  - **Delete Provider** — Confirmation dialog via overflow menu; disabled for default provider.
  - **Promote to Default** — Overflow menu action to atomically promote any provider to default.
  - **Test Connection** — Per-provider connectivity test with inline result on each card.
  - **Shared component** — `ProviderCard` component in `platform-settings/components/` for reuse by LLM view (025).
  - **Loading/empty/error states** — Card skeletons, empty state prompt, error alert with retry.

- **Platform User Management (023)** — Full user management UI at `/platform-settings/users` for platform admins.
  - **tRPC router** — New `platformUsers` router proxying to backend `/v1/platform-management/*` endpoints (list, deactivate, activate, grant role, revoke role).
  - **Users table** — Paginated table with avatar, display name, email, role badge, status badge, and relative join date.
  - **Search & filters** — Debounced search input (username/display name/email), server-side status filter (All/Active/Inactive), client-side role filter (All/Platform Admin/Regular User).
  - **Actions dropdown** — Per-user dropdown with Grant/Revoke Platform Admin and Activate/Deactivate User actions.
  - **Confirmation dialogs** — Destructive actions (revoke, deactivate) require confirmation; activate does not.
  - **Self-action protection** — Current user cannot deactivate themselves or revoke their own platform_admin role (disabled in dropdown with tooltip).
  - **Last-admin protection** — Backend 409 errors shown as specific toast messages ("Cannot remove the last platform admin").
  - **Loading/empty/error states** — Skeleton rows, empty state with reset-filters button, error alert with retry.
  - **Pagination** — Server-side pagination with Previous/page-numbers/Next navigation.

### Fixed

- **Session redirect loop** — Deactivated users no longer get ERR_TOO_MANY_REDIRECTS. When a session is invalidated server-side, the app layout redirects to `/login?expired=1`, which routes through `/api/auth/clear-session` to delete the stale cookie before showing the login form.

### Added

- **Platform Settings View (022)** — New top-level Platform Settings section for platform admins.
  - **Route group** — `/platform-settings` with sub-routes for Users, Embedding, and LLM (stub pages with "Coming soon").
  - **Layout** — Side navigation following the same pattern as `/settings/layout.tsx`, with `PageHeader` and three nav links (Users, Embedding, LLM).
  - **Menu entry** — "Platform Settings" link with `Shield` icon in the user dropdown menu, visible only to `platform_admin` users.
  - **Route protection** — Layout-level guard redirects non-admin users to `/dashboard`.
  - **Backend extension** — `GET /v1/users/me` now returns `platform_roles` array so the frontend can gate admin visibility.
  - **tRPC update** — `auth.me` procedure surfaces `platform_roles` on the user object.

- **File Preview Mode (021)** — Code/Preview tab bar for HTML, Markdown, and Mermaid files in the file viewer.
  - **Tab bar** — "Code" and "Preview" tabs appear above the code block for previewable file types (`html`, `markdown`, `mermaid`). Defaults to Code tab, resets when navigating between files. Non-previewable files render unchanged with no tab bar.
  - **HTML preview** — Sandboxed `<iframe>` rendering via `srcdoc` with theme-aware base styles (light/dark).
  - **Markdown preview** — `react-markdown` with GFM support (tables, task lists, strikethrough, autolinks) and prose typography (`@tailwindcss/typography`).
  - **Mermaid preview** — Mermaid diagram rendering with dark/light theme support and friendly error display for invalid syntax.
  - **Component structure** — New `CodeViewerWithPreview` wrapper with separate `HtmlPreview`, `MarkdownPreview`, and `MermaidPreview` components.
  - **Dependencies** — Added `react-markdown`, `remark-gfm`, `rehype-raw`, `mermaid`, `@tailwindcss/typography`.

- **Symbol Browser (020)** — Functional symbol browser replacing the placeholder Symbols tab.
  - **Symbol list** — Paginated table showing Name (monospace with qualified name subtitle), Kind (color-coded badge), File (link to file viewer), and Lines columns.
  - **Name search** — Debounced text input for ILIKE substring search with exact-match ranking.
  - **Kind filter** — Dropdown to filter by symbol kind (Function, Class, Interface, Type, Variable, Enum, Method).
  - **Inline detail panel** — Row expansion showing signature, documentation, qualified name, and "View source" link to file viewer at the symbol's line.
  - **Pagination** — Previous/Next navigation with total count display and "Showing X–Y of Z symbols" summary.
  - **tRPC procedures** — `projectSearch.listSymbols` and `projectSearch.getSymbol` wired to backend API, replacing NOT_IMPLEMENTED stubs.
  - **States** — Loading skeleton rows, error alert with retry, empty states for no index / no results / no symbols.

- **File Dependencies & Graph Visualization (019)** — Live dependency card and interactive graph dialog replacing the "Coming soon" Dependencies stub in the file detail sidebar.
  - **Dependencies card** — Sidebar card showing bidirectional dependency information: "Imports (N)" section listing what the file imports (internal files as navigable links, external packages with badge), "Imported By (N)" section listing files that import the current file. Default 5 entries per section with "Show more" toggle.
  - **Dependency graph dialog** — Full-width `@xyflow/react`-powered interactive graph opened via "View dependency graph" button. Features dagre hierarchical auto-layout, custom node rendering (internal files vs. external packages), root node highlighting, depth-based opacity, depth selector (1–5), truncation warning at 200-node cap, pan/zoom/fit-to-view controls.
  - **Node navigation** — Clicking a node in the graph or a file link in the card navigates to that file's detail view.
  - **Lazy loading** — Graph data only fetched when the dialog opens. Depth selector re-fetches with new depth.
  - **tRPC procedures** — `projectFiles.fileDependencies` (bidirectional file-scoped lookup) and `projectFiles.dependencyGraph` (BFS graph traversal).
  - **States** — Loading skeletons, error alert with retry, empty state for files with no dependencies.

- **Code File Viewer (017)** — Full-content code file viewer with syntax highlighting, metadata sidebar, and editorial history.
  - **Dedicated route** — `/projects/[id]/file?path=...` for deep linking and URL sharing, matching the commit detail pattern.
  - **Syntax highlighting** — Shiki-powered code display with `github-dark`/`github-light` themes (follows system preference), line numbers via CSS counters, and plain-text fallback for unsupported languages.
  - **Reusable CodeBlock component** — Designed for reuse in future syntax-highlighted diffs (ticket 016).
  - **File metadata sidebar** — File Info card (path, language, size, lines, last indexed), Editorial History card with paginated commit entries linking to commit detail view, AI Description placeholder, Dependencies placeholder.
  - **Editorial History** — Chronological list of commits that modified the file with short hash, message, author, relative date, change type badge, and line stats. Paginated with "Show more" (cap at 50 visible).
  - **Navigation** — Clicking a file in the Code tab tree navigates to the file viewer. "Back to files" link returns to Code tab. Code tab stays highlighted on the file viewer route.
  - **ChangeTypeBadge extraction** — Extracted from `commit-detail-content.tsx` into shared component for reuse.
  - **tRPC procedures** — `projectFiles.fileContent` (wraps existing files/context endpoint) and `projectFiles.fileHistory` (new endpoint, graceful fallback until backend deploys).
  - **States** — Loading skeletons, file-not-found with `FileX` icon, error alert with retry, independent error handling for code and history.

- **Configurable Max Tokens for Embedding Provider** — Added `max_tokens` field to the embedding provider settings UI.
  - **Summary display** — Effective config summary now shows the Max Tokens value.
  - **Custom form input** — New "Max Tokens" number input (1–131072) in the custom embedding config form.
  - **Validation** — Custom mode requires valid `max_tokens` alongside model and dimensions.
  - **TypeScript types** — Updated `EmbeddingProviderConfig` type and tRPC `customEmbeddingInput` schema with `max_tokens`.

### Added

- **Project Commit Browser (016)** — GitHub-style commit history browser in the project detail view.
  - **Commits tab** — New tab in the project detail tab bar, linking to `/projects/[id]/commits`. Tab order: Code | Commits | Symbols | Search | Indexing | Settings.
  - **Tab rename** — "Overview" tab renamed to "Code".
  - **Commit list** — Paginated table (newest-first) with hash, message subject, author, and relative date. Dual pagination: "Load more" for ≤120 items, page-number navigation for larger histories.
  - **Commit detail view** — `/projects/[id]/commits/[hash]` showing full commit message, author, date, full hash with copy button, clickable parent hashes, and diff stats summary.
  - **Expandable file diffs** — Per-file rows with change type badge (Added/Modified/Deleted/Renamed/Copied), line stats, and expandable unified diff patches. Two-phase fetching: metadata loads immediately, patches fetched lazily on expand.
  - **Diff rendering** — Monospace unified diff with line-level coloring (green additions, red deletions, blue hunk headers), scrollable max-height container.
  - **tRPC procedures** — `projectCommits.listCommits`, `projectCommits.getCommit`, `projectCommits.getCommitDiffs` proxying to Go backend.
  - **States** — Skeleton loading, empty state with `GitCommitHorizontal` icon, error alert.
- **Per-file patch fetching (024)** — Commit browser now fetches patches per-file instead of bulk.
  - **Lazy per-file loading** — Each `FileDiffRow` fetches its own patch via `diffId` query param when expanded, instead of fetching all patches at once.
  - **tRPC `diffId` param** — Added optional `diffId` to `getCommitDiffs` procedure input, passed as `diff_id` query param to the backend.
  - **Bandwidth optimization** — Expanding a single file now transfers only that file's patch instead of all patches in the commit.
- **Project Detail Git File Browser (015)** — GitHub-style file tree browser replacing the placeholder in the project Overview tab's left column.
  - **File tree** — Collapsible directory tree loaded from `GET /v1/projects/{id}/structure`. Directories expand/collapse with chevron icons, files show language-aware icons (extension-based mapping via lucide-react). Sorted: directories first (alphabetical), then files (alphabetical).
  - **File context panel** — Clicking a file replaces the right-column summary cards with a detail card showing file path (monospace), language badge, human-readable size, line count, last indexed timestamp, and a "View in search results" link. Close button restores the default cards.
  - **Loading/empty/error states** — Skeleton tree (10 animated rows), empty state with `FolderSearch` icon and "Trigger Index" CTA, error alert with retry button.
  - **tRPC procedures** — Implemented `projects.structure` and `projectSearch.fileContext` (previously stubbed with `NOT_IMPLEMENTED`).
  - **Cache invalidation** — `triggerIndex` success now also invalidates `projects.structure` so the tree refreshes after re-indexing.
  - **Utility** — Added `formatBytes` to shared format helpers.
- **Real-Time Notifications (016)** — Wired SSE streaming infrastructure to the backoffice UI, replacing dashboard polling with live event-driven updates.
  - **SSE proxy route** — Replaced 501 stub at `/api/events/stream` with a streaming proxy that pipes the Go backend's SSE endpoint directly to the browser.
  - **Per-event-type listeners** — Rewrote the SSE hook to register named listeners for all backend event types: job lifecycle (`job:queued`, `job:started`, `job:progress`, `job:completed`, `job:failed`), `snapshot:activated`, and membership changes (`member:added`, `member:removed`, `member:role_updated`).
  - **Toast notifications** — `job:completed` (success), `job:failed` (error), and membership changes (info) trigger sonner toasts with project name resolution from the tRPC query cache.
  - **React Query cache invalidation** — Job and snapshot events invalidate `dashboard.summary`, `users.listMyProjects`, and `projectIndexing.listJobs`. Membership events invalidate `projectMembers.list`.
  - **Dashboard polling removed** — Removed `refetchInterval: 30_000` from dashboard summary and projects queries; SSE-triggered invalidation is now the primary refresh mechanism.
  - **Events store updated** — `JobEvent` type expanded to match `contracts/events/sse-event.v1.schema.json` (added `event` type, `job_id`, nested `data` object with `job_type`, `files_processed`, `files_total`, `chunks_upserted`, `vectors_deleted`, `error_message`).

### Changed

- **Project Settings Access & Leave Project (014)** — Reworked project settings visibility so all project members can access the Settings tab, not just admin/owner.
  - **Settings tab ungated** — Settings tab and header overflow link now visible to all project members.
  - **Per-section role checks** — Admin-only sections (General, SSH Key, Embedding, LLM, API Keys) hidden for regular members; Members list and Danger Zone always visible.
  - **Read-only members list** — Members with `member` role see the members table without Add/Remove/Role-change actions. Card description changes to "View project members and their roles."
  - **Leave Project** — New `LeaveProjectDialog` confirmation dialog. "Leave" button on own row in members table (all roles, ghost style with `LogOut` icon). "Leave Project" button in Danger Zone for all members. On confirm: redirects to `/projects` with success toast, invalidates project cache.
  - **Last-owner guard** — Catches backend 409 and shows specific toast: "You are the last owner of this project. Transfer ownership before leaving."
  - **Delete Project restricted** — "Delete this project" row in Danger Zone now hidden entirely for non-owners (previously showed a disabled button).

### Added

- **Project Provider Settings (015)** (`/projects/[id]/settings`) — Replaced the embedding-only provider form with backend-driven Embedding and LLM provider cards.
  - **Two provider cards** — Added separate `Embedding Provider` and `LLM Provider` sections with effective-config summaries, source badges, and mode-aware descriptions.
  - **Backend-driven provider discovery** — Provider options now come from `providers.listSupported` (`GET /v1/settings/providers`) instead of a hardcoded frontend constant.
  - **Mode-based selection** — Each card supports `default`, `global`, and `custom` modes, including platform-config selection, project-owned custom configs, and reset-to-default flows.
  - **Credential-safe UX** — Cards expose credential status only (`Credentials managed by platform` / `Credentials saved`) and intentionally hide raw `settings` / `credentials` editors in phase 1.
  - **Connectivity testing** — Added inline connection-test feedback for resolved configs and unsaved custom drafts without requiring a save first.
  - **tRPC procedures** — Added `providers.listSupported`, added the full `projectLLM` router (`get`, `getAvailable`, `put`, `delete`, `getResolved`, `test`), and updated `projectEmbedding` to the new provider-settings contract.
- **SSH Key Management (010)** (`/settings/ssh-keys`) — Full management page for user-scoped SSH deploy keys replacing placeholder.
  - **Keys table** — Name (bold), fingerprint (truncated + tooltip), key type, status badge (Active green / Retired muted), created (relative time + tooltip), and actions dropdown. Rows clickable for detail view. Retired keys shown with reduced opacity. Sorted: active first, then newest.
  - **Create Key dialog** — Tabbed UI (Generate / Import). Generate mode: name input only (backend creates Ed25519). Import mode: name + PEM textarea with monospace font. Post-creation: dialog transitions to key-created view with copyable public key, fingerprint, and type info.
  - **Key Detail dialog** — Full key information (fingerprint, public key in monospace code block, type, status, created date, rotated date if set). Loads and displays assigned projects via `sshKeys.getProjects` with links to project pages. Actions: Copy Public Key, Retire Key (active only).
  - **Retire Key dialog** — Pre-checks project assignments. If projects assigned: shows blocking warning with project list. If no projects: confirmation with destructive Retire button. Handles 409 race condition errors.
  - **Empty state** — KeyRound icon with "No SSH keys yet" heading and Add Key CTA.
  - **Shared components** — Extracted reusable components into `src/components/ssh-keys/` (SSHKeyTable, CreateKeyDialog, KeyDetailDialog, RetireKeyDialog).
  - **tRPC procedures** — Implemented `sshKeys.get`, `sshKeys.getProjects`, and `sshKeys.retire` (previously stubbed with `NOT_IMPLEMENTED`).
  - **UI components** — Added shadcn Textarea and Tabs components.
- **Project Members management (007)** — Full member management section in project settings (`/projects/[id]/settings`), replacing the placeholder card.
  - **Members table** — Lists all project members with avatar (initials fallback), display name, username, role badge (owner/purple, admin/amber, member/muted), relative join date with tooltip, and actions dropdown.
  - **Actions menu** — Per-row `...` dropdown with "Change role" submenu (checkmark on current role, disabled) and "Remove from project" (destructive). Visibility enforces RBAC: admins see actions only on members; owners see actions on all non-self rows; self rows have no actions.
  - **Add Member dialog** — Two-step flow: (1) username/email lookup via `users.lookupUser`, (2) confirmation showing resolved user info (avatar, name, email) before adding. Role selector with Member/Admin (owners also see Owner).
  - **Remove Member dialog** — Destructive confirmation dialog with username and access warning.
  - **Owner promotion dialog** — Extra confirmation when changing a member's role to owner, warning about full project control.
  - **Loading/error/empty states** — Skeleton rows, inline error with retry button, and "No members found" empty state.
  - **tRPC procedures** — Implemented `projectMembers.list`, `.add`, `.updateRole`, `.remove` (previously stubbed with `NOT_IMPLEMENTED`).
- **User email field (015)** — Email added across the full stack.
  - **Profile form** — New email input field (type="email") between username and display name, with save/cancel support.
  - **Signup route** — Accepts and validates `email` in dev registration request body.
  - **OIDC bridge** — `email` added to `GoLoginResponse.user` type.
  - **tRPC `users.updateMe`** — Accepts optional `email` field.
  - **tRPC `users.lookupUser`** — New query procedure (`GET /v1/users/lookup?q={query}`).
  - **Types** — `email` added to `GoUser` (auth), `UserResponse` (users), and `RegisterResponse` (signup).
- **Project API Keys (009)** — API key management section in project settings (`/projects/[id]/settings`).
  - **Keys table** — Reuses shared `ApiKeyTable` component. Displays name, key prefix, role badge, expires, last used, created at, and revoke action.
  - **Create Key dialog** — Reuses shared `CreateKeyDialog` with "Create Project API Key" title. Name, role (read/write), expiry presets. Post-creation key reveal.
  - **Revoke Key dialog** — Reuses shared `DeleteKeyDialog` with project-specific description.
  - **Empty state** — "No API keys for this project yet." with Create Key CTA.
  - **Access control** — Section only visible to admin/owner roles (settings page already admin-gated).
  - **tRPC procedures** — Implemented `projectKeys.list`, `projectKeys.create`, and `projectKeys.delete` (previously stubbed with `NOT_IMPLEMENTED`).
  - **Shared component parameterization** — Added `dialogTitle` prop to `CreateKeyDialog` and `dialogDescription` prop to `DeleteKeyDialog` for context-specific text.
- **Personal API Keys (006)** (`/settings/api-keys`) — Full management page for personal API keys replacing placeholder.
  - **Keys table** — Name, key prefix (monospace), role badge (read/write), expires (relative time), last used, created at, and revoke action. Tooltips show absolute dates.
  - **Create Key dialog** — Name input, role select (read/write, default read), expiry presets (Never/30d/90d/1y/Custom date). Post-creation key reveal with amber warning, monospace plaintext key, and copy-to-clipboard button.
  - **Revoke Key dialog** — Confirmation dialog with destructive action. Soft-deletes the key (immediate removal from list).
  - **Empty state** — Key icon with "No API keys yet" heading and Create Key CTA.
  - **Shared components** — Extracted reusable components into `src/components/api-keys/` (ApiKeyTable, CreateKeyDialog, DeleteKeyDialog, KeyReveal, RoleBadge) for future reuse with project API keys (009).
  - **tRPC procedures** — Implemented `users.listMyKeys`, `users.createMyKey`, and `users.deleteMyKey` (previously stubbed with `NOT_IMPLEMENTED`).
- **Project detail frame (003)** (`/projects/[id]`) — GitHub-like project detail shell with header, tab navigation, and sidebar auto-collapse.
  - **Project header** — Name, repo URL (external link), branch badge, commit hash (7-char), status badge (active/paused), health indicator (dot + label). Right side: "Trigger Index" split button (incremental/full) and overflow menu (Pause/Resume, Copy Repo URL, Settings).
  - **Horizontal tab navigation** — Route-based tabs: Overview, Indexing, Search, Symbols, Settings (admin/owner only). Active tab highlighted with primary border.
  - **Overview tab** (default) — Two-column layout: file tree placeholder (ticket 013) on left, Index Summary card, SSH Deploy Key card (with copyable public key and provider doc links), and Quick Actions card on right.
  - **Settings tab** — General edit form (name, repo URL, branch, status toggle with save/cancel), SSH key management (current key display, reassign from library, generate new inline, remove with confirmation), Members placeholder (ticket 007), API Keys (ticket 009), Danger Zone (delete with type-name-to-confirm, owner-only).
  - **tRPC procedures** — Implemented `projects.get`, `projects.update`, `projects.delete`, `projects.getSSHKey`, `projects.putSSHKey`, `projects.deleteSSHKey`, and `projectIndexing.listJobs` (previously stubbed with `NOT_IMPLEMENTED`).
  - **Custom hooks** — `useProjectDetail` (fetches project with health fields, resolves role from cache), `useProjectDetailMutations` (trigger index, update, delete, SSH key operations with toast feedback and cache invalidation).
- **Create Project assistant (004)** (`/projects/new`) — Guided 4-step wizard replacing the simple Create Project modal.
  - **Step 1 — Project Details** — Repository URL (with inline validation), auto-populated project name from URL slug, and default branch.
  - **Step 2 — SSH Key** — Choose between generating a new Ed25519 deploy key (default, with editable name) or selecting an existing key from the user's library.
  - **Step 3 — Deploy Key** — Displays public key with one-click copy, auto-detected provider deep link (GitHub/GitLab/Bitbucket), collapsible provider-specific instructions, and confirmation checkbox. Project created atomically on "Create Project" click.
  - **Step 4 — Done** — Project summary with "Start First Index" action and navigation to project detail.
  - **Step indicator** — Numbered 1→2→3→4 progress bar with completed/current/future states.
  - **Cancel handling** — Safe navigation with confirmation dialog when an SSH key has already been generated.
  - **Entry points** — "New Project" button in projects list and dashboard empty state now navigate to `/projects/new`.
- **tRPC procedures** — Implemented `sshKeys.list`, `sshKeys.create`, and `projects.create` (previously stubbed with `NOT_IMPLEMENTED`).
- **Projects list page (002)** (`/projects`) — Full project management view replacing placeholder.
  - **Project table** — Name (with repo URL), Branch, Status badge, Last Indexed (relative time + commit tooltip), Role badge, and Actions dropdown. Clickable rows navigate to `/projects/[id]`.
  - **Status derivation** — Visual status badges: Indexing (blue) > Error (red) > Active (green) > Paused (yellow), derived from DB status + health fields.
  - **Client-side filtering** — Text search by project name/repo URL, status filter chips (All/Active/Paused).
  - **Actions dropdown** — Open, Settings, Trigger Index (working), Pause/Resume, Delete (with confirmation dialog). Delete restricted to owners.
  - ~~**Create Project dialog**~~ — Replaced by Create Project assistant (004).
  - **Empty state** — "No projects yet" with Create Project CTA.
  - **30s polling** — Auto-refresh via `refetchInterval`, with stale data warning on refetch errors.
- **User profile page** (`/settings/profile`) — View and edit display name and avatar URL. Shows avatar with initials fallback, `@username`, and member-since date. Inline success/error feedback. Saves via `PATCH /v1/users/me` and refreshes the sidebar avatar.
- **Dashboard overview (001)** — Full zone-based dashboard replacing stat card stubs.
  - **Health Strip** (Zone 1) — Compact bar showing project count, running jobs (blue pulse dot), and failed jobs (red badge). Always visible when projects exist.
  - **Alerts Zone** (Zone 2) — Conditional alert rows for failed jobs, never-indexed projects, and stale indexes (>48h). One alert per project (highest severity wins). Dismissible with 24h localStorage auto-expiry.
  - **Project Health List** (Zone 3) — Table with status dot, name, branch, commit hash, last indexed time, and health label. Sorted by severity then alphabetical. Clickable rows navigate to project detail.
  - **Agent Activity** (Zone 4) — Query count and p95 latency line, only renders when query traffic exists.
  - **Empty State** — "Connect your first repository" CTA when zero projects, replaces all zones.
  - **"Index Now"** — Triggers incremental index silently with sonner toast feedback.
  - **30s polling** — Both summary and project list auto-refresh via `refetchInterval`.
- **tRPC procedures** — Implemented `dashboard.summary`, `users.listMyProjects`, and `projectIndexing.triggerIndex` (previously stubbed with `NOT_IMPLEMENTED`).
- **App shell with collapsible sidebar** — Claude/ChatGPT-style navigation with icon-only collapsed mode, drag rail, and cookie-persisted state (`sidebar_state`).
- **Primary navigation** — Dashboard, Chats, Projects links with active-state highlighting via pathname prefix matching.
- **Contextual sidebar lists** — Recent Projects list (tRPC-powered) when on `/projects/*`, Recent Chats placeholder when on `/chats/*`. Hidden when sidebar is collapsed.
- **User profile dropdown** — Avatar + username in sidebar footer; links to Profile, SSH Keys, System Settings; theme toggle submenu; sign-out action.
- **Dark mode** — System/Light/Dark toggle via `next-themes` with SSR hydration support. Persisted to localStorage.
- ~~**Dashboard stat cards**~~ — Replaced by zone-based dashboard overview (001).
- **SSE event infrastructure** — Zustand store for job events + EventSource hook with exponential backoff reconnection (1s–30s). Gracefully handles 501 stub endpoint.
- **Sidebar auto-collapse** — Sidebar automatically collapses to icon-only when navigating into `/projects/[id]/*`.
- **Placeholder pages** for all planned routes:
  - `/chats`, `/chats/[id]`
  - `/projects`, `/projects/[id]`
  - `/projects/[id]/jobs`, `/projects/[id]/search`, `/projects/[id]/symbols`, `/projects/[id]/settings`, `/projects/[id]/members`
  - `/settings/profile`, `/settings/ssh-keys`, `/settings/api-keys`, `/settings/system`
- **Settings sub-layout** — Side navigation with Profile, SSH Keys, API Keys, System links.

### Superseded

- **Project Embedding Settings (008)** (`/projects/[id]/settings`) — Per-project embedding provider configuration section in project settings.
  - **Superseded by (015)** — The current provider-settings implementation lives under `Project Provider Settings (015)`, which replaced this embedding-only flow.
  - **Config form** — Provider, Endpoint URL, Model, and Dimensions fields with validation. Admins can create/update a project-level override.
  - **Test Connection** — Ad-hoc connectivity test using current form values before saving. Inline success/failure alert.
  - **Reset to Global Default** — Confirmation dialog to remove the project override and fall back to the global config.
  - **Source indicator** — Card description shows whether the project uses a project override or the global default.
  - **Read-only view** — Members see the resolved config as read-only text without action buttons.
  - **tRPC procedures** — Implemented `projectEmbedding.get`, `.put`, `.delete`, `.getResolved`, `.test` (previously stubbed with `NOT_IMPLEMENTED`).

### Changed

- **Project detail layout** — Rewritten from passthrough wrapper to client layout with project header and horizontal tab navigation.
- **Project detail pages** — `/projects/[id]/jobs`, `/projects/[id]/search`, `/projects/[id]/symbols` updated from generic "Coming soon" to ticket-specific placeholders.
- **Root layout** — Wrapped with `ThemeProvider` from `next-themes`; added `suppressHydrationWarning` to `<html>`.
- **App layout** — Transformed from bare auth guard to full `SidebarProvider` + `AppSidebar` + `SidebarInset` shell.
- **Dashboard page** — Rewritten from stat cards to conditional zone layout with health-aware project list, alerts, and agent activity.
- **DashboardSummary type** — Fixed `jobs_running` → `jobs_active` to match backend API.
- **UserProject type** — Extended with 8 nullable health fields for index snapshot, active job, and failed job data.
- **App layout** — Added sonner `<Toaster />` for toast notifications.

### Removed

- **Logout button component** (`dashboard/logout-button.tsx`) — Sign-out moved to user profile dropdown in sidebar.
- **Members page** (`/projects/[id]/members`) — Members management moved to Settings tab section.

### Dependencies

- Added `@xyflow/react` (interactive graph visualization for dependency graphs)
- Added `@dagrejs/dagre` (hierarchical auto-layout for graph node positioning)
- Added `sonner` (toast notifications)
- Added shadcn `table` component
- Added `zustand` (SSE event state management)
- Added `next-themes` (dark mode with SSR support)
- Added shadcn components: `sidebar`, `sheet`, `separator`, `tooltip`, `avatar`, `dropdown-menu`, `skeleton`, `badge`
- Added shadcn `dialog` and `select` components
- Added shadcn `checkbox` and `collapsible` components
