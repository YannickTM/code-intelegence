package middleware

import (
	"net/http"

	"myjungle/backend-api/internal/metrics"

	"github.com/go-chi/chi/v5"
)

// Metrics returns middleware that records request counts in the given collector.
// If c is nil, a passthrough middleware is returned.
func Metrics(c *metrics.Collector) func(http.Handler) http.Handler {
	if c == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Use the chi route pattern to avoid high-cardinality labels
			// from parameterized paths (e.g. /v1/projects/{id}).
			// Fall back to a constant for unmatched routes (404s from scanners).
			path := "/unmatched"
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if pattern := rctx.RoutePattern(); pattern != "" {
					path = pattern
				}
			}

			c.RecordRequest(r.Method, path, rec.statusCode)
		})
	}
}
