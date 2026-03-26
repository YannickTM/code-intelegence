# MCP Interface v8 (TypeScript)

## Role

The MCP server is the agent-facing compatibility layer. It exposes stable MCP tools and delegates all business logic to the Go backend.

## Why Separate MCP Service

- MCP transport and protocol can evolve independently of the backend
- Agent-specific concerns (tool schemas, response shaping, default injection) stay out of backend core
- Stdio and SSE entrypoints can be supported without backend changes
- Tool schema versioning can be managed without backend redeploy coupling

## Stack

- TypeScript + Node.js
- `@modelcontextprotocol/sdk`
- HTTP client to Go backend with timeout and retry policy
- Minimal runtime — no ORM, no database drivers

## Transport Modes

- `stdio`: local desktop/CLI agent integrations (direct process pipe)
- `SSE`: remote shared server integrations (HTTP endpoint)

Both transports expose the same tool set and response contracts.

## Tool Set

All 6 tools are available. Every tool requires `project_id` (UUID).

### 1. search_code

Semantic code search within one project.

- **Backend**: `POST /v1/projects/{project_id}/query/search`
- **Required**: `project_id`, `query`
- **Optional**: `language`, `symbol_type`, `file_pattern`, `limit` (default 10)
- **Language filter**: supports 28 languages

### 2. get_symbol_info

Read symbol details by symbol ID or name.

- **Backend**: `GET /v1/projects/{project_id}/symbols/{symbol_id}`
- **Required**: `project_id`
- **Optional**: `symbol_id` (UUID), `symbol_name`
- When `symbol_id` is provided, fetches the specific symbol. When `symbol_name` is provided, searches by name.

### 3. get_dependencies

Read dependency edges for a project, optionally filtered by file or symbol.

- **Backend**: `GET /v1/projects/{project_id}/dependencies`
- **Required**: `project_id`
- **Optional**: `file_path`, `symbol_name`, `limit` (default 100)

### 4. get_project_structure

Read file tree and module structure.

- **Backend**: `GET /v1/projects/{project_id}/structure`
- **Required**: `project_id`
- **Optional**: `depth` (default 6)

### 5. get_file_context

Read contextual code window around a file path and line.

- **Backend**: `GET /v1/projects/{project_id}/files/context`
- **Required**: `project_id`, `file_path`
- **Optional**: `line` (minimum 1), `radius_lines` (default 80)

### 6. get_conventions

Read inferred project conventions.

- **Backend**: `GET /v1/projects/{project_id}/conventions`
- **Required**: `project_id`
- **Optional**: `language`, `limit` (default 20)

## Tool-to-Backend Mapping

| MCP Tool | Method | Backend Endpoint |
|---|---|---|
| `search_code` | POST | `/v1/projects/{id}/query/search` |
| `get_symbol_info` | GET | `/v1/projects/{id}/symbols/{symbolId}` |
| `get_dependencies` | GET | `/v1/projects/{id}/dependencies` |
| `get_project_structure` | GET | `/v1/projects/{id}/structure` |
| `get_file_context` | GET | `/v1/projects/{id}/files/context` |
| `get_conventions` | GET | `/v1/projects/{id}/conventions` |

## Authentication Model

- MCP caller passes a bearer API key in the authorization header
- MCP server performs basic format checks and forwards the token to the backend
- Backend enforces project scope via `api_key_projects` access list, permissions, and rate limits
- MCP server itself is stateless regarding authorization — all access control lives in the backend

## Multi-Project Support

All MCP tools require `project_id` as a parameter. The bearer key determines which projects the caller can access; the backend enforces this on every request.

For local stdio mode, an optional default project can be configured to reduce boilerplate:

```env
MCP_DEFAULT_PROJECT_ID=uuid-of-main-project
```

When set and a tool call omits `project_id`, the MCP server injects the default before forwarding to the backend. This is a convenience for single-project local use — the backend still validates access.

## Response Shape

All tool responses return structured JSON payloads. Metadata fields are included where applicable:

| Field | Description |
|---|---|
| `query_time_ms` | Backend processing time in milliseconds |
| `index_snapshot_id` | Snapshot UUID the results were read from |
| `index_freshness_commit` | Git commit hash of the active snapshot |

Oversized payloads are capped and low-rank overflow results are summarized.

## Language Support

The `search_code` tool accepts a `language` filter. Supported languages cover 28 languages across three tiers:

- **Tier 1** (13 languages): full extractor coverage (symbols, imports, exports, references, JSX usages, network calls)
- **Tier 2** (15 languages): symbol and import extraction
- **Tier 3**: fallback chunk-only parsing for unrecognized file types

The language enum is maintained in the tool schema and matches the backend parser's language registry.

## Reliability Rules

- Default per-request timeout: 10 seconds (`MCP_REQUEST_TIMEOUT_MS`)
- Retries for transient backend/network errors only
- Circuit breaker behavior for repeated failures
- Explicit actionable error messages returned to agents

## Runtime Configuration

```env
MCP_TRANSPORT=sse
MCP_HTTP_PORT=4444
MCP_DEFAULT_PROJECT_ID=
MCP_REQUEST_TIMEOUT_MS=10000
BACKEND_BASE_URL=http://backend-api:8080
```

## HTTP Endpoints

When running in SSE transport mode, the MCP server exposes:

| Method | Path | Description |
|---|---|---|
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/tools` | List all tool definitions and schemas |
| POST | `/tools/{toolName}` | Invoke a specific tool |

## Docker Notes

- Dedicated `mcp-server` container
- Internal port: 4444 (`MCP_HTTP_PORT`), published as 3000 in docker-compose
- Environment: `BACKEND_BASE_URL=http://backend-api:8080`
- Depends on `backend-api` health readiness
- Does not connect to PostgreSQL, Qdrant, Redis, or Ollama directly
- Only dependency is the backend HTTP API

## Service Boundary Rules

- No direct access to any data store (PostgreSQL, Qdrant, Redis)
- No direct access to Ollama or Git remotes
- All data flows through `backend-api`
- No parser logic — parsing is internal to the worker process

## Security Notes

- Never log full bearer tokens
- Redact sensitive values in trace logs
- Enforce max request body size to prevent abuse
- Input validation on all tool parameters before forwarding
