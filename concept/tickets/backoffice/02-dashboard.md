# 02 — Dashboard & Real-Time Events

## Status
Done

## Goal
Built the main landing page after login that answers "Is everything indexed and healthy, and are my agents using it?" The dashboard renders progressively with conditional zones that only appear when there is real data to show. Also wired up the SSE streaming infrastructure for real-time event delivery, toast notifications, and TanStack Query cache invalidation across the application.

## Depends On
01-app-shell

## Scope

### Dashboard Page (`/dashboard`)

Four conditional zones, plus a zero-project empty state.

**Empty State** (zero projects): centered layout with "Connect your first repository" heading, subtext, and "Create Project" CTA navigating to `/projects`. Replaces all zones when `projects_total === 0`.

**Zone 1 — Health Strip** (always visible when projects exist): compact horizontal bar at the top showing "N projects", "N running" (teal dot when > 0), "N failed" (red badge when > 0). Visually quiet when healthy, draws attention only when something is wrong.

**Zone 2 — Alerts** (conditional, only when actionable): dismissible alert rows for failed jobs, never-indexed projects, and stale indices (> 48h). At most one alert per project (highest severity wins). Dismissed state stored in `localStorage` per alert ID with 24h auto-expiry. "Index Now" action triggers incremental index + sonner toast. Alert IDs follow format `alert:{projectId}:{type}`.

**Zone 3 — Project Health List** (always visible when projects exist): table with one row per project showing status dot (green/yellow/red/grey), name (clickable to `/projects/:id`), branch, commit hash (7-char monospace), relative last-indexed time, and health label. Sorted by severity (failed > stale > never indexed > healthy), then alphabetical. Running jobs show "Indexing..." with pulse animation.

**Zone 4 — Agent Activity** (conditional, only when `query_count_24h > 0`): single summary line showing query count and p95 latency over 24h.

### tRPC Procedures

| Procedure | Backend Endpoint | Purpose |
|---|---|---|
| `dashboard.summary` | `GET /v1/dashboard/summary` | Aggregate counts: projects_total, jobs_active, jobs_failed_24h, query_count_24h, p95_latency_ms_24h |
| `users.listMyProjects` | `GET /v1/users/me/projects` | All user projects with health fields (index_git_commit, index_branch, index_activated_at, active_job_id, active_job_status, failed_job_id, failed_job_finished_at, failed_job_type) |

### SSE Real-Time Events

**SSE Proxy Route** (`src/app/api/events/stream/route.ts`): streams Go backend's `GET /v1/events/stream` through to the browser with cookie forwarding. No buffering.

**Event Hook** (`src/hooks/use-sse-connection.ts`): registers per-event-type listeners for `connected`, `job:queued`, `job:started`, `job:progress`, `job:completed`, `job:failed`, `snapshot:activated`, `member:added`, `member:removed`, `member:role_updated`. Receives tRPC `utils` for cache invalidation.

**Events Store** (`src/stores/events-store.ts`): Zustand store with `connected` boolean, `jobEvents` rolling buffer (100 events), `lastEventAt` timestamp. Membership events are fire-and-forget (toast + invalidation only, not stored).

**Toast Notifications** (sonner): `job:completed` (success), `job:failed` (error), `member:added/removed/role_updated` (info). Project names resolved from `listMyProjects` query cache, fallback to truncated UUID.

**Query Invalidation on SSE Events**:
- Job events: invalidate `dashboard.summary`, `users.listMyProjects`, `projectIndexing.listJobs`
- `snapshot:activated`: invalidate `dashboard.summary`, `users.listMyProjects`
- Member events: invalidate `projectMembers.list`

**Dashboard Polling Removal**: `refetchInterval: 30_000` removed from dashboard queries once SSE is active. `refetchOnWindowFocus: true` retained as safety net.

### Dashboard Utility Library

| File | Purpose |
|---|---|
| `dashboard-types.ts` | `ProjectWithHealth`, `HealthStatus`, `DashboardSummary` types |
| `health-utils.ts` | `getProjectHealthStatus()`, `getStatusDotColor()`, `getHealthLabel()`, `formatRelativeTime()` |
| `use-dismissed-alerts.ts` | localStorage hook with 24h auto-expiry for alert dismissal |

## Key Files

| File | Purpose |
|---|---|
| `src/app/(app)/dashboard/page.tsx` | Server component delegating to DashboardContent |
| `src/app/(app)/dashboard/dashboard-content.tsx` | Zone orchestration, query hooks, conditional rendering |
| `src/app/(app)/dashboard/components/empty-state.tsx` | Zero-project empty state |
| `src/app/(app)/dashboard/components/health-strip.tsx` | Zone 1: compact stat bar |
| `src/app/(app)/dashboard/components/alerts-zone.tsx` | Zone 2: conditional alert list |
| `src/app/(app)/dashboard/components/alert-row.tsx` | Single dismissible alert row |
| `src/app/(app)/dashboard/components/project-health-list.tsx` | Zone 3: project table |
| `src/app/(app)/dashboard/components/project-health-row.tsx` | Single project row |
| `src/app/(app)/dashboard/components/agent-activity.tsx` | Zone 4: query stats line |
| `src/app/(app)/dashboard/lib/dashboard-types.ts` | Shared TypeScript types |
| `src/app/(app)/dashboard/lib/health-utils.ts` | Health status computation and formatting |
| `src/app/(app)/dashboard/lib/use-dismissed-alerts.ts` | Alert dismissal with localStorage |
| `src/app/api/events/stream/route.ts` | SSE proxy route to Go backend |
| `src/hooks/use-sse-connection.ts` | SSE event hook with per-event-type listeners |
| `src/stores/events-store.ts` | Zustand store for real-time events |
| `src/server/api/routers/dashboard.ts` | Dashboard tRPC router |

## Acceptance Criteria
- [x] Zero projects renders only the empty state with "Create Project" CTA
- [x] Health strip shows project count, running jobs (teal dot), and failed jobs (red badge)
- [x] Alerts zone renders at most one alert per project with highest severity
- [x] Dismissed alerts persist in localStorage and auto-expire after 24h
- [x] "Index Now" triggers incremental index and shows sonner toast
- [x] Project health list sorted by severity, then alphabetical
- [x] Running jobs show "Indexing..." with pulse animation
- [x] Project rows navigate to `/projects/:id` on click
- [x] Agent activity only renders when query traffic exists
- [x] SSE proxy streams backend events without buffering
- [x] SSE hook registers listeners for all event types (job, snapshot, member)
- [x] Toast notifications fire for job:completed, job:failed, and member events
- [x] TanStack Query caches invalidated on relevant SSE events
- [x] Dashboard polling removed; SSE-driven invalidation is primary refresh mechanism
- [x] Jobs tab polling (3s) retained as fallback alongside SSE
