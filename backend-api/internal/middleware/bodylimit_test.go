package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimit_UnderLimit(t *testing.T) {
	var bodyRead string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodyRead = string(b)
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small"))
	BodyLimit(1024)(inner).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if bodyRead != "small" {
		t.Errorf("body = %q, want %q", bodyRead, "small")
	}
}

func TestBodyLimit_OverLimit(t *testing.T) {
	var readErr error
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	big := strings.Repeat("x", 2048)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	BodyLimit(1024)(inner).ServeHTTP(w, r)

	if readErr == nil {
		t.Error("expected read error for oversized body")
	}
}

func TestBodyLimit_NilBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	BodyLimit(1024)(inner).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
