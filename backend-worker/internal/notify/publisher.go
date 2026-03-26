// Package notify publishes job lifecycle events to a Redis pub/sub channel.
// The API's SSE subscriber consumes these events and broadcasts them to
// connected clients.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Channel is the Redis pub/sub channel for SSE events.
// Must match backend-api/internal/sse/subscriber.go EventChannel.
const Channel = "myjungle:events"

// SSEEvent matches contracts/events/sse-event.v1.schema.json.
type SSEEvent struct {
	Event      string         `json:"event"`
	ProjectID  string         `json:"project_id"`
	JobID      string         `json:"job_id,omitempty"`
	SnapshotID string         `json:"snapshot_id,omitempty"`
	Timestamp  string         `json:"timestamp"`
	Data       map[string]any `json:"data,omitempty"`
}

// EventPublisher publishes SSE events to the Redis pub/sub channel.
// Nil-safe: calling any method on a nil *EventPublisher is a no-op.
type EventPublisher struct {
	rdb *redis.Client
}

// NewEventPublisher creates an EventPublisher connected to the given Redis URL.
func NewEventPublisher(redisURL string) (*EventPublisher, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("notify: parse redis url: %w", err)
	}
	return &EventPublisher{rdb: redis.NewClient(opts)}, nil
}

// Publish marshals the event to JSON and publishes it to the Redis channel.
// Nil-safe: calling Publish on a nil EventPublisher returns nil.
func (p *EventPublisher) Publish(ctx context.Context, evt SSEEvent) error {
	if p == nil || p.rdb == nil {
		return nil
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("notify: marshal event: %w", err)
	}
	if err := p.rdb.Publish(ctx, Channel, data).Err(); err != nil {
		return fmt.Errorf("notify: publish: %w", err)
	}
	return nil
}

// Close shuts down the underlying Redis client.
// Nil-safe: calling Close on a nil EventPublisher returns nil.
func (p *EventPublisher) Close() error {
	if p == nil || p.rdb == nil {
		return nil
	}
	return p.rdb.Close()
}

// PublishJobStarted publishes a job:started event.
func (p *EventPublisher) PublishJobStarted(ctx context.Context, projectID, jobID, jobType string) {
	err := p.Publish(ctx, SSEEvent{
		Event:     "job:started",
		ProjectID: projectID,
		JobID:     jobID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"status":   "running",
			"job_type": jobType,
		},
	})
	if err != nil {
		slog.Warn("failed to publish job:started event",
			slog.String("job_id", jobID), slog.Any("error", err))
	}
}

// PublishJobProgress publishes a job:progress event.
//
// Callers are responsible for throttling progress events (at most once per
// 2 seconds) to avoid flooding Redis and SSE clients.
func (p *EventPublisher) PublishJobProgress(ctx context.Context, projectID, jobID, jobType string, filesProcessed, filesTotal, chunksUpserted int) {
	err := p.Publish(ctx, SSEEvent{
		Event:     "job:progress",
		ProjectID: projectID,
		JobID:     jobID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"status":           "running",
			"job_type":         jobType,
			"files_processed":  filesProcessed,
			"files_total":      filesTotal,
			"chunks_upserted":  chunksUpserted,
		},
	})
	if err != nil {
		slog.Warn("failed to publish job:progress event",
			slog.String("job_id", jobID), slog.Any("error", err))
	}
}

// PublishJobCompleted publishes a job:completed event.
func (p *EventPublisher) PublishJobCompleted(ctx context.Context, projectID, jobID, jobType string, filesProcessed, chunksUpserted, vectorsDeleted int) {
	err := p.Publish(ctx, SSEEvent{
		Event:     "job:completed",
		ProjectID: projectID,
		JobID:     jobID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"status":          "completed",
			"job_type":        jobType,
			"files_processed": filesProcessed,
			"chunks_upserted": chunksUpserted,
			"vectors_deleted": vectorsDeleted,
		},
	})
	if err != nil {
		slog.Warn("failed to publish job:completed event",
			slog.String("job_id", jobID), slog.Any("error", err))
	}
}

// PublishJobFailed publishes a job:failed event.
func (p *EventPublisher) PublishJobFailed(ctx context.Context, projectID, jobID, jobType, errorMessage string) {
	err := p.Publish(ctx, SSEEvent{
		Event:     "job:failed",
		ProjectID: projectID,
		JobID:     jobID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"status":        "failed",
			"job_type":      jobType,
			"error_message": errorMessage,
		},
	})
	if err != nil {
		slog.Warn("failed to publish job:failed event",
			slog.String("job_id", jobID), slog.Any("error", err))
	}
}

// PublishSnapshotActivated publishes a snapshot:activated event.
func (p *EventPublisher) PublishSnapshotActivated(ctx context.Context, projectID, snapshotID, activeCommit string) {
	err := p.Publish(ctx, SSEEvent{
		Event:      "snapshot:activated",
		ProjectID:  projectID,
		SnapshotID: snapshotID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"active_commit": activeCommit,
		},
	})
	if err != nil {
		slog.Warn("failed to publish snapshot:activated event",
			slog.String("snapshot_id", snapshotID), slog.Any("error", err))
	}
}
