package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/sse"
	"myjungle/backend-api/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// firstWriteRecorder wraps httptest.ResponseRecorder and signals a channel
// on the first Write call. This provides a race-free synchronisation point
// for tests that need to know when the handler has produced output.
type firstWriteRecorder struct {
	*httptest.ResponseRecorder
	once    sync.Once
	writeCh chan struct{}
}

func newFirstWriteRecorder() *firstWriteRecorder {
	return &firstWriteRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		writeCh:          make(chan struct{}),
	}
}

func (r *firstWriteRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseRecorder.Write(p)
	r.once.Do(func() { close(r.writeCh) })
	return n, err
}

// Flush delegates to the underlying recorder so SSE flushing works.
func (r *firstWriteRecorder) Flush() {
	r.ResponseRecorder.Flush()
}

// newTestEventHandler creates an EventHandler with a Hub and no DB.
// The handler's HandleStream will fail when it tries to query the DB.
// Use reqWithUser + cancelled context for tests that only check initial behaviour.
func newTestEventHandler(maxConns int) *EventHandler {
	return NewEventHandler(sse.NewHub(maxConns), nil, 30*time.Second)
}

// reqWithUser attaches a domain.User to the request context.
func reqWithUser(r *http.Request, u *domain.User) *http.Request {
	ctx := auth.ContextWithUser(r.Context(), u)
	return r.WithContext(ctx)
}

func TestHandleStream_NoFlusher(t *testing.T) {
	h := newTestEventHandler(10)
	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", nil)
	req = reqWithUser(req, &domain.User{ID: "u1"})
	w := &testutil.NoFlushResponseWriter{ResponseWriter: httptest.NewRecorder()}

	h.HandleStream(w, req)

	rec := w.ResponseWriter.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	m := mustDecodeJSON(t, rec.Body)
	if msg, _ := m["error"].(string); msg != "streaming unsupported" {
		t.Errorf("error = %v, want %q", m["error"], "streaming unsupported")
	}
}

func TestHandleStream_503_AtCapacity(t *testing.T) {
	h := newTestEventHandler(0) // capacity=0 → early rejection before DB query
	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", nil)
	req = reqWithUser(req, &domain.User{ID: "u1"})
	w := httptest.NewRecorder()

	h.HandleStream(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleStream_NoUser(t *testing.T) {
	h := newTestEventHandler(10)
	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", nil)
	w := httptest.NewRecorder()

	h.HandleStream(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleLogStream_NoFlusher(t *testing.T) {
	h := newTestEventHandler(10)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-123/logs/stream", nil)
	w := &testutil.NoFlushResponseWriter{ResponseWriter: httptest.NewRecorder()}

	h.HandleLogStream(w, req)

	rec := w.ResponseWriter.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleLogStream_503_AtCapacity(t *testing.T) {
	h := newTestEventHandler(0) // capacity=0 → immediate rejection
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-123/logs/stream", nil)
	req = reqWithUser(req, &domain.User{ID: "u1"})
	w := httptest.NewRecorder()

	h.HandleLogStream(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleLogStream_Headers_And_ConnectedEvent(t *testing.T) {
	h := newTestEventHandler(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-123/logs/stream", nil)
	req = req.WithContext(auth.ContextWithUser(ctx, &domain.User{ID: "u1"}))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.HandleLogStream(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-cache")
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: log:connected") {
		t.Errorf("body missing 'event: log:connected': %q", body)
	}
	if !strings.Contains(body, `"project_id":"proj-123"`) {
		t.Errorf("body missing project_id: %q", body)
	}
}

func TestHandleLogStream_ContextCancel(t *testing.T) {
	h := newTestEventHandler(10)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-123/logs/stream", nil)
	req = req.WithContext(auth.ContextWithUser(ctx, &domain.User{ID: "u1"}))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := newFirstWriteRecorder()

	done := make(chan struct{})
	go func() {
		h.HandleLogStream(w, req)
		close(done)
	}()

	select {
	case <-w.writeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not write initial event in time")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("handler did not exit after context cancellation")
	}
}
