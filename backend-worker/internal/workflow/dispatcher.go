// Package workflow defines the handler contract and dispatcher for queue tasks.
package workflow

import "context"

// WorkflowTask is the decoded queue envelope.
// It matches contracts/queue/workflow-task.v1.schema.json.
type WorkflowTask struct {
	JobID       string `json:"job_id"`
	Workflow    string `json:"workflow"`
	EnqueuedAt  string `json:"enqueued_at"`
	ProjectID   string `json:"project_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
	RequestedBy string `json:"requested_by,omitempty"`
	WorkerID    string `json:"-"` // set by consumer, not serialized
}

// Handler processes a decoded workflow task.
type Handler interface {
	Handle(ctx context.Context, task WorkflowTask) error
}

// Dispatcher holds registered workflow handlers.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher creates an empty Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]Handler)}
}

// Register adds a handler for the given workflow name.
func (d *Dispatcher) Register(workflow string, h Handler) {
	d.handlers[workflow] = h
}

// Handlers returns a copy of the registered handlers.
func (d *Dispatcher) Handlers() map[string]Handler {
	out := make(map[string]Handler, len(d.handlers))
	for k, v := range d.handlers {
		out[k] = v
	}
	return out
}
