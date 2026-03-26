package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORS_Wildcard(t *testing.T) {
	handler := NewCORS(nil, true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://anything.com")
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want %q", got, "*")
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	allowed := []string{"http://example.com"}
	handler := NewCORS(allowed, false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Errorf("ACAO = %q, want %q", got, "http://example.com")
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

func TestCORS_AllowedOrigin_Credentials(t *testing.T) {
	allowed := []string{"http://example.com"}
	handler := NewCORS(allowed, false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
	}
}

func TestCORS_Wildcard_NoCredentials(t *testing.T) {
	handler := NewCORS(nil, true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://anything.com")
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want empty (wildcard mode)", got)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	allowed := []string{"http://example.com"}
	handler := NewCORS(allowed, false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://evil.com")
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty", got)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	allowed := []string{"http://example.com"}
	handler := NewCORS(allowed, false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})
	handler := NewCORS(nil, true)(inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if called {
		t.Error("inner handler should not be called on OPTIONS preflight")
	}
}

func TestCORS_PreflightMaxAge(t *testing.T) {
	handler := NewCORS(nil, true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	handler.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("Max-Age = %q, want %q", got, "86400")
	}
}

func TestCORS_ExposeHeaders(t *testing.T) {
	handler := NewCORS(nil, true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	got := w.Header().Get("Access-Control-Expose-Headers")
	if !containsWord(got, "X-Request-ID") {
		t.Errorf("Expose-Headers = %q, want X-Request-ID", got)
	}
}

func TestCORS_AllowMethods(t *testing.T) {
	handler := NewCORS(nil, true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	got := w.Header().Get("Access-Control-Allow-Methods")
	if got == "" {
		t.Error("Access-Control-Allow-Methods is empty")
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if !containsWord(got, method) {
			t.Errorf("Allow-Methods %q does not contain %q", got, method)
		}
	}
}

func TestCORS_AllowHeaders(t *testing.T) {
	handler := NewCORS(nil, true)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	got := w.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Authorization", "Content-Type", "X-Request-ID"} {
		if !containsWord(got, h) {
			t.Errorf("Allow-Headers %q does not contain %q", got, h)
		}
	}
}

// containsWord checks if s contains word as a comma-separated token.
func containsWord(s, word string) bool {
	for _, tok := range strings.Split(s, ",") {
		if strings.TrimSpace(tok) == word {
			return true
		}
	}
	return false
}
