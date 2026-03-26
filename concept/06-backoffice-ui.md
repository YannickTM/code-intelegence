# Backoffice UI v8 (Next.js + TypeScript + tRPC)

## Role

Backoffice is the operational console for:

- Managing multiple projects, SSH keys, and API keys
- Creating reusable SSH keys and assigning them to projects
- Configuring embedding and LLM providers with connectivity testing
- Triggering and observing indexing jobs with real-time progress
- Exploring indexed artifacts: files, symbols, dependencies, search quality
- Browsing commit history and file-level diffs
- Monitoring platform health across all services

## Authentication

Authentication uses OIDC/OAuth via better-auth with session-based cookies. The Go backend owns user records and sessions; better-auth handles only the OIDC redirect flow, storing ephemeral state in an in-memory SQLite database.

Login flow:

1. User visits `/login` and initiates the OIDC sign-in.
2. better-auth completes the OAuth redirect and obtains the identity token.
3. A bridge endpoint calls the Go backend `POST /v1/auth/login` to create a backend session.
4. The backend session cookie is set in the browser for all subsequent requests.

OIDC provider configuration is driven by environment variables (`OIDC_DISCOVERY_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`). All three must be set together or all omitted.

API keys (used by MCP agents) are a separate concern from backoffice access. API keys control which projects an agent can query; backoffice access controls who can manage the platform.

## Stack

| Area | Choice |
|---|---|
| App framework | Next.js (App Router) |
| Language | TypeScript |
| API layer | tRPC (server + client) |
| Server state | TanStack Query (via @trpc/react-query) |
| Local UI state | Zustand |
| Styling | Tailwind CSS |
| Component library | shadcn/ui (Radix primitives) |
| Icons | Lucide React |
| Syntax highlighting | Shiki |
| Graph visualization | @xyflow/react + dagre |
| Markdown rendering | react-markdown + remark-gfm + rehype-raw |
| Toasts | Sonner |
| Real-time events | SSE via EventSource API |
| Validation | Zod |
| Package manager | pnpm |

## Application Layout

The app uses Next.js route groups:

- `(auth)` — unauthenticated routes: login page
- `(app)` — authenticated routes: sidebar shell with all operational views

The `(app)` layout provides a persistent sidebar with navigation sections for Dashboard, Projects, Settings, Platform Settings, and Chats.

## SPA Sections

### 1. Dashboard (Cross-Project)

- Total project count, active/failed jobs across all projects
- Recent job timeline with project labels
- Real-time job status updates via SSE
- Quick links to projects needing attention

### 2. Projects

- List all projects the current user has access to, with status and last indexed commit
- Create new project with repository URL, branch, and SSH key assignment
- Project detail view with sub-navigation:
  - **Overview**: project status, active branch, last index timestamp, trigger full or incremental index
  - **Jobs**: per-project job history with live progress indicators and error details
  - **Search**: semantic search tester with language/symbol_type/file_pattern filters, preview scores and matched chunks
  - **Symbols**: searchable list of functions, classes, methods, interfaces, types with declaration signatures and line ranges
  - **File browser**: navigable file tree of indexed files with file detail navigation
  - **Commits**: project commit timeline with per-commit file diffs
  - **Settings**: project configuration, SSH key assignment, branch management, member management

#### SSH Key Panel (per project)

Displayed within the project settings view:

- Shows the currently assigned SSH key label and fingerprint
- Dropdown to assign a different existing SSH key
- Quick action to create a new SSH key and assign it immediately
- Public key shown in a copyable text field for Git provider setup
- Warning when reassigning keys
- Instructions for adding deploy keys to common Git providers

### 3. File Detail

Accessed from the file browser or symbol navigation within a project:

- Code viewer with Shiki syntax highlighting and line numbers
- Exports panel: symbols exported from the file
- References panel: inbound references to symbols in this file
- JSX usages: component usage locations across the project
- Network calls: HTTP/API calls detected in the file
- File metadata: language, line count, file hash, chunk count

### 4. Commit History

Accessed from the project commits sub-navigation:

- Project commit timeline with hash, author, message, and timestamp
- Commit detail view with file-level diffs
- Navigation to affected files in the file browser

### 5. Keys and Access

Located under Settings in the sidebar:

#### SSH Keys (Git)

