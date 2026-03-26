# 10 — Server-Sent Events Infrastructure

## Status
Done

## Goal
Built the SSE connection management infrastructure with a Hub that tracks concurrent client connections, filters events by project membership, and supports both project-scoped broadcasts and user-targeted delivery. A Redis pub/sub subscriber bridge enables multi-instance event propagation for job lifecycle, snapshot activation, and membership change events.

## Depends On
- Ticket 04 (User Identity & Session Middleware)
- Ticket 06 (Project Membership)

## Scope

### SSE Hub (`internal/sse/hub.go`)

The `Hub` manages concurrent SSE client connections with a configurable maximum connection limit (`MaxSSEConnections` from `EventsConfig`).

**Core types:**

- `Hub` -- holds a map of `*Client` under `sync.RWMutex`, with `maxConns` limit and an optional `MembershipLoader` callback
- `Client` -- represents a connected user with `UserID`, `ProjectIDs` (membership set for filtering), a buffered `Ch` channel (size 16), and a `Done` channel closed on unregister
- `SSEEvent` -- Go representation of `contracts/events/sse-event.v1.schema.json` with `Event`, `ProjectID`, `JobID`, `SnapshotID`, `Timestamp`, `Data`, and `Origin` fields

**Methods:**

| Method | Description |
|--------|-------------|
| `Register(c)` | Adds client; returns error if at `maxConns` capacity |
| `Unregister(c)` | Removes client, closes `Done` channel |
| `Broadcast(projectID, data)` | Sends to clients whose project set contains `projectID`; non-blocking (drops if `Ch` full, logs warning) |
| `Publish(evt)` | Marshals `SSEEvent` to JSON, formats as SSE frame, calls `Broadcast`. Nil-safe. |
| `SendToUser(userID, data)` | Sends to all clients belonging to `userID`; non-blocking |
| `PublishToUser(userID, evt)` | Marshals and sends SSE frame to user's clients. Nil-safe. |
| `AddProjectForUser(userID, projectID)` | Updates live membership sets when a user is added to a project |
| `RemoveProjectForUser(userID, projectID)` | Updates live membership sets when a user is removed |
| `RefreshAllMemberships(ctx)` | Reloads all connected clients' project sets from the database |
| `RunPeriodicRefresh(ctx, interval)` | Background goroutine that calls `RefreshAllMemberships` on a fixed interval |

### Endpoints

**Event Stream -- `GET /v1/events/stream`** (session auth via `RequireUser`):

1. Validates `http.Flusher` support
2. Checks Hub capacity (503 if at limit)
3. Loads user's project IDs via `ListUserProjectIDs` query
4. Registers client with Hub
5. Sends `connected` event (`event: connected`)
6. Enters event loop: reads from client channel, sends keepalive pings at configured interval, exits on context cancellation
7. Unregisters client on disconnect

**Log Stream -- `GET /v1/projects/{projectID}/logs/stream`** (dual-auth: session or API key, member+):

Stub endpoint for future project activity logging. Sends `log:connected` event with `project_id` and enters a keepalive loop. No Hub integration.

### Redis Pub/Sub Subscriber Bridge

The `EventPublisher` publishes SSE events to a Redis channel. A `Subscriber` running in each API instance listens for events and calls `Hub.Broadcast` or `Hub.SendToUser` to deliver them to locally connected clients. The `Origin` field on `SSEEvent` enables deduplication: the instance that originated an event skips re-broadcasting it from the subscriber.

Event types bridged through Redis:

| Channel Pattern | Events |
|----------------|--------|
| `job:*` | `job:started`, `job:progress`, `job:completed`, `job:failed` |
| `snapshot:activated` | Snapshot activation notifications |
| `member:*` | `member:added`, `member:role_updated`, `member:removed` |

### Keepalive and Connection Management

SSE comments (`: keepalive\n\n`) are sent at the `SSEKeepaliveInterval` from config. The Hub is created at app startup, exposed on the `App` struct for downstream wiring. A `MembershipLoader` callback is installed for periodic membership refresh to correct drift from missed Redis pub/sub deltas.

## Key Files

| File | Purpose |
|------|---------|
| `backend-api/internal/sse/hub.go` | Hub, Client, Broadcast, Publish, membership management |
| `backend-api/internal/sse/publisher.go` | Redis EventPublisher for cross-instance events |
| `backend-api/internal/sse/subscriber.go` | Redis pub/sub subscriber that bridges to Hub |
| `backend-api/internal/handler/event.go` | `HandleStream` (real SSE), `HandleLogStream` (stub) |
| `datastore/postgres/queries/auth.sql` | `ListUserProjectIDs` query |

## Acceptance Criteria
- [x] `GET /v1/events/stream` requires session authentication (401 if unauthenticated)
- [x] Correct SSE headers set (Content-Type: text/event-stream, Cache-Control: no-cache, Connection: keep-alive)
- [x] `connected` event sent on connection establishment
- [x] 503 returned when `MaxSSEConnections` is reached
- [x] User's project IDs loaded at connection time for event filtering
- [x] Keepalive comments sent at configured interval
- [x] Clean disconnect on client context cancellation
- [x] Hub.Broadcast only delivers to clients with matching project membership
- [x] Hub.Broadcast does not block on slow clients (non-blocking send, drop if full)
- [x] Hub.Publish and Hub.PublishToUser are nil-safe
- [x] Live membership sets updated on member add/remove via AddProjectForUser/RemoveProjectForUser
- [x] Periodic membership refresh corrects drift from missed Redis events
- [x] Redis pub/sub subscriber bridges job, snapshot, and member events to local Hub
- [x] Origin-based dedup prevents re-broadcasting events on the originating instance
- [x] `GET /v1/projects/{id}/logs/stream` sends `log:connected` event with proper SSE headers
- [x] Hub created at app startup and exposed for downstream wiring
