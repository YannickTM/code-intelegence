package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/config"
)

// newTestApp creates an App with explicit CORS origins for testing.
func newTestApp(t *testing.T) *app.App {
	t.Helper()
	cfg := config.LoadForTest()
	cfg.Server.CORSAllowedOrigins = []string{"http://test.local"}
	cfg.Server.CORSWildcard = false
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	return a
}

// doRequest executes a request against the app router and returns the recorder.
func doRequest(t *testing.T, a *app.App, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, r)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	return m
}

func TestNew_ReturnsValidApp(t *testing.T) {
	a := newTestApp(t)
	if a == nil {
		t.Fatal("New() returned nil")
	}
	if a.Router == nil {
		t.Error("Router is nil")
	}
	if a.Config == nil {
		t.Error("Config is nil")
	}
}

func TestRoutes_HealthEndpoints(t *testing.T) {
	a := newTestApp(t)

	tests := []struct {
		path string
	}{
		{"/health/live"},
		{"/health/ready"},
		{"/metrics"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := doRequest(t, a, http.MethodGet, tt.path, "")
			if w.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want %d", tt.path, w.Code, http.StatusOK)
			}
		})
	}
}

func TestRoutes_PublicEndpoints(t *testing.T) {
	a := newTestApp(t)

	// POST /v1/users is public (no auth required).
	// Verify the route is reachable without authentication — the exact
	// status depends on nil-DB internals, so we only assert it's NOT 401.
	t.Run("POST /v1/users (public)", func(t *testing.T) {
		w := doRequest(t, a, http.MethodPost, "/v1/users", `{"username":"testuser"}`)
		if w.Code == http.StatusUnauthorized {
			t.Errorf("status = %d; public route should not require auth", w.Code)
		}
		if w.Code == http.StatusNotFound {
			t.Errorf("status = %d; route not registered", w.Code)
		}
		if w.Code == http.StatusMethodNotAllowed {
			t.Errorf("status = %d; method not wired for route", w.Code)
		}
	})

	// POST /v1/auth/login is public.
	t.Run("POST /v1/auth/login (public)", func(t *testing.T) {
		w := doRequest(t, a, http.MethodPost, "/v1/auth/login", `{"username":"testuser"}`)
		if w.Code == http.StatusUnauthorized {
			t.Errorf("status = %d; public route should not require auth", w.Code)
		}
		if w.Code == http.StatusNotFound {
			t.Errorf("status = %d; route not registered", w.Code)
		}
		if w.Code == http.StatusMethodNotAllowed {
			t.Errorf("status = %d; method not wired for route", w.Code)
		}
	})
}

