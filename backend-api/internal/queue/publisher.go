// Package queue provides a Redis/asynq publisher for dispatching workflow tasks.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// JobEnqueuer abstracts queue publishing so handlers can be tested with fakes.
type JobEnqueuer interface {
	EnqueueIndexJob(ctx context.Context, jobID, workflow, projectID, requestedBy string) error
	Close() error
}

// taskPayload matches contracts/queue/workflow-task.v1.schema.json.
type taskPayload struct {
	JobID       string `json:"job_id"`
	Workflow    string `json:"workflow"`
	EnqueuedAt string `json:"enqueued_at"`
	ProjectID   string `json:"project_id,omitempty"`
	RequestedBy string `json:"requested_by,omitempty"`
}

// PublisherConfig holds per-workflow asynq task options.
type PublisherConfig struct {
	IndexFullTimeout        time.Duration
	IndexIncrementalTimeout time.Duration
	MaxRetries              int
}

// Publisher wraps an asynq.Client for enqueuing workflow tasks.
type Publisher struct {
	client *asynq.Client
	cfg    PublisherConfig
}

// NewPublisher creates a Publisher connected to the given Redis URL.
// The URL should be a standard redis:// connection string.
func NewPublisher(redisURL string, cfg PublisherConfig) (*Publisher, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue: parse redis url: %w", err)
	}
	return &Publisher{client: asynq.NewClient(opt), cfg: cfg}, nil
}

// EnqueueIndexJob enqueues an indexing workflow task.
// The workflow value (e.g. "full-index") is used as the asynq task type.
// Timeout and retry limits are set per-workflow so asynq enforces them
// instead of relying on defaults (which caused retry storms for failed jobs).
// It is nil-safe: calling EnqueueIndexJob on a nil Publisher or one with a nil client is a no-op.
func (p *Publisher) EnqueueIndexJob(ctx context.Context, jobID, workflow, projectID, requestedBy string) error {
	if p == nil || p.client == nil {
		return nil
	}
	payload := taskPayload{
		JobID:       jobID,
		Workflow:    workflow,
		EnqueuedAt:  time.Now().UTC().Format(time.RFC3339),
		ProjectID:   projectID,
		RequestedBy: requestedBy,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("queue: marshal payload: %w", err)
	}

	// Select timeout based on workflow type.
	timeout := p.cfg.IndexFullTimeout
	if workflow == "incremental-index" {
		timeout = p.cfg.IndexIncrementalTimeout
	}

	task := asynq.NewTask(workflow, data)
	opts := []asynq.Option{
		asynq.MaxRetry(p.cfg.MaxRetries),
		asynq.Timeout(timeout),
	}
	if _, err := p.client.EnqueueContext(ctx, task, opts...); err != nil {
		return fmt.Errorf("queue: enqueue %s: %w", workflow, err)
	}
	return nil
}

// Close shuts down the underlying asynq client.
// It is nil-safe: calling Close on a nil Publisher or one with a nil client is a no-op.
func (p *Publisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}

// MapJobTypeToWorkflow converts a job_type value to the corresponding workflow name.
// It returns an error for unrecognised job types to prevent unroutable tasks.
func MapJobTypeToWorkflow(jobType string) (string, error) {
	switch jobType {
	case "full":
		return "full-index", nil
	case "incremental":
		return "incremental-index", nil
	default:
		return "", fmt.Errorf("queue: unknown job type: %q", jobType)
	}
}
