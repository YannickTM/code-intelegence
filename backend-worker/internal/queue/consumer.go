package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"myjungle/backend-worker/internal/logger"
	"myjungle/backend-worker/internal/registry"
	"myjungle/backend-worker/internal/workflow"
)

// DecodeWorkflowTask unmarshals and validates a queue payload.
// It returns an error if the payload is invalid or required fields are missing.
func DecodeWorkflowTask(payload []byte) (*workflow.WorkflowTask, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("consumer: empty payload")
	}

	var task workflow.WorkflowTask
	if err := json.Unmarshal(payload, &task); err != nil {
		return nil, fmt.Errorf("consumer: decode payload: %w", err)
	}

	if task.JobID == "" {
		return nil, fmt.Errorf("consumer: missing required field job_id")
	}
	if task.Workflow == "" {
		return nil, fmt.Errorf("consumer: missing required field workflow")
	}
	if task.EnqueuedAt == "" {
		return nil, fmt.Errorf("consumer: missing required field enqueued_at")
	}

	return &task, nil
}

// BuildServeMux creates an asynq.ServeMux with a handler for each registered
// workflow. The workflow name is used as the mux pattern key, matching the
// task type used by the API publisher.
func BuildServeMux(handlers map[string]workflow.Handler, reg *registry.Registry) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	for name, h := range handlers {
		mux.HandleFunc(name, wrapHandler(name, h, reg))
	}
	return mux
}

// wrapHandler returns an asynq handler function that decodes the task payload,
// validates required fields, logs context, manages registry status, and
// delegates to the workflow handler.
func wrapHandler(name string, h workflow.Handler, reg *registry.Registry) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		task, err := DecodeWorkflowTask(t.Payload())
		if err != nil {
			slog.Error("failed to decode task payload",
				slog.String("workflow", name),
				slog.Any("error", err))
			return fmt.Errorf("consumer: %s: %w", name, err)
		}

		if task.Workflow != name {
			slog.Error("workflow mismatch between route and payload",
				slog.String("route", name),
				slog.String("payload_workflow", task.Workflow))
			return fmt.Errorf("consumer: %s: workflow mismatch: payload=%s", name, task.Workflow)
		}

		// Build per-job logger and inject into context so all downstream
		// components (pipeline, embedding, parser, workspace) inherit it.
		retryCount, _ := asynq.GetRetryCount(ctx)
		log := slog.With(
			slog.String("job_id", task.JobID),
			slog.String("workflow", task.Workflow),
		)
		ctx = logger.WithLogger(ctx, log)

		log.Info("processing task",
			slog.String("project_id", task.ProjectID),
			slog.String("trace_id", task.TraceID),
			slog.Int("retry_count", retryCount))

		if reg != nil {
			reg.SetBusy(task.JobID, task.ProjectID)
			defer reg.SetIdle()
			task.WorkerID = reg.WorkerID()
		}

		if err := h.Handle(ctx, *task); err != nil {
			log.Error("task handler failed",
				slog.Any("error", err))
			return err
		}

		log.Info("task completed")
		return nil
	}
}
