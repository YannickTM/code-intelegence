package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myjungle/backend-api/internal/testutil"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestLogging_PassesThrough(t *testing.T) {
	// Suppress log output; restore previous default on exit.
	prev := slog.Default()
	slog.SetDefault(discardLogger())
	defer slog.SetDefault(prev)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	Logging(inner).ServeHTTP(w, r)

	if w.Body.String() != "hello" {
		t.Errorf("body = %q, want %q", w.Body.String(), "hello")
	}
}

func TestLogging_CapturesStatus(t *testing.T) {
	prev := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	Logging(inner).ServeHTTP(w, r)

	logLine := buf.String()
	if !strings.Contains(logLine, "status=404") {
		t.Errorf("log line %q does not contain %q", logLine, "status=404")
	}
	if !strings.Contains(logLine, "method=GET") {
		t.Errorf("log line %q does not contain %q", logLine, "method=GET")
	}
	if !strings.Contains(logLine, "path=/missing") {
		t.Errorf("log line %q does not contain %q", logLine, "path=/missing")
	}
}

func TestLogging_IncludesErrorCode(t *testing.T) {
	// This test runs serially to avoid races on global log output.
	prev := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Error-Code", "not_found")
		w.WriteHeader(http.StatusNotFound)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	Logging(inner).ServeHTTP(w, r)

	logLine := buf.String()
	if !strings.Contains(logLine, "error_code=not_found") {
		t.Errorf("log line %q does not contain %q", logLine, "error_code=not_found")
	}
}

func TestLogging_NoErrorCode(t *testing.T) {
	// This test runs serially to avoid races on global log output.
	prev := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ok", nil)
	Logging(inner).ServeHTTP(w, r)

	logLine := buf.String()
	if strings.Contains(logLine, "error_code=") {
		t.Errorf("log line %q should not contain error_code= for success", logLine)
	}
}

func TestStatusRecorder_DefaultStatus(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	if rec.statusCode != http.StatusOK {
		t.Errorf("default statusCode = %d, want %d", rec.statusCode, http.StatusOK)
	}
}

func TestStatusRecorder_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	rec.WriteHeader(http.StatusTeapot)
	if rec.statusCode != http.StatusTeapot {
		t.Errorf("statusCode = %d, want %d", rec.statusCode, http.StatusTeapot)
	}
}

func TestStatusRecorder_Flush(t *testing.T) {
	t.Run("with_flusher", func(t *testing.T) {
		w := httptest.NewRecorder() // implements http.Flusher
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		// Should not panic.
		rec.Flush()
	})

	t.Run("without_flusher", func(t *testing.T) {
		// NoFlushResponseWriter does not implement http.Flusher.
		rec := &statusRecorder{ResponseWriter: &testutil.NoFlushResponseWriter{ResponseWriter: httptest.NewRecorder()}, statusCode: http.StatusOK}
		// Should not panic.
		rec.Flush()
	})
}
