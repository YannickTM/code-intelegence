// Package domain defines shared models and error types.
package domain

import (
	"fmt"
	"net/http"
)

// AppError represents a structured application error.
type AppError struct {
	Status  int    `json:"-"`
	Code    string `json:"code,omitempty"`
	Message string `json:"error"`
	Details any    `json:"details,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

// NewAppError creates a new AppError with the given status, code, and message.
func NewAppError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

// Errorf creates a new AppError based on a template with a formatted message.
// If base is nil, a generic 500 internal error is used as the template.
func Errorf(base *AppError, format string, args ...any) *AppError {
	status := http.StatusInternalServerError
	code := "internal_error"
	if base != nil {
		status = base.Status
		code = base.Code
	}
	return &AppError{
		Status:  status,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// Sentinel errors used across handlers.
var (
	ErrNotFound         = &AppError{Status: 404, Code: "not_found", Message: "not found"}
	ErrConflict         = &AppError{Status: 409, Code: "conflict", Message: "resource already exists"}
	ErrBadRequest       = &AppError{Status: 400, Code: "bad_request", Message: "invalid request"}
	ErrForbidden        = &AppError{Status: 403, Code: "forbidden", Message: "forbidden"}
	ErrUnauthorized     = &AppError{Status: 401, Code: "unauthorized", Message: "authentication required"}
	ErrInternal         = &AppError{Status: 500, Code: "internal_error", Message: "internal server error"}
	ErrMethodNotAllowed = &AppError{Status: 405, Code: "method_not_allowed", Message: "method not allowed"}
	ErrValidation       = &AppError{Status: 422, Code: "validation_error", Message: "validation failed"}
	ErrPayloadTooLarge  = &AppError{Status: 413, Code: "payload_too_large", Message: "request body too large"}
)

// NotFound returns a 404 error with a custom message.
func NotFound(msg string) *AppError {
	return &AppError{Status: ErrNotFound.Status, Code: ErrNotFound.Code, Message: msg}
}

// Conflict returns a 409 error with a custom message.
func Conflict(msg string) *AppError {
	return &AppError{Status: ErrConflict.Status, Code: ErrConflict.Code, Message: msg}
}

// Forbidden returns a 403 error with a custom message.
func Forbidden(msg string) *AppError {
	return &AppError{Status: ErrForbidden.Status, Code: ErrForbidden.Code, Message: msg}
}

// BadRequest returns a 400 error with a custom message.
func BadRequest(msg string) *AppError {
	return &AppError{Status: ErrBadRequest.Status, Code: ErrBadRequest.Code, Message: msg}
}

// Unauthorized returns a 401 error with a custom message.
func Unauthorized(msg string) *AppError {
	return &AppError{Status: ErrUnauthorized.Status, Code: ErrUnauthorized.Code, Message: msg}
}

// ValidationError returns a 422 error with per-field validation details.
func ValidationError(fields map[string]string) *AppError {
	return &AppError{
		Status:  ErrValidation.Status,
		Code:    ErrValidation.Code,
		Message: ErrValidation.Message,
		Details: fields,
	}
}
