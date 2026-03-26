// Package registry publishes ephemeral worker status in Redis.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyPrefix         = "worker:status:"
	heartbeatInterval = 10 * time.Second
	keyTTL            = 30 * time.Second
)

// Status constants matching contracts/redis/worker-status.v1.schema.json.
const (
	StatusStarting = "starting"
	StatusIdle     = "idle"
	StatusBusy     = "busy"
	StatusDraining = "draining"
)

// statusPayload matches contracts/redis/worker-status.v1.schema.json.
type statusPayload struct {
	WorkerID           string   `json:"worker_id"`
	Status             string   `json:"status"`
	StartedAt          string   `json:"started_at"`
	LastHeartbeatAt    string   `json:"last_heartbeat_at"`
	Hostname           string   `json:"hostname,omitempty"`
	SupportedWorkflows []string `json:"supported_workflows"`
	CurrentJobID       string   `json:"current_job_id,omitempty"`
	CurrentProjectID   string   `json:"current_project_id,omitempty"`
}

// Registry manages ephemeral worker status in Redis.
type Registry struct {
	client    *redis.Client
	workerID  string
	hostname  string
	startedAt time.Time
	workflows []string

	mu             sync.Mutex
	status         string
	currentJobID   string
	currentProjID  string

	stopCh   chan struct{}
	done     chan struct{}
	doneOnce sync.Once
	started  bool
}

// New creates a Registry connected to the given Redis URL.
// workerID should be resolved from WORKER_ID env var or os.Hostname() by the caller.
func New(redisURL, workerID string, workflows []string) (*Registry, error) {
	if workerID == "" {
		return nil, fmt.Errorf("registry: empty workerID")
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("registry: parse redis url: %w", err)
	}

	hostname, _ := os.Hostname()

	return &Registry{
		client:    redis.NewClient(opt),
		workerID:  workerID,
		hostname:  hostname,
		startedAt: time.Now().UTC(),
		workflows: workflows,
		status:    StatusStarting,
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}, nil
}

// SetStatus updates the worker status (thread-safe).
func (r *Registry) SetStatus(status string) {
	r.mu.Lock()
	r.status = status
	r.currentJobID = ""
	r.currentProjID = ""
	r.mu.Unlock()
}

// SetBusy marks the worker as busy with the given job context.
func (r *Registry) SetBusy(jobID, projectID string) {
	r.mu.Lock()
	r.status = StatusBusy
	r.currentJobID = jobID
	r.currentProjID = projectID
	r.mu.Unlock()
}

// SetIdle marks the worker as idle and clears current job fields.
func (r *Registry) SetIdle() {
	r.mu.Lock()
	r.status = StatusIdle
	r.currentJobID = ""
	r.currentProjID = ""
	r.mu.Unlock()
}

// StartHeartbeat starts the background heartbeat goroutine.
// It publishes the worker status to Redis every heartbeatInterval.
// The goroutine stops when ctx is cancelled or Close() is called.
func (r *Registry) StartHeartbeat(ctx context.Context) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()
	go func() {
		defer r.doneOnce.Do(func() { close(r.done) })
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		// Publish immediately on start.
		r.publish(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.publish(ctx)
			}
		}
	}()
}

// Close marks the worker as draining, publishes a final heartbeat,
// stops the heartbeat goroutine, and closes the Redis client.
func (r *Registry) Close() {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.status = StatusDraining
	r.currentJobID = ""
	r.currentProjID = ""
	r.mu.Unlock()

	// Publish final draining status.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r.publish(ctx)

	// Stop heartbeat goroutine.
	select {
	case <-r.stopCh:
		// Already closed.
	default:
		close(r.stopCh)
	}

	r.mu.Lock()
	started := r.started
	r.mu.Unlock()

	if started {
		// Wait for goroutine to finish.
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			slog.Warn("registry: heartbeat goroutine did not stop in time")
		}
	} else {
		// StartHeartbeat was never called; close done ourselves.
		r.doneOnce.Do(func() { close(r.done) })
	}

	if r.client != nil {
		if err := r.client.Close(); err != nil {
			slog.Warn("registry: failed to close redis client", slog.Any("error", err))
		}
	}
}

// publish writes the current status to Redis with TTL.
func (r *Registry) publish(ctx context.Context) {
	if r.client == nil {
		return
	}
	r.mu.Lock()
	payload := statusPayload{
		WorkerID:           r.workerID,
		Status:             r.status,
		StartedAt:          r.startedAt.Format(time.RFC3339),
		LastHeartbeatAt:    time.Now().UTC().Format(time.RFC3339),
		Hostname:           r.hostname,
		SupportedWorkflows: r.workflows,
		CurrentJobID:       r.currentJobID,
		CurrentProjectID:   r.currentProjID,
	}
	r.mu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("registry: failed to marshal status", slog.Any("error", err))
		return
	}

	key := keyPrefix + r.workerID
	if err := r.client.Set(ctx, key, data, keyTTL).Err(); err != nil {
		slog.Warn("registry: failed to publish status",
			slog.String("key", key),
			slog.Any("error", err))
	}
}

// WorkerID returns the configured worker identifier.
func (r *Registry) WorkerID() string {
	return r.workerID
}
