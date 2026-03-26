package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_GeneratesUUID(t *testing.T) {
	var ctxID string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxID = GetRequestID(r.Context())
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	RequestID(inner).ServeHTTP(w, r)

	got := w.Header().Get("X-Request-ID")
	if got == "" {
		t.Fatal("expected X-Request-ID in response, got empty")
	}
	if !validRequestID.MatchString(got) {
		t.Errorf("generated ID %q is not a valid UUID", got)
	}
	if ctxID != got {
		t.Errorf("context ID %q != response header %q", ctxID, got)
	}
}

func TestRequestID_AcceptsValid(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440000"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", validUUID)

	RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)

	if got := w.Header().Get("X-Request-ID"); got != validUUID {
		t.Errorf("X-Request-ID = %q, want %q", got, validUUID)
	}
}

func TestRequestID_RejectsInvalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"short", "abc"},
		{"truncated", "550e8400-e29b-41d4-a716"},
		{"invalid_hex", "550e8400-e29b-41d4-a716-44665544000g"},
		{"no_dashes", "550e8400e29b41d4a716446655440000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.value != "" {
				r.Header.Set("X-Request-ID", tt.value)
			}

			RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)

			got := w.Header().Get("X-Request-ID")
			if got == tt.value {
				t.Errorf("expected a new UUID, but got the same invalid value %q", got)
			}
			if !validRequestID.MatchString(got) {
				t.Errorf("replacement ID %q is not a valid UUID", got)
			}
		})
	}
}

func TestRequestID_SetsContext(t *testing.T) {
	var ctxID string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxID = GetRequestID(r.Context())
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	RequestID(inner).ServeHTTP(w, r)

	headerID := w.Header().Get("X-Request-ID")
	if ctxID != headerID {
		t.Errorf("context ID %q != header ID %q", ctxID, headerID)
	}
}

func TestGetRequestID_Empty(t *testing.T) {
	if got := GetRequestID(context.Background()); got != "" {
		t.Errorf("GetRequestID(background) = %q, want empty", got)
	}
}
