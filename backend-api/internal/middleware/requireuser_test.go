package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/domain"
)

func TestRequireUser_WithUser(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	u := &domain.User{ID: "user-1", Username: "alice"}
	handler := RequireUser()(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.ContextWithUser(req.Context(), u)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !called {
		t.Error("next handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireUser_NoUser(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	})

	handler := RequireUser()(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if called {
		t.Error("next handler should not have been called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if body["code"] != "unauthorized" {
		t.Errorf("code = %v, want %q", body["code"], "unauthorized")
	}
}

func TestRequireUser_ErrorHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})
	handler := RequireUser()(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	errCode := w.Header().Get("X-Error-Code")
	if errCode != "unauthorized" {
		t.Errorf("X-Error-Code = %q, want %q", errCode, "unauthorized")
	}
}
