package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
// It also implements http.Flusher for SSE compatibility.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Logging logs each request with structured fields: request_id, method, path,
// status, duration, and (optionally) error_code. The log level is based on
// the response status code: Error for 5xx, Warn for 4xx, Info otherwise.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)

		reqID := GetRequestID(r.Context())
		duration := time.Since(start)

		attrs := []slog.Attr{
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.statusCode),
			slog.Duration("duration", duration),
		}
		if errCode := rec.Header().Get("X-Error-Code"); errCode != "" {
			attrs = append(attrs, slog.String("error_code", errCode))
		}

		level := slog.LevelInfo
		if rec.statusCode >= 500 {
			level = slog.LevelError
		} else if rec.statusCode >= 400 {
			level = slog.LevelWarn
		}

		slog.LogAttrs(r.Context(), level, "http request", attrs...)
	})
}
