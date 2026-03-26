package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	"myjungle/backend-api/internal/domain"
)

// Recover catches panics in downstream handlers, logs the stack trace,
// and returns a 500 JSON error response.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				reqID := GetRequestID(r.Context())
				slog.ErrorContext(r.Context(), "panic recovered",
					slog.Any("error", err),
					slog.String("request_id", reqID),
					slog.String("stack", string(debug.Stack())),
				)

				w.Header().Set("X-Error-Code", domain.ErrInternal.Code)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(domain.ErrInternal)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
