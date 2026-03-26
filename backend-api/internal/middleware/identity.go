package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"

	"github.com/jackc/pgx/v5"
)

// apiKeyPrefix is the common prefix for all MyJungle API keys.
const apiKeyPrefix = "mj_"

// IdentityResolver resolves the caller's identity using (in priority order):
//
//  1. Authorization: Bearer <api_key>   (tokens starting with "mj_")
//  2. Authorization: Bearer <session_token>
//  3. Cookie (cookieName) containing a session token
//
// If a Bearer token is present but invalid/expired, the middleware returns 401
// immediately (no fallthrough). An invalid/expired cookie is cleared and the
// request continues as anonymous. Downstream RequireUser decides whether auth
// is required.
func IdentityResolver(pdb *postgres.DB, sess config.SessionConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pdb == nil || pdb.Queries == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Priority 1: Authorization: Bearer <token>
			if auth.HasBearerScheme(r) {
				rawToken := auth.ExtractBearerToken(r)
				if rawToken == "" {
					// Malformed/empty Bearer token → hard 401.
					writeMiddlewareError(r.Context(), w, domain.ErrUnauthorized)
					return
				}

				// API key tokens start with "mj_" — resolve as API key identity.
				if strings.HasPrefix(rawToken, apiKeyPrefix) {
					keyHash := auth.HashToken(rawToken)
					keyRow, err := pdb.Queries.GetAPIKeyByHash(r.Context(), keyHash)
					if err != nil {
						if errors.Is(err, pgx.ErrNoRows) {
							writeMiddlewareError(r.Context(), w, domain.ErrUnauthorized)
							return
						}
						slog.ErrorContext(r.Context(), "IdentityResolver: GetAPIKeyByHash failed", slog.Any("error", err))
						writeMiddlewareError(r.Context(), w, domain.ErrInternal)
						return
					}
					identity := &domain.APIKeyIdentity{
						KeyHash: keyHash,
						KeyType: keyRow.KeyType,
						Role:    keyRow.Role,
					}
					ctx := auth.ContextWithAPIKey(r.Context(), identity)
					// Fire-and-forget last_used_at update.
					go func() {
						bgCtx, cancel := context.WithTimeout(
							context.WithoutCancel(r.Context()), 5*time.Second)
						defer cancel()
						if err := pdb.Queries.UpdateAPIKeyLastUsed(bgCtx, keyHash); err != nil {
							slog.WarnContext(bgCtx, "IdentityResolver: UpdateAPIKeyLastUsed failed", slog.Any("error", err))
						}
					}()
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				// Session token path.
				tokenHash := auth.HashToken(rawToken)
				row, err := pdb.Queries.GetSessionByTokenHash(r.Context(), tokenHash)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						// Token not found / expired → hard 401 (do NOT fall through).
						writeMiddlewareError(r.Context(), w, domain.ErrUnauthorized)
						return
					}
					// Backend failure → 500.
					slog.ErrorContext(r.Context(), "IdentityResolver: GetSessionByTokenHash failed", slog.Any("error", err))
					writeMiddlewareError(r.Context(), w, domain.ErrInternal)
					return
				}
				u := dbconv.SessionRowToUser(row)
				ctx := auth.ContextWithUser(r.Context(), &u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Priority 2: Session cookie
			if cookie, err := r.Cookie(sess.CookieName); err == nil && cookie.Value != "" {
				tokenHash := auth.HashToken(cookie.Value)
				row, err := pdb.Queries.GetSessionByTokenHash(r.Context(), tokenHash)
				if err == nil {
					u := dbconv.SessionRowToUser(row)
					ctx := auth.ContextWithUser(r.Context(), &u)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					// Backend failure → 500 (don't clear a valid cookie on DB errors).
					slog.ErrorContext(r.Context(), "IdentityResolver: GetSessionByTokenHash (cookie) failed", slog.Any("error", err))
					writeMiddlewareError(r.Context(), w, domain.ErrInternal)
					return
				}
				// Session not found / expired → clear stale cookie, continue as anonymous.
				http.SetCookie(w, &http.Cookie{
					Name:     sess.CookieName,
					Value:    "",
					Path:     "/",
					HttpOnly: true,
					Secure:   sess.SecureCookie,
					SameSite: http.SameSiteLaxMode,
					MaxAge:   -1,
				})
			}

			// No valid session found → continue as anonymous.
			next.ServeHTTP(w, r)
		})
	}
}
