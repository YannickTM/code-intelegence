package handler

import (
	"net/http"
	"time"

	"myjungle/backend-api/internal/health"
	"myjungle/backend-api/internal/metrics"
)

// serviceVersion is the reported API version.
// Override at build time: -ldflags "-X myjungle/backend-api/internal/handler.serviceVersion=1.2.3"
var serviceVersion = "0.1.0"

// HealthHandler serves health check and metrics endpoints.
type HealthHandler struct {
	startedAt time.Time
	checkers  []health.Checker
	collector *metrics.Collector
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(startedAt time.Time, checkers []health.Checker, collector *metrics.Collector) *HealthHandler {
	return &HealthHandler{
		startedAt: startedAt,
		checkers:  checkers,
		collector: collector,
	}
}

// HandleLive returns liveness status. Always 200 if the process is running.
func (h *HealthHandler) HandleLive(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   "backend-api",
		"version":   serviceVersion,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReady returns readiness status. Returns 200 if all critical
// dependencies are reachable, 503 if any required dependency is down.
func (h *HealthHandler) HandleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]health.CheckResult, len(h.checkers))
	allOK := true

	for _, c := range h.checkers {
		result := c.Check(r.Context())
		checks[c.Name()] = result
		if result.Status != health.StatusUp && result.Status != health.StatusSkipped {
			allOK = false
		}
	}

	status := http.StatusOK
	overall := "ready"
	if !allOK {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}

	WriteJSON(w, status, map[string]any{
		"status":    overall,
		"service":   "backend-api",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"checks":    checks,
	})
}

// HandleMetrics returns Prometheus-format metrics.
func (h *HealthHandler) HandleMetrics(w http.ResponseWriter, _ *http.Request) {
	if h.collector == nil {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("# error: metrics collector not configured\n"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(h.collector.Render()))
}
