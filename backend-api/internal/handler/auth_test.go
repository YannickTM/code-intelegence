package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myjungle/backend-api/internal/config"
)

var testSessionCfg = config.SessionConfig{
	TTL:          config.DefaultSessionTTL,
	CookieName:   config.DefaultSessionCookieName,
	SecureCookie: false,
}

// --- HandleLogin (validation tests — no DB needed) ---

func TestHandleLogin_MissingUsername(t *testing.T) {
	h := NewAuthHandler(nil, testSessionCfg)
	req := newJSONRequest(t, http.MethodPost, "/v1/auth/login", map[string]any{})
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleLogin_WhitespaceUsername(t *testing.T) {
	h := NewAuthHandler(nil, testSessionCfg)
	req := newJSONRequest(t, http.MethodPost, "/v1/auth/login", map[string]any{"username": "   "})
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleLogin_InvalidJSON(t *testing.T) {
	h := NewAuthHandler(nil, testSessionCfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleLogin_NilDB(t *testing.T) {
	h := NewAuthHandler(nil, testSessionCfg)
	req := newJSONRequest(t, http.MethodPost, "/v1/auth/login", map[string]any{"username": "alice"})
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- HandleLogout ---

func TestHandleLogout_NilDB(t *testing.T) {
	h := NewAuthHandler(nil, testSessionCfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
