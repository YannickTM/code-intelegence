package middleware

import (
	"log/slog"
	"net/http"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"
)

// RequirePlatformAdmin rejects requests from users who do not hold the
// platform_admin role. Must run after IdentityResolver and RequireUser.
//
// On first evaluation per request, queries user_platform_roles and stores
// the result in context via auth.ContextWithPlatformRoles so downstream
// handlers can check platform admin status without an additional DB query.
//
// Responses:
//   - 401 if no user in context
//   - 403 if user is not a platform admin
func RequirePlatformAdmin(pdb *postgres.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.UserFromContext(r.Context())
			if !ok {
				writeMiddlewareError(r.Context(), w, domain.ErrUnauthorized)
				return
			}

			if pdb == nil || pdb.Queries == nil {
				writeMiddlewareError(r.Context(), w, domain.ErrInternal)
				return
			}

			pgUserID, err := dbconv.StringToPgUUID(user.ID)
			if err != nil {
				slog.ErrorContext(r.Context(), "platformadmin: invalid user UUID", slog.String("user_id", user.ID), slog.Any("error", err))
				writeMiddlewareError(r.Context(), w, domain.ErrInternal)
				return
			}

			hasRole, err := pdb.Queries.HasPlatformRole(r.Context(), db.HasPlatformRoleParams{
				UserID: pgUserID,
				Role:   domain.PlatformRoleAdmin,
			})
			if err != nil {
				slog.ErrorContext(r.Context(), "platformadmin: HasPlatformRole failed", slog.Any("error", err))
				writeMiddlewareError(r.Context(), w, domain.ErrInternal)
				return
			}
			if !hasRole {
				writeMiddlewareError(r.Context(), w, domain.Forbidden("platform admin role required"))
				return
			}

			roles, err := pdb.Queries.GetUserPlatformRoles(r.Context(), pgUserID)
			if err != nil {
				slog.ErrorContext(r.Context(), "platformadmin: GetUserPlatformRoles failed", slog.Any("error", err))
				writeMiddlewareError(r.Context(), w, domain.ErrInternal)
				return
			}

			ctx := auth.ContextWithPlatformRoles(r.Context(), roles)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
