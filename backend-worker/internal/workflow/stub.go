package workflow

import (
	"context"
	"log/slog"
)

// StubHandler is a placeholder handler that logs the task and returns nil.
// It will be replaced by real workflow implementations in later tasks.
type StubHandler struct {
	Name string
}

// Handle logs the received task for debugging purposes.
func (h *StubHandler) Handle(_ context.Context, task WorkflowTask) error {
	slog.Info("stub handler invoked",
		slog.String("handler", h.Name),
		slog.String("job_id", task.JobID),
		slog.String("workflow", task.Workflow),
		slog.String("project_id", task.ProjectID))
	return nil
}