func TestRoutes_AuthProtected_Anonymous(t *testing.T) {
	a := newTestApp(t)

	// All authenticated routes should return 401 when no user is in context.
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"POST /v1/auth/logout", http.MethodPost, "/v1/auth/logout"},
		{"GET /v1/users/me", http.MethodGet, "/v1/users/me"},
		{"PATCH /v1/users/me", http.MethodPatch, "/v1/users/me"},
		{"GET /v1/users/me/projects", http.MethodGet, "/v1/users/me/projects"},
		{"GET /v1/projects", http.MethodGet, "/v1/projects"},
		{"POST /v1/projects", http.MethodPost, "/v1/projects"},
		{"GET /v1/ssh-keys", http.MethodGet, "/v1/ssh-keys"},
		{"POST /v1/ssh-keys", http.MethodPost, "/v1/ssh-keys"},
		{"GET /v1/ssh-keys/{keyID}", http.MethodGet, "/v1/ssh-keys/k1"},
		{"GET /v1/ssh-keys/{keyID}/projects", http.MethodGet, "/v1/ssh-keys/k1/projects"},
		{"POST /v1/ssh-keys/{keyID}/retire", http.MethodPost, "/v1/ssh-keys/k1/retire"},
		{"GET /v1/users/me/keys", http.MethodGet, "/v1/users/me/keys"},
		{"POST /v1/users/me/keys", http.MethodPost, "/v1/users/me/keys"},
		{"DELETE /v1/users/me/keys/{keyID}", http.MethodDelete, "/v1/users/me/keys/k1"},
		{"GET /v1/platform-management/settings/embedding", http.MethodGet, "/v1/platform-management/settings/embedding"},
		{"PUT /v1/platform-management/settings/embedding", http.MethodPut, "/v1/platform-management/settings/embedding"},
		{"POST /v1/platform-management/settings/embedding/test", http.MethodPost, "/v1/platform-management/settings/embedding/test"},
		{"GET /v1/platform-management/settings/llm", http.MethodGet, "/v1/platform-management/settings/llm"},
		{"PUT /v1/platform-management/settings/llm", http.MethodPut, "/v1/platform-management/settings/llm"},
		{"POST /v1/platform-management/settings/llm/test", http.MethodPost, "/v1/platform-management/settings/llm/test"},
		{"GET /v1/dashboard/summary", http.MethodGet, "/v1/dashboard/summary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, a, tt.method, tt.path, "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, w.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRoutes_ProjectScoped_Anonymous(t *testing.T) {
	a := newTestApp(t)
	id := "00000000-0000-0000-0000-000000000001"

	// Project-scoped routes are behind RequireUser, so anonymous → 401.
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"get", http.MethodGet, "/v1/projects/" + id},
		{"update", http.MethodPatch, "/v1/projects/" + id},
		{"delete", http.MethodDelete, "/v1/projects/" + id},
		{"ssh-key-get", http.MethodGet, "/v1/projects/" + id + "/ssh-key"},
		{"ssh-key-set", http.MethodPut, "/v1/projects/" + id + "/ssh-key"},
		{"index", http.MethodPost, "/v1/projects/" + id + "/index"},
		{"jobs", http.MethodGet, "/v1/projects/" + id + "/jobs"},
		{"search", http.MethodPost, "/v1/projects/" + id + "/query/search"},
		{"symbols", http.MethodGet, "/v1/projects/" + id + "/symbols"},
		{"symbol", http.MethodGet, "/v1/projects/" + id + "/symbols/s1"},
		{"deps", http.MethodGet, "/v1/projects/" + id + "/dependencies"},
		{"structure", http.MethodGet, "/v1/projects/" + id + "/structure"},
		{"files", http.MethodGet, "/v1/projects/" + id + "/files/context"},
		{"conventions", http.MethodGet, "/v1/projects/" + id + "/conventions"},
		{"members", http.MethodGet, "/v1/projects/" + id + "/members"},
		{"member-update", http.MethodPatch, "/v1/projects/" + id + "/members/u1"},
		{"member-remove", http.MethodDelete, "/v1/projects/" + id + "/members/u1"},
		{"keys-list", http.MethodGet, "/v1/projects/" + id + "/keys"},
		{"keys-create", http.MethodPost, "/v1/projects/" + id + "/keys"},
		{"keys-delete", http.MethodDelete, "/v1/projects/" + id + "/keys/k1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, a, tt.method, tt.path, "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, w.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRoutes_EventStream_Anonymous(t *testing.T) {
	a := newTestApp(t)

	// EventStream is behind RequireUser, so anonymous → 401.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := httptest.NewRequest(http.MethodGet, "/v1/events/stream", nil)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRoutes_NotFound(t *testing.T) {
	a := newTestApp(t)
	w := doRequest(t, a, http.MethodGet, "/nonexistent", "")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	m := decodeJSON(t, w)
	if m["error"] != "not found" {
		t.Errorf("error = %v, want %q", m["error"], "not found")
	}
}

func TestRoutes_MethodNotAllowed(t *testing.T) {
	a := newTestApp(t)
	w := doRequest(t, a, http.MethodDelete, "/health/live", "")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	allow := w.Header().Get("Allow")
	if !strings.Contains(allow, "GET") {
		t.Errorf("Allow = %q, does not contain GET", allow)
	}
	m := decodeJSON(t, w)
	if m["error"] != "method not allowed" {
		t.Errorf("error = %v, want %q", m["error"], "method not allowed")
	}
}

func TestRoutes_MiddlewareRequestID(t *testing.T) {
	a := newTestApp(t)
	w := doRequest(t, a, http.MethodGet, "/health/live", "")

	rid := w.Header().Get("X-Request-ID")
	if rid == "" {
		t.Error("X-Request-ID header is empty")
	}
}

func TestRoutes_MiddlewareCORS(t *testing.T) {
	a := newTestApp(t)

	r := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	r.Header.Set("Origin", "http://test.local")
	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, r)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "http://test.local" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, "http://test.local")
	}
}

func TestRoutes_MiddlewareCORS_Wildcard(t *testing.T) {
	cfg := config.LoadForTest()
	cfg.Server.CORSWildcard = true
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	r.Header.Set("Origin", "http://any-origin.example.com")
	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, r)

	acao := w.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, "*")
	}
}
