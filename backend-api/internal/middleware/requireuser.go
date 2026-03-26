package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/domain"
)

// RequireUser rejects requests that have no authenticated user in the context.
// It must be applied after IdentityResolver in the middleware chain.
func RequireUser() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := auth.UserFromContext(r.Context()); !ok {
				writeMiddlewareError(r.Context(), w, domain.ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeMiddlewareError writes a structured JSON error response,
// matching the handler.WriteAppError format without importing the handler package.
func writeMiddlewareError(ctx context.Context, w http.ResponseWriter, appErr *domain.AppError) {
	if appErr == nil {
		slog.ErrorContext(ctx, "writeMiddlewareError: called with nil AppError")
		appErr = domain.ErrInternal
	}
	w.Header().Set("X-Error-Code", appErr.Code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.Status)
	if err := json.NewEncoder(w).Encode(appErr); err != nil {
		slog.ErrorContext(ctx, "writeMiddlewareError: encode failed", slog.String("code", appErr.Code), slog.Int("status", appErr.Status), slog.Any("error", err))
	}
}
