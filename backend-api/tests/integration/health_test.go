//go:build integration

package integration_test

import (
	"net/http"
	"testing"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/config"
)

func TestHealth_LivenessAlways200(t *testing.T) {
	a := setupTestApp(t)

	w := doRequest(t, a, http.MethodGet, "/health/live", nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	m := decodeJSON(t, w)

	if m["status"] != "ok" {
		t.Errorf("status = %v, want %q", m["status"], "ok")
	}
	if m["service"] != "backend-api" {
		t.Errorf("service = %v, want %q", m["service"], "backend-api")
	}
	if _, ok := m["version"]; !ok {
		t.Error("missing version field")
	}
	if _, ok := m["timestamp"]; !ok {
		t.Error("missing timestamp field")
	}
}

func TestHealth_ReadinessUp(t *testing.T) {
	a := setupTestApp(t)

	w := doRequest(t, a, http.MethodGet, "/health/ready", nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	m := decodeJSON(t, w)

	if m["status"] != "ready" {
		t.Errorf("status = %v, want %q", m["status"], "ready")
	}

	checks, ok := m["checks"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid checks field")
	}

	pg, ok := checks["postgres"].(map[string]any)
	if !ok {
		t.Fatal("missing postgres check")
	}
	if pg["status"] != "up" {
		t.Errorf("postgres.status = %v, want %q", pg["status"], "up")
	}
	// latency_ms may be omitted when 0 (omitempty on int64).
	if latency, ok := pg["latency_ms"].(float64); ok && latency < 0 {
		t.Errorf("postgres.latency_ms = %v, want non-negative number", pg["latency_ms"])
	}
}

func TestHealth_ReadinessDown(t *testing.T) {
	// Create a separate App so we can close its DB pool without affecting
	// other tests.
	cfg := config.LoadForTest()
	cfg.Postgres.DSN = testDSN

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	// Simulate DB unavailability by closing the connection pool.
	a.DB.Pool.Close()

	// Give the pool a moment to fully close.
	w := doRequest(t, a, http.MethodGet, "/health/ready", nil, nil)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}

	m := decodeJSON(t, w)

	if m["status"] != "degraded" {
		t.Errorf("status = %v, want %q", m["status"], "degraded")
	}

	checks, ok := m["checks"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid checks field")
	}

	pg, ok := checks["postgres"].(map[string]any)
	if !ok {
		t.Fatal("missing postgres check")
	}
	if pg["status"] != "down" {
		t.Errorf("postgres.status = %v, want %q", pg["status"], "down")
	}
	if _, ok := pg["error"]; !ok {
		t.Error("postgres.error should be present when DB is down")
	}
}

func TestHealth_Metrics(t *testing.T) {
	a := setupTestApp(t)

	// Make a request first so there's at least one counter entry.
	doRequest(t, a, http.MethodGet, "/health/live", nil, nil)

	w := doRequest(t, a, http.MethodGet, "/metrics", nil, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/plain; version=0.0.4")
	}

	body := w.Body.String()
	if !contains(body, "myjungle_api_uptime_seconds") {
		t.Error("metrics body missing myjungle_api_uptime_seconds")
	}
	if !contains(body, "myjungle_api_requests_total") {
		t.Error("metrics body missing myjungle_api_requests_total")
	}
}

// contains checks if s contains substr. Avoids importing strings in test files
// that already have a large import list.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Verify the health check runs without auth (no middleware blocking).
func TestHealth_NoAuthRequired(t *testing.T) {
	a := setupTestApp(t)

	for _, path := range []string{"/health/live", "/health/ready", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			w := doRequest(t, a, http.MethodGet, path, nil, nil)
			if w.Code == http.StatusUnauthorized {
				t.Errorf("%s returned 401 — health endpoints should not require auth", path)
			}
		})
	}
}

// Verify liveness works even when DB is down (it should never check DB).
func TestHealth_LivenessIgnoresDBState(t *testing.T) {
	cfg := config.LoadForTest()
	cfg.Postgres.DSN = testDSN

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	// Close the DB pool.
	a.DB.Pool.Close()

	// Liveness should still return 200 — it never checks dependencies.
	w := doRequest(t, a, http.MethodGet, "/health/live", nil, nil)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d — liveness must not depend on DB", w.Code, http.StatusOK)
	}
}
