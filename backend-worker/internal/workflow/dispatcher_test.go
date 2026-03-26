package workflow

import (
	"context"
	"testing"
)

type recordingHandler struct {
	called bool
	task   WorkflowTask
}

func (h *recordingHandler) Handle(_ context.Context, task WorkflowTask) error {
	h.called = true
	h.task = task
	return nil
}

func TestDispatcher_RegisterAndRetrieve(t *testing.T) {
	d := NewDispatcher()
	h1 := &recordingHandler{}
	h2 := &recordingHandler{}

	d.Register("full-index", h1)
	d.Register("incremental-index", h2)

	handlers := d.Handlers()
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(handlers))
	}
	if _, ok := handlers["full-index"]; !ok {
		t.Error("missing handler for full-index")
	}
	if _, ok := handlers["incremental-index"]; !ok {
		t.Error("missing handler for incremental-index")
	}
}

func TestDispatcher_HandlersReturnsCopy(t *testing.T) {
	d := NewDispatcher()
	d.Register("full-index", &recordingHandler{})

	handlers := d.Handlers()
	handlers["extra"] = &recordingHandler{}

	// Original dispatcher should not be affected.
	if len(d.Handlers()) != 1 {
		t.Error("Handlers() did not return a copy")
	}
}

func TestHandler_ReceivesCorrectTask(t *testing.T) {
	h := &recordingHandler{}

	task := WorkflowTask{
		JobID:    "job-1",
		Workflow: "full-index",
	}
	if err := h.Handle(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.called {
		t.Error("handler was not called")
	}
	if h.task.JobID != "job-1" {
		t.Errorf("job_id = %q, want %q", h.task.JobID, "job-1")
	}
}

func TestDispatcher_OverwriteHandler(t *testing.T) {
	d := NewDispatcher()
	h1 := &recordingHandler{}
	h2 := &recordingHandler{}

	d.Register("full-index", h1)
	d.Register("full-index", h2)

	handlers := d.Handlers()
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler after overwrite, got %d", len(handlers))
	}
}
