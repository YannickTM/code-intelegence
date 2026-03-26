// Package sse provides the SSE connection hub for managing concurrent client
// connections and broadcasting events to matching project members.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const clientBufferSize = 16

// MembershipLoader loads the current set of project IDs for a user from the
// database. Used by RefreshAllMemberships to reconcile in-memory state.
type MembershipLoader func(ctx context.Context, userID string) (map[string]struct{}, error)

// Hub manages SSE client connections and broadcasts events.
type Hub struct {
	mu               sync.RWMutex
	clients          map[*Client]struct{}
	maxConns         int
	membershipLoader MembershipLoader
}

// Client represents a connected SSE client.
type Client struct {
	UserID     string
	ProjectIDs map[string]struct{} // membership set for filtering
	Ch         chan []byte          // buffered outgoing event channel
	Done       chan struct{}        // closed on unregister
}

// NewClient creates a Client with the given user ID and project membership set.
func NewClient(userID string, projectIDs map[string]struct{}) *Client {
	return &Client{
		UserID:     userID,
		ProjectIDs: projectIDs,
		Ch:         make(chan []byte, clientBufferSize),
		Done:       make(chan struct{}),
	}
}

// SSEEvent is the Go representation of contracts/events/sse-event.v1.schema.json.
// Used by Hub.Publish for API-originated events.
type SSEEvent struct {
	Event      string         `json:"event"`
	ProjectID  string         `json:"project_id"`
	JobID      string         `json:"job_id,omitempty"`
	SnapshotID string         `json:"snapshot_id,omitempty"`
	Timestamp  string         `json:"timestamp"`
	Data       map[string]any `json:"data,omitempty"`
	Origin     string         `json:"origin,omitempty"`
}

// NewHub creates a new Hub with the given maximum connection limit.
func NewHub(maxConns int) *Hub {
	return &Hub{
		clients:  make(map[*Client]struct{}),
		maxConns: maxConns,
	}
}

// Register adds a client to the Hub. It returns an error if the Hub is at capacity.
func (h *Hub) Register(c *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.clients) >= h.maxConns {
		return fmt.Errorf("max SSE connections reached (%d)", h.maxConns)
	}
	h.clients[c] = struct{}{}
	return nil
}

// Unregister removes a client from the Hub and closes its done channel.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Done)
	}
}

// Broadcast sends data to all clients whose project set contains projectID.
// The send is non-blocking: if a client's channel is full, the message is
// dropped and a warning is logged.
func (h *Hub) Broadcast(projectID string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if _, ok := c.ProjectIDs[projectID]; !ok {
			continue
		}
		select {
		case c.Ch <- data:
		default:
			slog.Warn("SSE message dropped (buffer full)",
				slog.String("user_id", c.UserID),
				slog.String("project_id", projectID))
		}
	}
}

// Publish marshals an SSEEvent to JSON, formats it as an SSE frame, and
// broadcasts it to matching clients. It is nil-safe: calling Publish on a
// nil Hub is a no-op.
func (h *Hub) Publish(evt SSEEvent) {
	if h == nil {
		return
	}

	data, err := json.Marshal(evt)
	if err != nil {
		slog.Warn("failed to marshal SSE event", slog.Any("error", err))
		return
	}
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Event, data)
	h.Broadcast(evt.ProjectID, []byte(frame))
}

// HasCapacity reports whether the Hub can accept at least one more client.
// Use this for early rejection before doing expensive work (e.g. DB queries).
// Note: a subsequent Register may still fail due to races, so callers must
// still handle the Register error.
func (h *Hub) HasCapacity() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients) < h.maxConns
}

// AddProjectForUser adds projectID to the ProjectIDs set of every connected
// client belonging to userID. This keeps live SSE clients in sync when a user
// is added to a project at runtime. Nil-safe.
func (h *Hub) AddProjectForUser(userID, projectID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.UserID == userID {
			if c.ProjectIDs == nil {
				c.ProjectIDs = make(map[string]struct{})
			}
			c.ProjectIDs[projectID] = struct{}{}
		}
	}
}

