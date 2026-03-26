# 00 — Backoffice Component Overview

## Status
Done

## Goal
The backoffice is the operational console for managing projects, monitoring indexing jobs, browsing indexed code, and configuring platform providers. It provides project owners, admins, and members with a unified interface to connect repositories, observe indexing health, search code, and manage access. Built as a Next.js application communicating with the Go backend exclusively through tRPC server procedures.

## Depends On
None (root ticket)

## Scope

### Tech Stack

| Area | Choice |
|---|---|
| App framework | Next.js 16 (App Router) |
| Language | TypeScript |
| API layer | tRPC (server + client via `@trpc/react-query`) |
| Server state | TanStack Query (via `@trpc/react-query`) |
| Local UI state | Zustand |
| Styling | Tailwind CSS v4 |
| Component library | shadcn/ui (Radix primitives) |
| Icons | Lucide React |
| Syntax highlighting | Shiki |
| Graph visualization | @xyflow/react + dagre |
| Markdown rendering | react-markdown + remark-gfm + rehype-raw |
| Toasts | Sonner |
| Real-time events | SSE via native EventSource API |
| Auth | better-auth (OIDC/OAuth, GitHub provider) |
| Theming | next-themes (dark/light/system) |
| Validation | Zod |
| Package manager | pnpm |

### Page Route Map

```
/                              redirect to /dashboard
/login                         GitHub OAuth login (unauthenticated)
/dashboard                     Cross-project health overview
/projects                      Project list with search and filters
/projects/new                  Create project wizard (4-step assistant)
/projects/:id                  Project detail — Overview tab (default)
/projects/:id/file             File detail viewer with analysis cards
/projects/:id/jobs             Indexing jobs tab
/projects/:id/search           Full-text code search tab
/projects/:id/symbols          Symbol browser tab
/projects/:id/commits          Commit history tab
/projects/:id/settings         Project settings tab (admin+)
/settings/profile              User profile
/settings/ssh-keys             SSH key library
/settings/api-keys             Personal API keys
/settings/system               System health
/platform/users                Platform user management
/platform/embedding            Platform embedding settings
/platform/llm                  Platform LLM settings
/platform/workers              Worker status
```

### tRPC Router Table (18 routers)

| Router | File | Responsibility |
|---|---|---|
| `auth` | `routers/auth.ts` | Session management, current user |
| `dashboard` | `routers/dashboard.ts` | Cross-project summary statistics |
| `projects` | `routers/projects.ts` | Project CRUD, detail, SSH key assignment |
| `projectMembers` | `routers/project-members.ts` | Per-project member management |
| `projectIndexing` | `routers/project-indexing.ts` | Job listing, trigger index |
| `projectSearch` | `routers/project-search.ts` | Full-text code search, symbol queries |
| `projectFiles` | `routers/project-files.ts` | File tree, file metadata, dependencies |
| `projectCommits` | `routers/project-commits.ts` | Commit history and diffs |
| `projectKeys` | `routers/project-keys.ts` | Per-project API key management |
| `projectEmbedding` | `routers/project-embedding.ts` | Per-project embedding configuration |
| `projectLLM` | `routers/project-llm.ts` | Per-project LLM configuration |
| `sshKeys` | `routers/ssh-keys.ts` | SSH key library CRUD |
| `users` | `routers/users.ts` | Current user info, project list, user lookup |
| `providers` | `routers/providers.ts` | Provider connectivity testing |
| `platformEmbedding` | `routers/platform-embedding.ts` | Platform-wide embedding settings |
| `platformLLM` | `routers/platform-llm.ts` | Platform-wide LLM settings |
| `platformUsers` | `routers/platform-users.ts` | User administration |
| `platformWorkers` | `routers/platform-workers.ts` | Worker status |

All tRPC procedures run server-side and call `apiCall()` to the Go backend with forwarded session cookies.

### Ticket Progression

| Ticket | Title | Scope |
|---|---|---|
| 00-overview | Component Overview | This document |
| 01-app-shell | App Shell, Routing, Auth & Theming | Layout, sidebar, auth, themes, tRPC setup |
| 02-dashboard | Dashboard & Real-Time Events | Health strip, alerts, project list, SSE |
| 03-projects | Project List, Create Wizard & Detail Frame | List, wizard, detail layout, tabs |
| 04-project-members-ssh | Project Members & SSH Keys | Member management, SSH key library |
| 05-project-providers-keys | Provider Settings & API Keys | Embedding/LLM config, project API keys |
| 06-indexing-monitor | Indexing Jobs Monitor | Jobs table, status badges, polling |
| 07-code-search | Full-Text Code Search | Search UI, filters, highlighted results |

## Key Files

| File / Package | Purpose |
|---|---|
| `backoffice/package.json` | Dependencies and scripts |
| `backoffice/src/app/(app)/layout.tsx` | Authenticated app shell layout |
| `backoffice/src/app/(auth)/login/page.tsx` | Login page |
| `backoffice/src/server/api/root.ts` | tRPC router aggregation (18 routers) |
| `backoffice/src/server/api/trpc.ts` | tRPC context and procedure factories |
| `backoffice/src/server/api/lib/api-call.ts` | Backend HTTP client with cookie forwarding |
| `backoffice/src/components/app-sidebar.tsx` | Sidebar navigation with auto-collapse |
| `backoffice/src/hooks/use-sse-connection.ts` | SSE event hook with reconnection |
| `backoffice/src/stores/events-store.ts` | Zustand store for real-time events |

## Acceptance Criteria
- [x] Next.js 16 App Router project with TypeScript and Tailwind CSS v4
- [x] tRPC client/server setup with 18 routers registered in `root.ts`
- [x] better-auth OIDC integration with GitHub OAuth provider
- [x] shadcn/ui component library installed and configured
- [x] All page routes listed above are accessible with auth guards
- [x] SSE connection infrastructure for real-time event delivery
- [x] Dark/light theme support via next-themes
- [x] Zustand store for client-side event state
- [x] Sonner toast provider mounted in app layout
