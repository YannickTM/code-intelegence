package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myjungle/backend-api/internal/metrics"
	"myjungle/backend-api/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func TestMetrics_RecordsRequest(t *testing.T) {
	c := metrics.NewCollector(time.Now())

	r := chi.NewRouter()
	r.Use(middleware.Metrics(c))
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	out := c.Render()
	if !strings.Contains(out, `myjungle_api_requests_total{method="GET",path="/health/live",status="200"} 1`) {
		t.Errorf("collector did not record request:\n%s", out)
	}
}

func TestMetrics_CapturesStatusCode(t *testing.T) {
	c := metrics.NewCollector(time.Now())

	r := chi.NewRouter()
	r.Use(middleware.Metrics(c))
	r.Get("/items", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	out := c.Render()
	if !strings.Contains(out, `status="404"`) {
		t.Errorf("collector did not capture 404 status:\n%s", out)
	}
}

func TestMetrics_UsesRoutePattern(t *testing.T) {
	c := metrics.NewCollector(time.Now())

	r := chi.NewRouter()
	r.Use(middleware.Metrics(c))
	r.Get("/v1/projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/abc-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	out := c.Render()
	if !strings.Contains(out, `path="/v1/projects/{id}"`) {
		t.Errorf("expected route pattern label, got:\n%s", out)
	}
	if strings.Contains(out, `path="/v1/projects/abc-123"`) {
		t.Error("should not contain concrete path with ID")
	}
}

func TestMetrics_UnmatchedRouteFallback(t *testing.T) {
	c := metrics.NewCollector(time.Now())

	r := chi.NewRouter()
	r.Use(middleware.Metrics(c))
	r.Get("/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request a path that doesn't match any route.
	req := httptest.NewRequest(http.MethodGet, "/random/scanner/path", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	out := c.Render()
	if strings.Contains(out, `path="/random/scanner/path"`) {
		t.Error("should not contain raw unmatched path")
	}
	if !strings.Contains(out, `path="/unmatched"`) {
		t.Errorf("expected /unmatched fallback path in output:\n%s", out)
	}
}

func TestMetrics_NilCollector(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := middleware.Metrics(nil)(inner)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
