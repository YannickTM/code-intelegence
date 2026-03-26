package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AuthHandler manages session-based login and logout.
type AuthHandler struct {
	db      *postgres.DB
	session config.SessionConfig
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(pdb *postgres.DB, session config.SessionConfig) *AuthHandler {
	return &AuthHandler{db: pdb, session: session}
}

// ensureDB checks that the handler's database connection is usable.
func (h *AuthHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// HandleLogin authenticates a user by username and creates a session.
// POST /v1/auth/login — public (no auth required).
//
// Phase 1 uses username-only authentication on trusted local networks
// (ADR-018). The endpoint creates a server-side session and returns the
// raw token plus a Set-Cookie header.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	username := normalizeUsername(body.Username)
	errs := make(validate.Errors)
	validate.Required(username, "username", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	if !h.ensureDB(w) {
		return
	}

	// Look up the user — unknown username is a 401.
	dbUser, err := h.db.Queries.GetUserByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.Unauthorized("invalid credentials"))
			return
		}
		WriteAppError(w, domain.ErrInternal)
		return
	}

	if !dbUser.IsActive {
		WriteAppError(w, domain.Unauthorized("invalid credentials"))
		return
	}

	// Generate token pair (raw for client, hash for DB).
	rawToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	expiresAt := time.Now().UTC().Add(h.session.TTL)

	_, err = h.db.Queries.CreateSession(r.Context(), db.CreateSessionParams{
		UserID:    dbUser.ID,
		TokenHash: tokenHash,
		IpAddress: pgtype.Text{String: r.RemoteAddr, Valid: r.RemoteAddr != ""},
		UserAgent: pgtype.Text{String: r.UserAgent(), Valid: r.UserAgent() != ""},
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Set session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     h.session.CookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.session.SecureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.session.TTL.Seconds()),
	})

	u := dbconv.DBUserToDomain(dbUser)
	WriteJSON(w, http.StatusOK, map[string]any{
		"token":      rawToken,
		"expires_at": expiresAt,
		"user":       u,
	})
}

// HandleLogout destroys the current session.
// POST /v1/auth/logout — authenticated.
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	// Extract the raw token from Bearer header or cookie.
	rawToken := auth.ExtractBearerToken(r)
	if rawToken == "" {
		if cookie, err := r.Cookie(h.session.CookieName); err == nil {
			rawToken = cookie.Value
		}
	}

	if rawToken != "" {
		tokenHash := auth.HashToken(rawToken)
		if err := h.db.Queries.DeleteSession(r.Context(), tokenHash); err != nil {
			slog.ErrorContext(r.Context(), "auth: DeleteSession failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
	}

	// Clear session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     h.session.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.session.SecureCookie,
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusNoContent)
}
