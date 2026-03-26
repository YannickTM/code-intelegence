# Backoffice

## Role

Management dashboard for the MYJUNGLE Code Intelligence Platform. Provides project lifecycle management, indexing control, code search and exploration, provider configuration, and platform administration.

## Technology Stack

| Layer | Technology |
|---|---|
| Framework | Next.js 16 (App Router) |
| API layer | tRPC 11 + TanStack Query |
| Authentication | better-auth (OIDC) |
| UI components | shadcn/ui + Radix UI |
| Styling | Tailwind CSS 4 |
| State management | Zustand |
| Graph visualization | React Flow (@xyflow/react) + dagre |
| Diagrams | Mermaid |
| Code highlighting | Shiki |
| Markdown | react-markdown + remark-gfm |
| Testing | Vitest |
| Package manager | pnpm 10 |

## Authentication

OIDC via better-auth. The backoffice authenticates users through a configurable OIDC provider (WorkOS, Keycloak, etc.) and maintains sessions via better-auth. User identity is linked to the backend-api user model.

## Pages

### Auth

- `/login` — OIDC login

### Dashboard

- `/dashboard` — Platform summary (projects, vectors, jobs, query volume)

### Projects

- `/project` — Project list
- `/project/create` — Create new project
- `/project/[id]` — Project detail
- `/project/[id]/jobs` — Indexing job history
- `/project/[id]/search` — Semantic code search
- `/project/[id]/symbols` — Symbol browser
- `/project/[id]/commits` — Commit history
- `/project/[id]/commits/[hash]` — Commit detail with diffs
- `/project/[id]/file` — File viewer
- `/project/[id]/settings` — Project configuration

### User Settings

- `/settings/profile` — User profile
- `/settings/api-keys` — Personal API key management
- `/settings/ssh-keys` — SSH key library
- `/settings/system` — System settings

### Platform Administration

- `/platform-settings` — Admin overview
- `/platform-settings/embedding` — Global embedding provider configuration
- `/platform-settings/llm` — Global LLM provider configuration
- `/platform-settings/users` — User management
- `/platform-settings/workers` — Worker status monitoring

### Chat

- `/chats` — Chat interface
- `/chats/[id]` — Chat session

## Server API Integration

The backoffice uses tRPC routers that proxy requests to backend-api HTTP endpoints. Each router maps to a backend domain:

- `auth` — Login/logout, session management
- `dashboard` — Platform summary stats
- `projects` — Project CRUD
- `project-indexing` — Job triggers and history
- `project-search` — Code search
- `project-files` — File browser and artifacts
- `project-commits` — Commit history and diffs
- `project-members` — Membership management
- `project-keys` — Project API keys
- `project-embedding` — Per-project embedding config
- `project-llm` — Per-project LLM config
- `platform-embedding` — Global embedding settings
- `platform-llm` — Global LLM settings
- `platform-users` — User administration
- `platform-workers` — Worker status
- `providers` — Available provider list
- `ssh-keys` — SSH key library
- `users` — User profile

Real-time updates are received via SSE from `backend-api` and distributed to components through a Zustand event store.

## Configuration

Copy [`.env.example`](./.env.example) to `.env` and populate:

- `API_BASE_URL` — Backend API base URL (e.g., `http://localhost:8080`)
- `BETTER_AUTH_SECRET` — Session signing secret (min 32 chars)
- `BETTER_AUTH_URL` — Public URL of this Next.js app
- `OIDC_DISCOVERY_URL` — OIDC provider discovery endpoint
- `OIDC_CLIENT_ID` — OIDC client ID
- `OIDC_CLIENT_SECRET` — OIDC client secret
- `NEXT_PUBLIC_OIDC_PROVIDER_ID` — Client-side provider identifier

## Docker Build

Multi-stage build on Node 22 Alpine with pnpm. Produces a standalone Next.js output running as non-root on port 3000. See `Dockerfile`.

## Local Development

```bash
# Install dependencies
pnpm install

# Development server (Turbopack)
pnpm dev

# Type checking
pnpm typecheck

# Lint + type check
pnpm check

# Run tests
pnpm test

# Production build
pnpm build
pnpm start
```
