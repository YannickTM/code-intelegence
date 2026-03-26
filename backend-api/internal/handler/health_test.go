package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myjungle/backend-api/internal/health"
	"myjungle/backend-api/internal/metrics"
)

// fakeChecker implements health.Checker for testing.
type fakeChecker struct {
	name   string
	result health.CheckResult
}

func (f *fakeChecker) Name() string                              { return f.name }
func (f *fakeChecker) Check(_ context.Context) health.CheckResult { return f.result }

func newTestCollector() *metrics.Collector {
	return metrics.NewCollector(time.Now().UTC())
}

func TestHandleLive(t *testing.T) {
	h := NewHealthHandler(time.Now().UTC(), nil, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	h.HandleLive(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["status"] != "ok" {
		t.Errorf("status = %v, want %q", m["status"], "ok")
	}
	if m["service"] != "backend-api" {
		t.Errorf("service = %v, want %q", m["service"], "backend-api")
	}
	if m["timestamp"] == nil {
		t.Error("timestamp is missing")
	}
	if m["version"] != serviceVersion {
		t.Errorf("version = %v, want %q", m["version"], serviceVersion)
	}
}

func TestHandleReady_NoCheckers(t *testing.T) {
	h := NewHealthHandler(time.Now().UTC(), nil, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.HandleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["status"] != "ready" {
		t.Errorf("status = %v, want %q", m["status"], "ready")
	}
}

func TestHandleReady_AllUp(t *testing.T) {
	checkers := []health.Checker{
		&fakeChecker{name: "postgres", result: health.CheckResult{Status: health.StatusUp, LatencyMs: 1}},
		&fakeChecker{name: "redis", result: health.CheckResult{Status: health.StatusSkipped}},
	}
	h := NewHealthHandler(time.Now().UTC(), checkers, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.HandleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["status"] != "ready" {
		t.Errorf("status = %v, want %q", m["status"], "ready")
	}
	checks, ok := m["checks"].(map[string]any)
	if !ok {
		t.Fatal("checks is not a map")
	}
	pg, ok := checks["postgres"].(map[string]any)
	if !ok {
		t.Fatal("checks.postgres is not a map")
	}
	if pg["status"] != health.StatusUp {
		t.Errorf("postgres status = %v, want %q", pg["status"], health.StatusUp)
	}
}

func TestHandleReady_DepDown(t *testing.T) {
	checkers := []health.Checker{
		&fakeChecker{name: "postgres", result: health.CheckResult{Status: health.StatusDown, Error: "connection refused"}},
		&fakeChecker{name: "redis", result: health.CheckResult{Status: health.StatusSkipped}},
	}
	h := NewHealthHandler(time.Now().UTC(), checkers, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.HandleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["status"] != "degraded" {
		t.Errorf("status = %v, want %q", m["status"], "degraded")
	}
	checks, ok := m["checks"].(map[string]any)
	if !ok {
		t.Fatal("checks is not a map")
	}
	pg, ok := checks["postgres"].(map[string]any)
	if !ok {
		t.Fatal("checks.postgres is not a map")
	}
	if pg["status"] != health.StatusDown {
		t.Errorf("postgres status = %v, want %q", pg["status"], health.StatusDown)
	}
	if pg["error"] != "connection refused" {
		t.Errorf("postgres error = %v, want %q", pg["error"], "connection refused")
	}
}

func TestHandleReady_UnknownStatusDegrades(t *testing.T) {
	checkers := []health.Checker{
		&fakeChecker{name: "postgres", result: health.CheckResult{Status: health.StatusUp, LatencyMs: 1}},
		&fakeChecker{name: "mystery", result: health.CheckResult{Status: "oops"}},
	}
	h := NewHealthHandler(time.Now().UTC(), checkers, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.HandleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d (unknown status should degrade)", w.Code, http.StatusServiceUnavailable)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["status"] != "degraded" {
		t.Errorf("status = %v, want %q", m["status"], "degraded")
	}
	checks, ok := m["checks"].(map[string]any)
	if !ok {
		t.Fatal("checks is not a map")
	}
	mystery, ok := checks["mystery"].(map[string]any)
	if !ok {
		t.Fatal("checks.mystery is not a map")
	}
	if mystery["status"] != "oops" {
		t.Errorf("mystery status = %v, want %q", mystery["status"], "oops")
	}
}

func TestHandleReady_SkippedDoesNotFail(t *testing.T) {
	checkers := []health.Checker{
		&fakeChecker{name: "postgres", result: health.CheckResult{Status: health.StatusSkipped}},
		&fakeChecker{name: "redis", result: health.CheckResult{Status: health.StatusSkipped}},
	}
	h := NewHealthHandler(time.Now().UTC(), checkers, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.HandleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (skipped should not fail)", w.Code, http.StatusOK)
	}
}

func TestHandleMetrics_NilCollector(t *testing.T) {
	h := NewHealthHandler(time.Time{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	h.HandleMetrics(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want Prometheus content type", ct)
	}
}

func TestHandleMetrics_ContentType(t *testing.T) {
	h := NewHealthHandler(time.Now().UTC(), nil, newTestCollector())
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	h.HandleMetrics(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/plain; version=0.0.4")
	}
}

func TestHandleMetrics_Format(t *testing.T) {
	// Handler startedAt is unused by HandleMetrics; only the collector's matters.
	h := NewHealthHandler(time.Time{}, nil, metrics.NewCollector(time.Now().Add(-5*time.Second)))
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	h.HandleMetrics(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "# HELP myjungle_api_uptime_seconds") {
		t.Error("missing HELP line")
	}
	if !strings.Contains(body, "# TYPE myjungle_api_uptime_seconds gauge") {
		t.Error("missing TYPE line")
	}
	if !strings.Contains(body, "myjungle_api_uptime_seconds ") {
		t.Error("missing metric line")
	}
}
