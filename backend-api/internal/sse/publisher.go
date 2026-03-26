package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// EventPublisher publishes SSE events to the Redis channel so that all
// API instances (including remote ones) receive membership deltas and
// broadcast them to their local SSE clients.
//
// Nil-safe: calling Publish on a nil EventPublisher is a no-op.
type EventPublisher struct {
	rdb     *redis.Client
	channel string
}

// NewEventPublisher creates an EventPublisher connected to the given Redis URL.
// poolSize sets the maximum number of connections in the underlying Redis pool.
func NewEventPublisher(redisURL string, poolSize int) (*EventPublisher, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("sse event publisher: parse redis url: %w", err)
	}
	opts.PoolSize = poolSize
	return &EventPublisher{
		rdb:     redis.NewClient(opts),
		channel: EventChannel,
	}, nil
}

// Publish marshals an SSEEvent to JSON and publishes it to the Redis events
// channel. Remote Subscriber instances will receive the message, apply any
// membership deltas, and broadcast to their local clients.
// Nil-safe: calling Publish on a nil EventPublisher is a no-op.
func (p *EventPublisher) Publish(ctx context.Context, evt SSEEvent) {
	if p == nil {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		slog.Warn("SSE event publisher: marshal failed", slog.Any("error", err))
		return
	}
	if err := p.rdb.Publish(ctx, p.channel, data).Err(); err != nil {
		slog.Warn("SSE event publisher: publish failed",
			slog.String("channel", p.channel),
			slog.Any("error", err))
	}
}

// Close shuts down the underlying Redis client.
func (p *EventPublisher) Close() error {
	if p == nil || p.rdb == nil {
		return nil
	}
	return p.rdb.Close()
}
