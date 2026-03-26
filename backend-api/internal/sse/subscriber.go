package sse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	initialRetryDelay = 500 * time.Millisecond
	maxRetryDelay     = 10 * time.Second
)

// EventChannel is the Redis pub/sub channel for real-time events.
// Must match backend-worker/worker/notify/publisher.go Channel constant.
const EventChannel = "myjungle:events"

// eventEnvelope is a lightweight struct for extracting routing fields
// from incoming Redis messages. The Data field captures the raw JSON so
// that membership deltas can parse user_id without re-unmarshalling the
// full payload.
type eventEnvelope struct {
	Event     string          `json:"event"`
	ProjectID string          `json:"project_id"`
	Data      json.RawMessage `json:"data"`
	Origin    string          `json:"origin"`
}

// membershipData holds the user_id from a membership delta event's data
// field (member:added / member:removed).
type membershipData struct {
	UserID string `json:"user_id"`
}

// Subscriber reads from Redis pub/sub and broadcasts to the Hub.
type Subscriber struct {
	hub         *Hub
	rdb         *redis.Client
	channel     string
	instanceID  string         // identifies this API instance; used to skip self-originated events
	ready       chan struct{}   // closed when subscription is active
	once        sync.Once
	OnReconnect func() // called after subscriber reconnects to Redis
}

// NewSubscriber creates a Subscriber connected to the given Redis URL.
// The URL should be a standard redis:// connection string. poolSize sets
// the maximum number of connections in the underlying Redis pool.
// instanceID identifies the current API instance so the subscriber can
// skip re-broadcasting events that this instance already delivered locally.
func NewSubscriber(hub *Hub, redisURL string, poolSize int, instanceID string) (*Subscriber, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("sse subscriber: parse redis url: %w", err)
	}
	opts.PoolSize = poolSize
	return &Subscriber{
		hub:        hub,
		rdb:        redis.NewClient(opts),
		channel:    EventChannel,
		instanceID: instanceID,
		ready:      make(chan struct{}),
	}, nil
}

// Listen subscribes to the Redis events channel and broadcasts incoming
// messages to the Hub. It blocks until ctx is cancelled. Intended to be
// called in a goroutine: go subscriber.Listen(ctx).
func (s *Subscriber) Listen(ctx context.Context) {
	delay := initialRetryDelay
	connected := false

	for {
		sub := s.rdb.Subscribe(ctx, s.channel)

		// Wait for the subscription to be confirmed by Redis. Subscribe is
		// asynchronous; Receive blocks until the server acknowledges.
		if _, err := sub.Receive(ctx); err != nil {
			sub.Close()

			// If the context is done, exit cleanly.
			if ctx.Err() != nil {
				slog.Info("SSE subscriber stopping",
					slog.String("reason", "context cancelled"))
				s.once.Do(func() { close(s.ready) })
				return
			}

			slog.Error("SSE subscriber: subscription failed, retrying",
				slog.String("channel", s.channel),
				slog.Any("error", err),
				slog.Duration("backoff", delay))

			select {
			case <-ctx.Done():
				s.once.Do(func() { close(s.ready) })
				return
			case <-time.After(delay):
			}

			// Exponential backoff capped at maxRetryDelay.
			delay = delay * 2
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			continue
		}

		// Subscription confirmed — reset backoff and signal readiness.
		delay = initialRetryDelay
		s.once.Do(func() { close(s.ready) })

		if connected {
			slog.Info("SSE subscriber reconnected", slog.String("channel", s.channel))
			if s.OnReconnect != nil {
				s.OnReconnect()
			}
		} else {
			slog.Info("SSE subscriber started", slog.String("channel", s.channel))
			connected = true
		}

		s.consumeMessages(ctx, sub)
		sub.Close()

		// If the context is done, exit cleanly.
		if ctx.Err() != nil {
			return
		}

		// Channel closed unexpectedly — retry.
		slog.Warn("SSE subscriber: connection lost, reconnecting",
			slog.String("channel", s.channel))
	}
}