- Create SSH keys in a centralized key library
- View key fingerprint and public key, copy to clipboard
- View project usage list per key (which projects currently use this key)
- Retire keys only when no projects are actively assigned

#### API Keys (MCP Access)

- Create API keys with multi-project access (select one or more projects)
- Revoke keys
- Show usage, last-used timestamps, and project access list per key

### 6. Platform Settings (Provider Configuration)

Located under Platform Settings in the sidebar, accessible to authorized users:

#### Embedding Configuration

- View and edit embedding provider endpoint URL and model name
- Display configured vector dimensions
- "Test Connection" button that pings the provider and reports success/failure
- Warning banner when no config is set or endpoint is unreachable
- Notice when changing model: existing indexes require full re-index

#### LLM Configuration

- View and edit LLM provider endpoint URL and model name
- Test connectivity

#### Users

- View and manage platform user accounts and roles

#### Workers

- View backend worker status and configuration

### 7. System

Located under Settings in the sidebar:

- Service health checks: backend-api, PostgreSQL, Qdrant, Redis, Ollama
- Connectivity status for each service
- Version and migration status

## tRPC Router Structure

The backoffice communicates with the Go backend exclusively through tRPC server procedures, which call the backend HTTP API. Key routers:

| Router | Responsibility |
|---|---|
| `auth` | Login flow, session management |
| `dashboard` | Cross-project summary and statistics |
| `projects` | Project CRUD, listing |
| `projectMembers` | Per-project member management |
| `projectIndexing` | Job listing, trigger index |
| `projectSearch` | Semantic search queries |
| `projectFiles` | File tree, file metadata |
| `projectCommits` | Commit history and diffs |
| `projectKeys` | Per-project API key management |
| `projectEmbedding` | Per-project embedding configuration |
| `projectLLM` | Per-project LLM configuration |
| `sshKeys` | SSH key library CRUD |
| `users` | Current user info, project list |
| `platformEmbedding` | Platform-wide embedding settings |
| `platformLLM` | Platform-wide LLM settings |
| `platformUsers` | User administration |
| `platformWorkers` | Worker status |
| `providers` | Provider connectivity testing |

All tRPC procedures run server-side and call `apiCall()` to the Go backend with forwarded session cookies.

## Real-Time Integration

The backoffice receives live events via Server-Sent Events from the backend, proxied through a Next.js API route:

```text
GET /api/events/stream  →  SSE connection (proxied to backend)
```

### Event Handling Pattern

- SSE events update Zustand store (`useEventsStore`) for immediate UI reactivity
- TanStack Query caches are selectively invalidated on relevant events
- Job list, dashboard counters, and project status reflect changes within seconds
- SSE reconnection uses exponential backoff (1s to 30s max)

### Events Consumed

| Event | UI Effect |
|---|---|
| `job:queued` | Job appears in list with "queued" badge |
| `job:started` | Badge updates to "running", spinner shown |
| `job:progress` | Progress bar updates (files processed / total) |
| `job:completed` | Badge updates to "completed", dashboard counters refresh, toast notification |
| `job:failed` | Badge updates to "failed", error detail available, toast notification |
| `snapshot:activated` | Project detail shows new active commit |
| `member:added` | Member list invalidated, toast notification |
| `member:removed` | Member list invalidated, toast notification |
| `member:role_updated` | Member list invalidated, toast notification |

## UX Requirements

- Clear job state transitions: queued, running, completed, failed
- Error detail with copyable diagnostics
- Strong filtering and search in table-heavy views
- Multi-project navigation as the primary entry point
- SSH key copy action with clear feedback (toast notification)
- Project creation blocks submit until an SSH key is selected
- Provider settings validate endpoint URL format before saving
- Dark mode support via next-themes

## Docker Notes

- `backoffice` container serves Next.js on port 3000 internally (5173 published)
- Environment variables:
  - `BETTER_AUTH_SECRET` — session signing secret
  - `OIDC_DISCOVERY_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` — OIDC provider config
  - `API_BASE_URL` — Go backend URL (defaults to `http://backend-api:8080` in Docker network)
- Depends on `backend-api` health readiness
- No direct connections to PostgreSQL, Qdrant, Redis, or Ollama — all data access goes through `backend-api`

## Non-Goals

- In-browser code editor
- Real-time collaborative sessions
- Built-in username/password account system (OIDC-only)
