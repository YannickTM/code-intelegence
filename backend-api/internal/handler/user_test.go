package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/domain"
)

// --- HandleRegister (validation tests — no DB needed) ---

func TestHandleRegister_MissingUsername(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_WhitespaceUsername(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{"username": "   "})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_InvalidJSON(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRegister_BodyTooLarge(t *testing.T) {
	h := NewUserHandler(nil, nil)
	bigBody := strings.NewReader(`{"username":"` + strings.Repeat("a", (1<<20)+1) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/users", bigBody)
	w := httptest.NewRecorder()

	req.Body = http.MaxBytesReader(w, req.Body, 1<<20)
	h.HandleRegister(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleRegister_MissingEmail(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{"username": "alice"})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_InvalidEmail(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{"username": "alice", "email": "not-an-email"})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_MultipleAtEmail(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{"username": "alice", "email": "a@b@c.com"})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_WhitespaceEmail(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{"username": "alice", "email": "alice @example.com"})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_NilDB(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := newJSONRequest(t, http.MethodPost, "/v1/users", map[string]any{"username": "alice", "email": "alice@example.com"})
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- HandleGetMe ---

func TestHandleGetMe_NoUser(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	w := httptest.NewRecorder()

	h.HandleGetMe(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetMe_WithUser(t *testing.T) {
	h := NewUserHandler(nil, nil)
	u := &domain.User{
		ID:          "00000000-0000-0000-0000-000000000001",
		Username:    "alice",
		DisplayName: "Alice",
		IsActive:    true,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	ctx := auth.ContextWithUser(req.Context(), u)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.HandleGetMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	m := mustDecodeJSON(t, w.Body)
	user, ok := m["user"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'user' object")
	}
	if user["username"] != "alice" {
		t.Errorf("username = %v, want %q", user["username"], "alice")
	}
	if user["id"] != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("id = %v, want %q", user["id"], "00000000-0000-0000-0000-000000000001")
	}
	if user["display_name"] != "Alice" {
		t.Errorf("display_name = %v, want %q", user["display_name"], "Alice")
	}
	if user["is_active"] != true {
		t.Errorf("is_active = %v, want true", user["is_active"])
	}
}

// --- HandleUpdateMe ---

func TestHandleUpdateMe_NoUser(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.HandleUpdateMe(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// --- HandleMyProjects ---

func TestHandleMyProjects_NoUser(t *testing.T) {
	h := NewUserHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me/projects", nil)
	w := httptest.NewRecorder()
	h.HandleMyProjects(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// --- normalizeUsername ---

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Alice", "alice"},
		{"  bob  ", "bob"},
		{"  CHARLIE ", "charlie"},
		{"", ""},
		{"   ", ""},
		{"MiXeD", "mixed"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			if got := normalizeUsername(tt.input); got != tt.want {
				t.Errorf("normalizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
