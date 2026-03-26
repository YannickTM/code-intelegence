# MCP Server (TypeScript)

## Role

The MCP server is the agent-facing compatibility layer.
It exposes stable MCP tools and delegates business logic to the backend API.

## Why Separate Service

- MCP transport concerns stay isolated from backend domain logic
- Stdio and SSE support can evolve independently
- Tool schema/versioning can be managed without backend redeploy coupling

## Transport Modes

- `stdio` for local desktop/CLI integrations
- `sse` (HTTP) for remote shared integration

Both transports expose the same tool set and response contracts.

## Phase 1 Tool Set

- `search_code`
- `get_symbol_info`
- `get_dependencies`
- `get_file_context`
- `get_project_structure`
- `get_conventions`

## Tool Mapping

| MCP Tool | Backend Endpoint |
| --- | --- |
| `search_code` | `POST /v1/projects/{id}/query/search` |
| `get_symbol_info` | `GET /v1/projects/{id}/symbols/{symbolId}` |
| `get_dependencies` | `GET /v1/projects/{id}/dependencies` |
| `get_file_context` | `GET /v1/projects/{id}/files/context` |
| `get_project_structure` | `GET /v1/projects/{id}/structure` |
| `get_conventions` | `GET /v1/projects/{id}/conventions` |

## Authentication Model

- MCP caller sends bearer token
- MCP server performs basic format checks and forwards token
- Backend enforces project access using `api_key_projects`
- MCP server remains stateless for authorization decisions

## Request and Response Rules

### Request

- `project_id` is required for all tools (default may be injected in local mode)
- Inputs are validated with runtime schema checks
- Enforce max input size to protect backend

### Response

- Return structured JSON payloads
- Include metadata fields where applicable:
  - `query_time_ms`
  - `index_snapshot_id`
  - `index_freshness_commit`
- Cap oversized payloads and summarize overflow results

## Reliability Rules

- Default request timeout budget: 10s
- Retry only transient errors
- Circuit breaker behavior for repeated backend failures
- Return actionable error messages to agents

## Runtime Configuration

```env
MCP_TRANSPORT=sse
MCP_HTTP_PORT=3000
MCP_DEFAULT_PROJECT_ID=
MCP_REQUEST_TIMEOUT_MS=10000
BACKEND_BASE_URL=http://backend-api:8080
BACKEND_RETRY_ATTEMPTS=2
```

## Service Boundary Rules

- No direct access to PostgreSQL, Qdrant, Redis, or Ollama
- Only dependency is backend HTTP API

## Security Notes

- Never log full bearer tokens
- Redact sensitive values in trace logs
- Apply request body size limits
