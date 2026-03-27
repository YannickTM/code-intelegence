package registry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSetStatus(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusStarting,
		activeJobs: make(map[string]string),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}

	r.SetStatus(StatusIdle)
	r.mu.Lock()
	if r.status != StatusIdle {
		t.Errorf("status = %q, want %q", r.status, StatusIdle)
	}
	r.mu.Unlock()
}

func TestSetBusy(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusIdle,
		activeJobs: make(map[string]string),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}

	r.SetBusy("job-1", "proj-1")
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != StatusBusy {
		t.Errorf("status = %q, want %q", r.status, StatusBusy)
	}
	if r.activeJobs["job-1"] != "proj-1" {
		t.Errorf("activeJobs[job-1] = %q, want %q", r.activeJobs["job-1"], "proj-1")
	}
}

func TestClearJob_SingleJob(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusBusy,
		activeJobs: map[string]string{"job-1": "proj-1"},
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}

	r.ClearJob("job-1")
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != StatusIdle {
		t.Errorf("status = %q, want %q (no active jobs)", r.status, StatusIdle)
	}
	if len(r.activeJobs) != 0 {
		t.Errorf("activeJobs should be empty, got %v", r.activeJobs)
	}
}

func TestClearJob_ConcurrentJobs(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusBusy,
		activeJobs: map[string]string{"job-1": "proj-1", "job-2": "proj-2"},
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}

	// Clear one of two active jobs — should stay busy.
	r.ClearJob("job-1")
	r.mu.Lock()

	if r.status != StatusBusy {
		t.Errorf("status = %q, want %q (still has active job)", r.status, StatusBusy)
	}
	if _, ok := r.activeJobs["job-1"]; ok {
		t.Error("job-1 should have been removed from activeJobs")
	}
	if r.activeJobs["job-2"] != "proj-2" {
		t.Error("job-2 should still be in activeJobs")
	}
	r.mu.Unlock()

	// Clear the last job — should become idle.
	r.ClearJob("job-2")
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != StatusIdle {
		t.Errorf("status = %q, want %q (no active jobs)", r.status, StatusIdle)
	}
	if len(r.activeJobs) != 0 {
		t.Errorf("activeJobs should be empty, got %v", r.activeJobs)
	}
}

func TestClearJob_PreservesDrainingStatus(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusDraining,
		activeJobs: map[string]string{"job-1": "proj-1"},
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}

	// ClearJob during draining should NOT revert to idle.
	r.ClearJob("job-1")
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != StatusDraining {
		t.Errorf("status = %q, want %q (draining must not revert to idle)", r.status, StatusDraining)
	}
	if len(r.activeJobs) != 0 {
		t.Errorf("activeJobs should be empty, got %v", r.activeJobs)
	}
}

func TestStatusPayload_Marshal(t *testing.T) {
	p := statusPayload{
		WorkerID:           "test-worker",
		Status:             StatusBusy,
		StartedAt:          "2026-03-08T12:00:00Z",
		LastHeartbeatAt:    "2026-03-08T12:05:10Z",
		Hostname:           "test-host",
		SupportedWorkflows: []string{"full-index", "incremental-index"},
		CurrentJobID:       "job-1",
		CurrentProjectID:   "proj-1",
		ActiveJobs:         map[string]string{"job-1": "proj-1", "job-2": "proj-2"},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	for _, field := range []string{
		`"worker_id":"test-worker"`,
		`"status":"busy"`,
		`"started_at":"2026-03-08T12:00:00Z"`,
		`"last_heartbeat_at":"2026-03-08T12:05:10Z"`,
		`"hostname":"test-host"`,
		`"supported_workflows":["full-index","incremental-index"]`,
		`"current_job_id":"job-1"`,
		`"current_project_id":"proj-1"`,
		`"active_jobs":{`,
	} {
		if !strings.Contains(s, field) {
			t.Errorf("JSON missing field: %s\ngot: %s", field, s)
		}
	}
}

func TestStatusPayload_OmitsEmptyOptionals(t *testing.T) {
	p := statusPayload{
		WorkerID:           "test-worker",
		Status:             StatusIdle,
		StartedAt:          "2026-03-08T12:00:00Z",
		LastHeartbeatAt:    "2026-03-08T12:05:10Z",
		SupportedWorkflows: []string{"full-index"},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "current_job_id") {
		t.Error("JSON should not contain current_job_id when empty")
	}
	if strings.Contains(s, "current_project_id") {
		t.Error("JSON should not contain current_project_id when empty")
	}
	if strings.Contains(s, "hostname") {
		t.Error("JSON should not contain hostname when empty")
	}
	if strings.Contains(s, "active_jobs") {
		t.Error("JSON should not contain active_jobs when empty")
	}
}

func TestClose_NilSafe(t *testing.T) {
	var r *Registry
	r.Close() // Should not panic.
}

func TestClose_Idempotent(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusIdle,
		activeJobs: make(map[string]string),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}
	// StartHeartbeat was never called, so Close() must not block.
	r.Close()
	r.Close() // Second call should not panic.
}

func TestClose_WithoutStartHeartbeat(t *testing.T) {
	r := &Registry{
		workerID:   "test-worker",
		workflows:  []string{"full-index"},
		status:     StatusIdle,
		activeJobs: make(map[string]string),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}
	// Close should complete without blocking when StartHeartbeat was never called.
	r.Close()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status != StatusDraining {
		t.Errorf("status = %q, want %q", r.status, StatusDraining)
	}
}
