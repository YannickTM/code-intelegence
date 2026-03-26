package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// withChiParams injects chi URL parameters into the request context.
func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestProjectHandler_Stubs(t *testing.T) {
	// Pass nil for all deps — only testing stub methods that don't use them.
	h := NewProjectHandler(nil, nil, nil, nil, nil, nil)

	// HandleIndex and HandleListJobs are real DB-backed handlers (Task 17) —
	// covered by integration tests, not stubs.
	tests := []struct {
		name       string
		handler    func(http.ResponseWriter, *http.Request)
		method     string
		wantStatus int
		wantKey    string // optional: key that must exist in JSON response
		chiParams  map[string]string
	}{
		// HandleSearch is a real DB-backed handler (Task 35) —
		// covered by integration tests, not stubs.
		// HandleListSymbols and HandleGetSymbol are real DB-backed handlers (Task 28) —
		// covered by integration tests, not stubs.
		// HandleDependencies, HandleFileDependencies, HandleDependencyGraph are
		// real DB-backed handlers (Task 27) — covered by integration tests, not stubs.
		// HandleStructure and HandleFileContext are real DB-backed handlers (Task 22) —
		// covered by integration tests, not stubs.
		{"HandleConventions", h.HandleConventions, http.MethodGet, 200, "items", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.chiParams != nil {
				req = withChiParams(req, tt.chiParams)
			}
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantKey != "" {
				m := mustDecodeJSON(t, w.Body)
				if _, ok := m[tt.wantKey]; !ok {
					t.Errorf("response missing key %q", tt.wantKey)
				}
			}
		})
	}
}

// APIKeyHandler is no longer a stub — covered by integration tests.

// DashboardHandler is no longer a stub — covered by integration tests.

// EmbeddingHandler global methods are no longer stubs — real implementations in embedding.go.

// MembershipHandler is no longer a stub — covered by integration tests.

