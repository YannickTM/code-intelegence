package domain

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	e := &AppError{Message: "something broke"}
	if got := e.Error(); got != "something broke" {
		t.Errorf("Error() = %q, want %q", got, "something broke")
	}
}

func TestNewAppError(t *testing.T) {
	e := NewAppError(http.StatusTeapot, "teapot", "I'm a teapot")
	if e.Status != http.StatusTeapot {
		t.Errorf("Status = %d, want %d", e.Status, http.StatusTeapot)
	}
	if e.Code != "teapot" {
		t.Errorf("Code = %q, want %q", e.Code, "teapot")
	}
	if e.Message != "I'm a teapot" {
		t.Errorf("Message = %q, want %q", e.Message, "I'm a teapot")
	}
}

func TestErrorf_WithBase(t *testing.T) {
	e := Errorf(ErrBadRequest, "field %s invalid", "name")
	if e.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", e.Status, http.StatusBadRequest)
	}
	if e.Code != "bad_request" {
		t.Errorf("Code = %q, want %q", e.Code, "bad_request")
	}
	if e.Message != "field name invalid" {
		t.Errorf("Message = %q, want %q", e.Message, "field name invalid")
	}
}

func TestErrorf_NilBase(t *testing.T) {
	e := Errorf(nil, "oops")
	if e.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", e.Status, http.StatusInternalServerError)
	}
	if e.Code != "internal_error" {
		t.Errorf("Code = %q, want %q", e.Code, "internal_error")
	}
	if e.Message != "oops" {
		t.Errorf("Message = %q, want %q", e.Message, "oops")
	}
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     *AppError
		status  int
		code    string
		message string
	}{
		{"ErrNotFound", ErrNotFound, 404, "not_found", "not found"},
		{"ErrConflict", ErrConflict, 409, "conflict", "resource already exists"},
		{"ErrBadRequest", ErrBadRequest, 400, "bad_request", "invalid request"},
		{"ErrForbidden", ErrForbidden, 403, "forbidden", "forbidden"},
		{"ErrUnauthorized", ErrUnauthorized, 401, "unauthorized", "authentication required"},
		{"ErrInternal", ErrInternal, 500, "internal_error", "internal server error"},
		{"ErrMethodNotAllowed", ErrMethodNotAllowed, 405, "method_not_allowed", "method not allowed"},
		{"ErrValidation", ErrValidation, 422, "validation_error", "validation failed"},
		{"ErrPayloadTooLarge", ErrPayloadTooLarge, 413, "payload_too_large", "request body too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Status != tt.status {
				t.Errorf("Status = %d, want %d", tt.err.Status, tt.status)
			}
			if tt.err.Code != tt.code {
				t.Errorf("Code = %q, want %q", tt.err.Code, tt.code)
			}
			if tt.err.Message != tt.message {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.message)
			}
		})
	}
}

func TestConvenienceConstructors(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(string) *AppError
		msg     string
		status  int
		code    string
	}{
		{"NotFound", NotFound, "user not found", 404, "not_found"},
		{"Conflict", Conflict, "name taken", 409, "conflict"},
		{"Forbidden", Forbidden, "no access", 403, "forbidden"},
		{"BadRequest", BadRequest, "missing field", 400, "bad_request"},
		{"Unauthorized", Unauthorized, "token expired", 401, "unauthorized"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.fn(tt.msg)
			if e.Status != tt.status {
				t.Errorf("Status = %d, want %d", e.Status, tt.status)
			}
			if e.Code != tt.code {
				t.Errorf("Code = %q, want %q", e.Code, tt.code)
			}
			if e.Message != tt.msg {
				t.Errorf("Message = %q, want %q", e.Message, tt.msg)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	fields := map[string]string{"name": "is required", "email": "invalid format"}
	e := ValidationError(fields)

	if e.Status != 422 {
		t.Errorf("Status = %d, want 422", e.Status)
	}
	if e.Code != "validation_error" {
		t.Errorf("Code = %q, want %q", e.Code, "validation_error")
	}
	if e.Message != "validation failed" {
		t.Errorf("Message = %q, want %q", e.Message, "validation failed")
	}
	details, ok := e.Details.(map[string]string)
	if !ok {
		t.Fatalf("Details type = %T, want map[string]string", e.Details)
	}
	if details["name"] != "is required" {
		t.Errorf("Details[name] = %q, want %q", details["name"], "is required")
	}
	if details["email"] != "invalid format" {
		t.Errorf("Details[email] = %q, want %q", details["email"], "invalid format")
	}
}

func TestAppError_DetailsOmitEmpty(t *testing.T) {
	// Sentinel without Details should omit the field in JSON.
	b, err := json.Marshal(ErrNotFound)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, exists := m["details"]; exists {
		t.Error("details field should be omitted when nil")
	}
}

func TestAppError_DetailsIncluded(t *testing.T) {
	e := ValidationError(map[string]string{"name": "required"})
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	details, ok := m["details"].(map[string]any)
	if !ok {
		t.Fatalf("details type = %T, want map", m["details"])
	}
	if details["name"] != "required" {
		t.Errorf("details.name = %v, want %q", details["name"], "required")
	}
}
