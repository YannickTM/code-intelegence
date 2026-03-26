package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/sse"
	"myjungle/backend-api/internal/storage/postgres"

	"github.com/go-chi/chi/v5"
)

// EventHandler serves SSE event stream endpoints.
type EventHandler struct {
	hub       *sse.Hub
	db        *postgres.DB
	keepalive time.Duration
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(hub *sse.Hub, db *postgres.DB, keepalive time.Duration) *EventHandler {
	return &EventHandler{
		hub:       hub,
		db:        db,
		keepalive: keepalive,
	}
}

// HandleStream opens an SSE connection and delivers project events to the
// authenticated user (GET /v1/events/stream).
func (h *EventHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteAppError(w, domain.Errorf(domain.ErrInternal, "streaming unsupported"))
		return
	}

	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	// Fail fast if the hub is already at capacity (avoids a DB round-trip).
	if !h.hub.HasCapacity() {
		WriteError(w, http.StatusServiceUnavailable, "too many SSE connections")
		return
	}

	// Load the user's project membership set.
	userUUID, err := dbconv.StringToPgUUID(user.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}
	pgIDs, err := h.db.Queries.ListUserProjectIDs(r.Context(), userUUID)
	if err != nil {
		slog.Error("failed to load user project IDs", slog.Any("error", err))
		WriteAppError(w, domain.Errorf(domain.ErrInternal, "failed to load project memberships"))
		return
	}

	projectIDs := make(map[string]struct{}, len(pgIDs))
	for _, pid := range pgIDs {
		projectIDs[dbconv.PgUUIDToString(pid)] = struct{}{}
	}

	client := sse.NewClient(user.ID, projectIDs)

	if err := h.hub.Register(client); err != nil {
		WriteError(w, http.StatusServiceUnavailable, "too many SSE connections")
		return
	}
	defer h.hub.Unregister(client)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	connected := map[string]any{
		"status":    "connected",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if err := SendSSE(w, "connected", connected); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(h.keepalive)
	defer ticker.Stop()

	for {
		select {
		case msg := <-client.Ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// HandleLogStream opens an SSE connection for project activity logs
// (GET /v1/projects/{projectID}/logs/stream). This is a stub that only
// sends a connected event and keepalives. The connection is registered
// with the Hub (using an empty project set) so it counts toward
// MaxSSEConnections even though no events are delivered yet.
func (h *EventHandler) HandleLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteAppError(w, domain.Errorf(domain.ErrInternal, "streaming unsupported"))
		return
	}

	// Build a descriptive client ID from the authenticated principal.
	// RequireProjectRole middleware guarantees a user or API key is present.
	projectID := chi.URLParam(r, "projectID")
	var clientID string
	if u, ok := auth.UserFromContext(r.Context()); ok {
		clientID = fmt.Sprintf("log:%s:%s", u.ID, projectID)
	} else if k, ok := auth.APIKeyFromContext(r.Context()); ok {
		clientID = fmt.Sprintf("log:apikey-%s:%s", k.KeyHash[:8], projectID)
	} else {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	// Count this connection toward MaxSSEConnections.
	// Empty project set means no broadcasts will match this client.
	client := sse.NewClient(clientID, nil)
	if err := h.hub.Register(client); err != nil {
		WriteError(w, http.StatusServiceUnavailable, "too many SSE connections")
		return
	}
	defer h.hub.Unregister(client)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	connected := map[string]any{
		"status":     "connected",
		"project_id": projectID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
	if err := SendSSE(w, "log:connected", connected); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(h.keepalive)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
