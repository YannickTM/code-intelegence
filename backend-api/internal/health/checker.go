// Package health defines a pluggable health check interface for dependency probes.
package health

import "context"

// Status constants for health check results.
const (
	StatusUp      = "up"
	StatusDown    = "down"
	StatusSkipped = "skipped"
)

// Checker is a named health check for a single dependency.
type Checker interface {
	Name() string
	Check(ctx context.Context) CheckResult
}

// CheckResult describes the outcome of a single health check.
type CheckResult struct {
	Status    string `json:"status"` // StatusUp, StatusDown, or StatusSkipped
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}
