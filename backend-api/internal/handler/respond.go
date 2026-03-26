package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"myjungle/backend-api/internal/domain"
)

// WriteJSON writes a JSON response with the given status code and payload.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed writing response", slog.Any("error", err))
	}
}

// WriteError writes a JSON error response with the standard envelope.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]any{"error": message})
}

// WriteErrorWithCode writes a JSON error response with both code and message.
func WriteErrorWithCode(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, map[string]any{"code": code, "error": message})
}

// WriteAppError writes a structured JSON error response.
// If err is (or wraps) a *domain.AppError, its status, code, message,
// and details are used. Otherwise a generic 500 is returned.
// It also sets the X-Error-Code response header for structured logging.
func WriteAppError(w http.ResponseWriter, err error) {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		w.Header().Set("X-Error-Code", appErr.Code)
		WriteJSON(w, appErr.Status, appErr)
		return
	}
	w.Header().Set("X-Error-Code", domain.ErrInternal.Code)
	WriteJSON(w, http.StatusInternalServerError, domain.ErrInternal)
}

// DecodeJSON decodes the JSON request body into dst.
// Body size limiting is handled by the BodyLimit middleware.
// This function handles MaxBytesError (413) and JSON decode errors (400).
// Returns true on success.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			WriteAppError(w, domain.ErrPayloadTooLarge)
			return false
		}
		WriteAppError(w, domain.BadRequest("invalid JSON body"))
		return false
	}
	return true
}

// SendSSE writes a Server-Sent Event to the response.
// Returns an error if marshalling or writing fails.
func SendSSE(w http.ResponseWriter, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err = w.Write([]byte("event: " + event + "\n")); err != nil {
		return err
	}
	_, err = w.Write([]byte("data: " + string(body) + "\n\n"))
	return err
}

// NotFound writes a 404 JSON response.
func NotFound(w http.ResponseWriter) {
	WriteAppError(w, domain.ErrNotFound)
}

// MethodNotAllowed writes a 405 JSON response with an Allow header.
func MethodNotAllowed(w http.ResponseWriter, methods ...string) {
	if len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
	}
	WriteAppError(w, domain.ErrMethodNotAllowed)
}
