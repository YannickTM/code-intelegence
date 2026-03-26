# 01 — App Shell, Routing, Auth & Theming

## Status
Done

## Goal
Built the foundational application shell that all authenticated views render inside. This includes the collapsible sidebar with navigation, GitHub OAuth login via better-auth, session management with auth guards, dark/light theme support, tRPC client/server wiring, the shadcn/ui component library, and the SSE connection infrastructure. Deactivated users are redirected to a blocked page.

## Depends On
00-overview

## Scope

### Authentication
- OIDC/OAuth login flow using better-auth with a GitHub provider
- Environment-driven configuration: `OIDC_DISCOVERY_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`
- Bridge endpoint calls Go backend `POST /v1/auth/login` to create a backend session
- Backend session cookie set in browser for all subsequent tRPC requests
- Login page at `/login` with GitHub sign-in button
- Auth guard on `(app)` route group redirects unauthenticated users to `/login`
- Deactivated user check via `auth.me` query; redirects to a blocked page if user is inactive
- Logout via `POST /v1/auth/logout` clears session cookie

### App Shell Layout
- Next.js `(app)/layout.tsx` wraps all authenticated routes with `SidebarProvider` + `AppSidebar` + `SidebarInset`
- Collapsible sidebar (Claude/ChatGPT-style) with cookie-based state persistence (`sidebar_state`)
- Sidebar structure (top to bottom):
  - App logo/wordmark (icon-only when collapsed)
  - Primary nav: Dashboard, Projects
  - Contextual list: Recent Chats (default) or Recent Projects (when on `/projects` routes)
  - Platform Settings nav section (admin-only)
  - User profile menu (pinned to bottom): avatar, username, dropdown with Profile, SSH Keys, theme toggle, Sign Out
- Sidebar auto-collapses to icon-only when entering `/projects/[id]` routes (`SidebarAutoCollapse` component)
- Collapsed sidebar shows icons with hover tooltips

### Routing
- Route groups: `(auth)` for unauthenticated, `(app)` for authenticated
- Root `/` redirects to `/dashboard`
- All `(app)` routes protected by session check
- Settings sub-layout at `/settings/*` with left nav (Profile, SSH Keys, API Keys, System)
- Platform settings at `/platform/*` (Users, Embedding, LLM, Workers)

### Theming
- `next-themes` ThemeProvider wrapping the root layout
- Three modes: light, dark, system
- Theme toggle in user profile dropdown menu
- `suppressHydrationWarning` on root `<html>` for SSR compatibility

### tRPC Setup
- Server-side tRPC context factory with session cookie forwarding
- `apiCall()` helper for HTTP requests to Go backend with cookie passthrough
- `createTRPCRouter` and `protectedProcedure` factories
- Client-side tRPC provider with TanStack Query integration
- 18 routers registered in `root.ts`

### shadcn/ui
- Component library configured in `components.json`
- Initial components installed: Sidebar, Sheet, Separator, Tooltip, Avatar, DropdownMenu, Skeleton, Badge, Button
- Radix primitives via `radix-ui` package

### SSE Infrastructure
- `useSSEConnection` hook: connects to `/api/events/stream`, exponential backoff reconnection (1s to 30s)
- Zustand `events-store.ts`: rolling buffer of 100 job events, connected state
- `AppShellClient` component mounts the SSE hook in the shell layout
- Gracefully handles 501 (backend SSE not ready) with backoff retry

### tRPC Routers (initial)
- `auth.me` — returns current user session
- `dashboard.summary` — aggregate health counts (stubbed initially)
- `users.listMyProjects` — projects the current user has access to

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/layout.tsx` | Authenticated shell: SidebarProvider + AppSidebar + SidebarInset |
| `src/app/(auth)/login/page.tsx` | GitHub OAuth login page |
| `src/app/layout.tsx` | Root layout with ThemeProvider |
| `src/components/app-sidebar.tsx` | Sidebar: nav, contextual lists, auto-collapse |
| `src/components/app-shell-client.tsx` | Mounts SSE connection hook |
| `src/components/theme-provider.tsx` | next-themes wrapper |
| `src/components/theme-toggle.tsx` | Light/Dark/System dropdown |
| `src/components/nav-user.tsx` | Sidebar footer user dropdown |
| `src/components/logo.tsx` | TreePine icon + wordmark |
| `src/components/recent-projects-list.tsx` | tRPC-powered recent projects sidebar list |
| `src/hooks/use-sse-connection.ts` | EventSource hook with backoff |
| `src/stores/events-store.ts` | Zustand event store |
| `src/server/api/root.ts` | tRPC router aggregation |
| `src/server/api/trpc.ts` | tRPC context and procedure factories |
| `src/server/api/lib/api-call.ts` | Backend HTTP client |

## Acceptance Criteria
- [x] GitHub OAuth login flow works end-to-end with session cookie set
- [x] Unauthenticated users redirected to `/login`
- [x] Deactivated users redirected to blocked page
- [x] Sidebar renders with nav items, contextual list, and user profile menu
- [x] Sidebar collapses/expands with state persisted via cookie
- [x] Sidebar auto-collapses on `/projects/[id]` route entry
- [x] Dark/light/system theme toggle works with SSR hydration
- [x] tRPC client/server wiring functional with cookie forwarding to backend
- [x] SSE connection established with reconnection on failure
- [x] Zustand store receives and buffers SSE events
- [x] All route groups properly guarded
- [x] shadcn/ui components installed and rendering correctly
- [x] Sonner `<Toaster>` mounted in app layout