// consumeMessages reads from the PubSub channel and broadcasts to the Hub.
// It returns when ctx is cancelled or the channel is closed.
func (s *Subscriber) consumeMessages(ctx context.Context, sub *redis.PubSub) {
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			slog.Info("SSE subscriber stopping", slog.String("reason", "context cancelled"))
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			s.handleMessage(msg.Payload)
		}
	}
}

// handleMessage parses a Redis message payload, validates it, formats it
// as an SSE frame, and broadcasts to the Hub. Malformed or incomplete
// messages are logged and skipped. Events that originated from this
// instance (matching Origin) are skipped because the handler already
// delivered them locally and applied any membership deltas.
func (s *Subscriber) handleMessage(payload string) {
	var env eventEnvelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		slog.Warn("SSE subscriber: malformed message, skipping",
			slog.Any("error", err))
		return
	}
	if env.ProjectID == "" {
		slog.Warn("SSE subscriber: missing project_id, skipping")
		return
	}
	if env.Event == "" {
		slog.Warn("SSE subscriber: missing event type, skipping")
		return
	}

	// Skip events published by this instance — the handler already
	// delivered the notification locally and applied membership deltas.
	if s.instanceID != "" && env.Origin != "" && env.Origin == s.instanceID {
		return
	}

	// Apply membership deltas and extract target user for routing.
	// Membership events are personal notifications sent to the targeted
	// user; applyMembershipDelta also updates the Hub's live client
	// membership for future project-wide events.
	targetUserID := s.applyMembershipDelta(env)

	// Compact the JSON to ensure it fits on a single data: line.
	// Pretty-printed payloads with newlines would break SSE framing.
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(payload)); err != nil {
		slog.Warn("SSE subscriber: failed to compact JSON, skipping",
			slog.Any("error", err))
		return
	}

	// Format as SSE frame. Membership events are personal notifications
	// sent to the targeted user; all other events are project-wide broadcasts.
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", env.Event, compact.String())
	if targetUserID != "" {
		s.hub.SendToUser(targetUserID, []byte(frame))
	} else {
		s.hub.Broadcast(env.ProjectID, []byte(frame))
	}
}

// applyMembershipDelta checks whether the event is a membership change
// and updates the Hub's live client membership accordingly. For all three
// membership event types (member:added, member:removed, member:role_updated)
// it returns the targeted user's ID so the caller can route the event as a
// personal notification. Returns "" for non-membership events.
//
// It reuses the already-parsed eventEnvelope (including its raw Data field)
// to avoid re-unmarshalling the full payload.
func (s *Subscriber) applyMembershipDelta(env eventEnvelope) string {
	switch env.Event {
	case "member:added", "member:removed", "member:role_updated":
		var data membershipData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			slog.Debug("SSE subscriber: failed to parse membership data",
				slog.String("event", env.Event),
				slog.String("project_id", env.ProjectID),
				slog.Any("error", err))
			return ""
		}
		if data.UserID == "" {
			slog.Debug("SSE subscriber: membership event missing user_id",
				slog.String("event", env.Event),
				slog.String("project_id", env.ProjectID))
			return ""
		}
		// Apply Hub state changes for added/removed; role_updated needs
		// no state change (the user remains a project member).
		if env.Event == "member:added" {
			s.hub.AddProjectForUser(data.UserID, env.ProjectID)
		} else if env.Event == "member:removed" {
			s.hub.RemoveProjectForUser(data.UserID, env.ProjectID)
		}
		return data.UserID
	}
	return ""
}

// Close shuts down the underlying Redis client.
// It is nil-safe: calling Close on a nil Subscriber is a no-op.
func (s *Subscriber) Close() error {
	if s == nil || s.rdb == nil {
		return nil
	}
	return s.rdb.Close()
}
