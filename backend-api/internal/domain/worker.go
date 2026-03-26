package domain

// WorkerStatus represents an active worker's ephemeral heartbeat data,
// read from Redis keys matching worker:status:{workerID}.
// Mirrors contracts/redis/worker-status.v1.schema.json.
type WorkerStatus struct {
	WorkerID           string   `json:"worker_id"`
	Status             string   `json:"status"`
	StartedAt          string   `json:"started_at"`
	LastHeartbeatAt    string   `json:"last_heartbeat_at"`
	SupportedWorkflows []string `json:"supported_workflows"`
	Hostname           string   `json:"hostname,omitempty"`
	Version            string   `json:"version,omitempty"`
	CurrentJobID       string   `json:"current_job_id,omitempty"`
	CurrentProjectID   string   `json:"current_project_id,omitempty"`
	DrainReason        string   `json:"drain_reason,omitempty"`
}