// RemoveProjectForUser removes projectID from the ProjectIDs set of every
// connected client belonging to userID. This keeps live SSE clients in sync
// when a user is removed from a project at runtime. Nil-safe.
func (h *Hub) RemoveProjectForUser(userID, projectID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.UserID == userID {
			delete(c.ProjectIDs, projectID)
		}
	}
}

// SendToUser sends data to all connected clients belonging to userID.
// The send is non-blocking: if a client's channel is full, the message is
// dropped and a warning is logged. Nil-safe.
func (h *Hub) SendToUser(userID string, data []byte) {
	if h == nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c.UserID != userID {
			continue
		}
		select {
		case c.Ch <- data:
		default:
			slog.Warn("SSE message dropped (buffer full)",
				slog.String("user_id", c.UserID))
		}
	}
}

// PublishToUser marshals an SSEEvent to JSON, formats it as an SSE frame,
// and sends it to all connected clients belonging to userID. It is nil-safe:
// calling PublishToUser on a nil Hub is a no-op.
func (h *Hub) PublishToUser(userID string, evt SSEEvent) {
	if h == nil {
		return
	}

	data, err := json.Marshal(evt)
	if err != nil {
		slog.Warn("failed to marshal SSE event", slog.Any("error", err))
		return
	}
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Event, data)
	h.SendToUser(userID, []byte(frame))
}

// ClientCount returns the number of currently connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// SetMembershipLoader installs a callback that loads a user's current project
// memberships from the database. Required by RefreshAllMemberships.
//
// Must be called before RunPeriodicRefresh is started; RunPeriodicRefresh
// checks the loader once at launch and exits immediately if none is set.
// In production the ordering is guaranteed: App.New sets the loader while
// App.Run starts the periodic goroutine.
func (h *Hub) SetMembershipLoader(fn MembershipLoader) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.membershipLoader = fn
}

// RefreshAllMemberships reloads every connected client's ProjectIDs set from
// the database via the installed MembershipLoader. Clients whose loader call
// fails are skipped (logged). This corrects drift caused by missed Redis
// pub/sub membership deltas (e.g. after a subscriber reconnect).
func (h *Hub) RefreshAllMemberships(ctx context.Context) {
	if h == nil {
		return
	}

	// Collect unique user IDs and snapshot the loader under read lock.
	h.mu.RLock()
	loader := h.membershipLoader
	userIDs := make(map[string]struct{})
	for c := range h.clients {
		userIDs[c.UserID] = struct{}{}
	}
	h.mu.RUnlock()

	if loader == nil {
		return
	}

	if len(userIDs) == 0 {
		return
	}

	// Load memberships from DB (outside lock).
	memberships := make(map[string]map[string]struct{}, len(userIDs))
	for uid := range userIDs {
		pids, err := loader(ctx, uid)
		if err != nil {
			slog.Warn("SSE membership refresh failed",
				slog.String("user_id", uid),
				slog.Any("error", err))
			continue
		}
		memberships[uid] = pids
	}

	// Apply under write lock.
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if pids, ok := memberships[c.UserID]; ok {
			c.ProjectIDs = pids
		}
	}

	slog.Debug("SSE membership refresh completed",
		slog.Int("users", len(memberships)))
}

// RunPeriodicRefresh calls RefreshAllMemberships on a fixed interval until
// ctx is cancelled. Intended to be called in a goroutine.
//
// It checks membershipLoader once at startup and returns immediately if none
// is installed. Callers must ensure SetMembershipLoader is called first
// (App.New installs the loader; App.Run starts this goroutine).
func (h *Hub) RunPeriodicRefresh(ctx context.Context, interval time.Duration) {
	if h == nil {
		return
	}
	h.mu.RLock()
	hasLoader := h.membershipLoader != nil
	h.mu.RUnlock()
	if !hasLoader {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.RefreshAllMemberships(ctx)
		}
	}
}
