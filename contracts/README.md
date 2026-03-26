# Contracts

This folder contains versioned interface contracts shared across services.

## Contents

- `openapi/v1.yaml`: backend HTTP API contract (backoffice + MCP integration target)
- `events/sse-event.v1.schema.json`: SSE event envelope schema
- `queue/workflow-task.v1.schema.json`: Redis/asynq workflow queue-message schema
- `redis/worker-status.v1.schema.json`: Redis worker-registry heartbeat schema
- `mcp/tools.v1.json`: MCP phase 1 tool input/output contract

## Versioning

- Breaking changes require a new major version path/file.
- Non-breaking additions are allowed in the same major version.
- Service implementations should be validated against these files in CI.
