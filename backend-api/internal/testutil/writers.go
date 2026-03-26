package testutil

import "net/http"

// NoFlushResponseWriter wraps an http.ResponseWriter but strips the
// http.Flusher interface. Use it to test code paths that check for Flusher
// support (e.g., SSE handlers, statusRecorder.Flush).
type NoFlushResponseWriter struct {
	http.ResponseWriter
}
