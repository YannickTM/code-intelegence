package handler

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myjungle/backend-api/internal/domain"
)

func TestWriteJSON_StatusAndContentType(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]any{"ok": true})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteJSON_EncodesPayload(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]any{"key": "val"})

	m := mustDecodeJSON(t, w.Body)
	if m["key"] != "val" {
		t.Errorf("key = %v, want %q", m["key"], "val")
	}
}

func TestWriteJSON_NilPayload(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, nil)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "null" {
		t.Errorf("body = %q, want %q", body, "null")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["error"] != "bad input" {
		t.Errorf("error = %v, want %q", m["error"], "bad input")
	}
}

func TestWriteErrorWithCode(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorWithCode(w, http.StatusUnprocessableEntity, "validation_error", "field invalid")

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "validation_error" {
		t.Errorf("code = %v, want %q", m["code"], "validation_error")
	}
	if m["error"] != "field invalid" {
		t.Errorf("error = %v, want %q", m["error"], "field invalid")
	}
}

func TestWriteAppError_WithAppError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteAppError(w, domain.BadRequest("bad field"))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "bad_request" {
		t.Errorf("code = %v, want %q", m["code"], "bad_request")
	}
	if m["error"] != "bad field" {
		t.Errorf("error = %v, want %q", m["error"], "bad field")
	}
}

func TestWriteAppError_WithWrappedAppError(t *testing.T) {
	w := httptest.NewRecorder()
	wrapped := fmt.Errorf("outer: %w", domain.ErrNotFound)
	WriteAppError(w, wrapped)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "not_found" {
		t.Errorf("code = %v, want %q", m["code"], "not_found")
	}
}

func TestWriteAppError_WithGenericError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteAppError(w, errors.New("something unexpected"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "internal_error" {
		t.Errorf("code = %v, want %q", m["code"], "internal_error")
	}
	if m["error"] != "internal server error" {
		t.Errorf("error = %v, want %q", m["error"], "internal server error")
	}
}

func TestWriteAppError_WithDetails(t *testing.T) {
	w := httptest.NewRecorder()
	WriteAppError(w, domain.ValidationError(map[string]string{"name": "required"}))

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "validation_error" {
		t.Errorf("code = %v, want %q", m["code"], "validation_error")
	}
	details, ok := m["details"].(map[string]any)
	if !ok {
		t.Fatalf("details type = %T, want map", m["details"])
	}
	if details["name"] != "required" {
		t.Errorf("details.name = %v, want %q", details["name"], "required")
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	w := httptest.NewRecorder()
	body := `{"name":"test"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	var dst struct {
		Name string `json:"name"`
	}
	if !DecodeJSON(w, r, &dst) {
		t.Fatal("DecodeJSON returned false for valid input")
	}
	if dst.Name != "test" {
		t.Errorf("Name = %q, want %q", dst.Name, "test")
	}
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad"))
	var dst struct{}
	if DecodeJSON(w, r, &dst) {
		t.Fatal("DecodeJSON returned true for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "bad_request" {
		t.Errorf("code = %v, want %q", m["code"], "bad_request")
	}
}

func TestDecodeJSON_BodyTooLarge(t *testing.T) {
	w := httptest.NewRecorder()
	// Simulate the BodyLimit middleware wrapping r.Body with MaxBytesReader.
	big := strings.Repeat("x", (1<<20)+1)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"data":"`+big+`"}`))
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var dst struct{}
	if DecodeJSON(w, r, &dst) {
		t.Fatal("DecodeJSON returned true for oversized body")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["code"] != "payload_too_large" {
		t.Errorf("code = %v, want %q", m["code"], "payload_too_large")
	}
}

func TestSendSSE(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]any{"status": "ok"}
	if err := SendSSE(w, "test:event", payload); err != nil {
		t.Fatalf("SendSSE error: %v", err)
	}

	body := w.Body.String()
	if !strings.HasPrefix(body, "event: test:event\n") {
		t.Errorf("body does not start with event line: %q", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Errorf("body does not contain data line: %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("body does not end with double newline: %q", body)
	}
}

func TestSendSSE_MarshalError(t *testing.T) {
	w := httptest.NewRecorder()
	err := SendSSE(w, "test", math.Inf(1))
	if err == nil {
		t.Error("expected error for unmarshalable value, got nil")
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	m := mustDecodeJSON(t, w.Body)
	if m["error"] != "not found" {
		t.Errorf("error = %v, want %q", m["error"], "not found")
	}
}

func TestMethodNotAllowed_WithMethods(t *testing.T) {
	w := httptest.NewRecorder()
	MethodNotAllowed(w, "GET", "POST")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	allow := w.Header().Get("Allow")
	if allow != "GET, POST" {
		t.Errorf("Allow = %q, want %q", allow, "GET, POST")
	}
	m := mustDecodeJSON(t, w.Body)
	if m["error"] != "method not allowed" {
		t.Errorf("error = %v, want %q", m["error"], "method not allowed")
	}
}

func TestMethodNotAllowed_NoMethods(t *testing.T) {
	w := httptest.NewRecorder()
	MethodNotAllowed(w)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	if allow := w.Header().Get("Allow"); allow != "" {
		t.Errorf("Allow = %q, want empty", allow)
	}
}
