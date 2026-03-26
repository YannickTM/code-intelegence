<details>
<summary>Relevant source files</summary>

The following files were used as context for generating this wiki page:

- [concept/05-backoffice-ui.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/05-backoffice-ui.md)
- [concept/01-system-overview.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/01-system-overview.md)
- [README.md](https://github.com/YannickTM/code-intelegence/blob/main/README.md)
</details>

# SSE Real-Time Stream Integrations

## Introduction
The SSE (Server-Sent Events) Real-Time Stream Integrations facilitate live, unidirectional communication between the backend API and the Backoffice UI. This system is primarily used to push asynchronous job events, progress updates, and project status changes directly to the browser without requiring the client to poll for updates. It ensures that the operational console remains reactive to the background activities of the `backend-worker` and other system components.

The integration utilizes the standard HTML5 `EventSource` API on the frontend, which handles automatic reconnection and provides a simple interface for consuming streams. On the backend, an SSE bridge acts as a proxy for events published via Redis Pub/Sub, translating internal system messages into formatted SSE events for the UI.
Sources: [concept/05-backoffice-ui.md]()
