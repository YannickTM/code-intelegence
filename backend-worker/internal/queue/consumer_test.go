package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"myjungle/backend-worker/internal/workflow"
)

func TestDecodeWorkflowTask_AllFields(t *testing.T) {
	payload := workflow.WorkflowTask{
		JobID:       "69dc1de1-5ad3-4ea4-b0f0-f4ad9d1f5ff4",
		Workflow:    "full-index",
		EnqueuedAt:  "2026-03-08T11:00:00Z",
		ProjectID:   "a8f3d8f6-0e37-45e0-8826-1ce6f57f2244",
		TraceID:     "req_01JNX6H09YY4Q6B4A0A4K9R9GT",
		RequestedBy: "user:8fdd1e4f-cf42-4d6d-83a4-e4a6c7080b57",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := DecodeWorkflowTask(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != payload.JobID {
		t.Errorf("job_id = %q, want %q", got.JobID, payload.JobID)
	}
	if got.Workflow != payload.Workflow {
		t.Errorf("workflow = %q, want %q", got.Workflow, payload.Workflow)
	}
	if got.EnqueuedAt != payload.EnqueuedAt {
		t.Errorf("enqueued_at = %q, want %q", got.EnqueuedAt, payload.EnqueuedAt)
	}
	if got.ProjectID != payload.ProjectID {
		t.Errorf("project_id = %q, want %q", got.ProjectID, payload.ProjectID)
	}
	if got.TraceID != payload.TraceID {
		t.Errorf("trace_id = %q, want %q", got.TraceID, payload.TraceID)
	}
	if got.RequestedBy != payload.RequestedBy {
		t.Errorf("requested_by = %q, want %q", got.RequestedBy, payload.RequestedBy)
	}
}

func TestDecodeWorkflowTask_RequiredFieldsOnly(t *testing.T) {
	data := []byte(`{"job_id":"abc-123","workflow":"incremental-index","enqueued_at":"2026-03-08T11:00:00Z"}`)
	got, err := DecodeWorkflowTask(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProjectID != "" {
		t.Errorf("project_id should be empty, got %q", got.ProjectID)
	}
	if got.TraceID != "" {
		t.Errorf("trace_id should be empty, got %q", got.TraceID)
	}
	if got.RequestedBy != "" {
		t.Errorf("requested_by should be empty, got %q", got.RequestedBy)
	}
}

func TestDecodeWorkflowTask_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "missing job_id",
			payload: `{"workflow":"full-index","enqueued_at":"2026-03-08T11:00:00Z"}`,
			wantErr: "missing required field job_id",
		},
		{
			name:    "missing workflow",
			payload: `{"job_id":"abc-123","enqueued_at":"2026-03-08T11:00:00Z"}`,
			wantErr: "missing required field workflow",
		},
		{
			name:    "missing enqueued_at",
			payload: `{"job_id":"abc-123","workflow":"full-index"}`,
			wantErr: "missing required field enqueued_at",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeWorkflowTask([]byte(tc.payload))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !strings.Contains(got, tc.wantErr) {
				t.Errorf("error = %q, want substring %q", got, tc.wantErr)
			}
		})
	}
}

func TestDecodeWorkflowTask_MalformedJSON(t *testing.T) {
	_, err := DecodeWorkflowTask([]byte(`{not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if got := err.Error(); !strings.Contains(got, "decode payload") {
		t.Errorf("error = %q, want substring %q", got, "decode payload")
	}
}

func TestDecodeWorkflowTask_EmptyPayload(t *testing.T) {
	_, err := DecodeWorkflowTask(nil)
	if err == nil {
		t.Fatal("expected error for nil payload")
	}
	if got := err.Error(); !strings.Contains(got, "empty payload") {
		t.Errorf("error = %q, want substring %q", got, "empty payload")
	}

	_, err = DecodeWorkflowTask([]byte{})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}
