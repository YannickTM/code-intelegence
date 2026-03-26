package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AdminHandler implements platform admin endpoints for user management
// and platform role management.
type AdminHandler struct {
	db *postgres.DB
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(pdb *postgres.DB) *AdminHandler {
	return &AdminHandler{db: pdb}
}

func (h *AdminHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// User management endpoints
// ---------------------------------------------------------------------------

// HandleListUsers returns a paginated list of users with project counts and platform roles.
// GET /v1/platform-management/users
func (h *AdminHandler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	limit, offset, err := parsePagination(r, 50, 200)
	if err != nil {
		WriteAppError(w, err)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	searchPattern := ""
	if search != "" {
		searchPattern = "%" + search + "%"
	}

	var isActive pgtype.Bool
	if v := r.URL.Query().Get("is_active"); v != "" {
		switch strings.ToLower(v) {
		case "true":
			isActive = pgtype.Bool{Bool: true, Valid: true}
		case "false":
			isActive = pgtype.Bool{Bool: false, Valid: true}
		}
	}

	sort := r.URL.Query().Get("sort")
	if sort != "username" && sort != "created_at" {
		sort = "created_at"
	}

	ctx := r.Context()

	total, err := h.db.Queries.AdminCountUsers(ctx, db.AdminCountUsersParams{
		Search:   searchPattern,
		IsActive: isActive,
	})
	if err != nil {
		slog.ErrorContext(ctx, "admin: AdminCountUsers failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.AdminListUsers(ctx, db.AdminListUsersParams{
		Search:    searchPattern,
		IsActive:  isActive,
		Sort:      sort,
		RowLimit:  int32(limit),
		RowOffset: int32(offset),
	})
	if err != nil {
		slog.ErrorContext(ctx, "admin: AdminListUsers failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":             dbconv.PgUUIDToString(row.ID),
			"username":       row.Username,
			"email":          row.Email,
			"display_name":   pgTextToPtr(row.DisplayName),
			"avatar_url":     pgTextToPtr(row.AvatarUrl),
			"is_active":      row.IsActive,
			"created_at":     pgTimestamptzToTime(row.CreatedAt),
			"updated_at":     pgTimestamptzToTime(row.UpdatedAt),
			"project_count":  row.ProjectCount,
			"platform_roles": toPlatformRolesSlice(row.PlatformRoles),
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// HandleGetUser returns detailed user information including memberships and platform roles.
// GET /v1/platform-management/users/{userId}
func (h *AdminHandler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	pgUserID, ok := h.parseUserID(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	dbUser, err := h.db.Queries.AdminGetUserByID(ctx, pgUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("user not found"))
			return
		}
		slog.ErrorContext(ctx, "admin: AdminGetUserByID failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	roles, err := h.db.Queries.GetUserPlatformRoles(ctx, pgUserID)
	if err != nil {
		slog.ErrorContext(ctx, "admin: GetUserPlatformRoles failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	memberships, err := h.db.Queries.AdminGetUserMemberships(ctx, pgUserID)
	if err != nil {
		slog.ErrorContext(ctx, "admin: AdminGetUserMemberships failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	membershipItems := make([]map[string]any, 0, len(memberships))
	for _, m := range memberships {
		membershipItems = append(membershipItems, map[string]any{
			"project_id":   dbconv.PgUUIDToString(m.ProjectID),
			"project_name": m.ProjectName,
			"role":         m.Role,
			"joined_at":    pgTimestamptzToTime(m.JoinedAt),
		})
	}

	user := dbconv.DBUserToDomain(dbUser)
	WriteJSON(w, http.StatusOK, map[string]any{
		"user":           user,
		"platform_roles": roles,
		"memberships":    membershipItems,
	})
}

// HandleUpdateUser updates a user's display_name and/or avatar_url.
// PATCH /v1/platform-management/users/{userId}
func (h *AdminHandler) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	pgUserID, ok := h.parseUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	if body.DisplayName == nil && body.AvatarURL == nil {
		WriteAppError(w, domain.BadRequest("at least one field must be provided"))
		return
	}

	errs := make(validate.Errors)
	if body.DisplayName != nil {
		v := strings.TrimSpace(*body.DisplayName)
		if v == "" {
			errs.Add("display_name", "display_name cannot be blank")
		}
	}
	if body.AvatarURL != nil && *body.AvatarURL != "" {
		validate.URL(*body.AvatarURL, "avatar_url", errs)
	}
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	params := db.AdminUpdateUserParams{ID: pgUserID}
	if body.DisplayName != nil {
		params.DisplayName = pgtype.Text{String: strings.TrimSpace(*body.DisplayName), Valid: true}
	}
	if body.AvatarURL != nil {
		params.AvatarUrl = pgtype.Text{String: *body.AvatarURL, Valid: true}
	}

	dbUser, err := h.db.Queries.AdminUpdateUser(r.Context(), params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("user not found"))
			return
		}
		slog.ErrorContext(r.Context(), "admin: AdminUpdateUser failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	user := dbconv.DBUserToDomain(dbUser)
	WriteJSON(w, http.StatusOK, map[string]any{"user": user})
}

// HandleDeactivateUser soft-disables a user and invalidates their sessions.
// POST /v1/platform-management/users/{userId}/deactivate
func (h *AdminHandler) HandleDeactivateUser(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	pgUserID, ok := h.parseUserID(w, r)
	if !ok {
		return
	}
	userIDStr := strings.ToLower(chi.URLParam(r, "userId"))

	ctx := r.Context()

	// Self-check
	actingUser, _ := auth.UserFromContext(ctx)
	if actingUser.ID == userIDStr {
		WriteAppError(w, domain.BadRequest("cannot deactivate yourself"))
		return
	}

	// Fetch target user
	target, err := h.db.Queries.AdminGetUserByID(ctx, pgUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("user not found"))
			return
		}
		slog.ErrorContext(ctx, "admin: AdminGetUserByID failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	if !target.IsActive {
		WriteAppError(w, domain.BadRequest("user is already deactivated"))
		return
	}

	// Deactivate inside a transaction that enforces the last-admin guard.
	// The check MUST be inside the same TX as deactivation: a separate
	// HasPlatformRole read outside the TX is racy because a concurrent
	// GrantPlatformRole can make the target an admin after the read,
	// and a concurrent revoke of another admin can leave this target as
	// the sole admin — bypassing the guard entirely.
	if err := h.db.WithTx(ctx, func(q *db.Queries) error {
		// Lock all active admin rows to serialize concurrent mutations.
		_, err := q.CountActivePlatformAdminsForUpdate(ctx)
		if err != nil {
			return err
		}

		if err := q.DeactivateUser(ctx, pgUserID); err != nil {
			return err
		}

		// After deactivation, verify at least one active admin remains.
		remaining, err := q.CountActivePlatformAdmins(ctx)
		if err != nil {
			return err
		}
		if remaining < 1 {
			return domain.Conflict("cannot deactivate the last active platform admin")
		}

		return nil
	}); err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) {
			WriteAppError(w, appErr)
			return
		}
		slog.ErrorContext(ctx, "admin: deactivate tx failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Invalidate sessions (outside tx — acceptable partial failure)
	if err := h.db.Queries.DeleteUserSessions(ctx, pgUserID); err != nil {
		slog.WarnContext(ctx, "admin: DeleteUserSessions failed", slog.Any("error", err))
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "user deactivated",
		"user_id": userIDStr,
	})
}

// HandleActivateUser re-enables a deactivated user.
// POST /v1/platform-management/users/{userId}/activate
func (h *AdminHandler) HandleActivateUser(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	pgUserID, ok := h.parseUserID(w, r)
	if !ok {
		return
	}
	userIDStr := strings.ToLower(chi.URLParam(r, "userId"))

	ctx := r.Context()

	target, err := h.db.Queries.AdminGetUserByID(ctx, pgUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("user not found"))
			return
		}
		slog.ErrorContext(ctx, "admin: AdminGetUserByID failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	if target.IsActive {
		WriteAppError(w, domain.BadRequest("user is already active"))
		return
	}

	if err := h.db.Queries.ActivateUser(ctx, pgUserID); err != nil {
		slog.ErrorContext(ctx, "admin: ActivateUser failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "user activated",
		"user_id": userIDStr,
	})
}

// ---------------------------------------------------------------------------
// Platform role management endpoints
// ---------------------------------------------------------------------------

// HandleListPlatformRoles returns all active platform role assignments.
// GET /v1/platform-management/platform-roles
func (h *AdminHandler) HandleListPlatformRoles(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	rows, err := h.db.Queries.ListActivePlatformRoleAssignments(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "admin: ListActivePlatformRoleAssignments failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":           dbconv.PgUUIDToString(row.ID),
			"user_id":      dbconv.PgUUIDToString(row.UserID),
			"username":     row.Username,
			"display_name": pgTextToPtr(row.DisplayName),
			"avatar_url":   pgTextToPtr(row.AvatarUrl),
			"role":         row.Role,
			"granted_by":   pgUUIDToPtr(row.GrantedBy),
			"created_at":   pgTimestamptzToTime(row.CreatedAt),
		}
		items = append(items, item)
	}

	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleGrantPlatformRole grants a platform role to a user.
// POST /v1/platform-management/platform-roles
func (h *AdminHandler) HandleGrantPlatformRole(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	errs := make(validate.Errors)
	userIDStr := validate.UUID(body.UserID, "user_id", errs)
	validate.Required(body.Role, "role", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	if !domain.PlatformRoleKnown(body.Role) {
		WriteAppError(w, domain.BadRequest("unknown platform role"))
		return
	}

	pgUserID, _ := dbconv.StringToPgUUID(userIDStr)

	ctx := r.Context()

	// Verify target user exists and is active
	target, err := h.db.Queries.AdminGetUserByID(ctx, pgUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("user not found"))
			return
		}
		slog.ErrorContext(ctx, "admin: AdminGetUserByID failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}
	if !target.IsActive {
		WriteAppError(w, domain.BadRequest("user is not active"))
		return
	}

	// Get acting user for granted_by
	actingUser, _ := auth.UserFromContext(ctx)
	pgGrantedBy, _ := dbconv.StringToPgUUID(actingUser.ID)

	grant, err := h.db.Queries.GrantPlatformRole(ctx, db.GrantPlatformRoleParams{
		UserID:    pgUserID,
		Role:      body.Role,
		GrantedBy: pgGrantedBy,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ErrNoRows from INSERT...SELECT...ON CONFLICT DO NOTHING RETURNING
			// has two causes: (a) conflict — role already exists, or (b) the
			// active-user filter matched no rows so the INSERT never ran.
			// Disambiguate by checking whether the role actually exists.
			hasRole, checkErr := h.db.Queries.HasPlatformRole(ctx, db.HasPlatformRoleParams{
				UserID: pgUserID,
				Role:   body.Role,
			})
			if checkErr != nil {
				slog.ErrorContext(ctx, "admin: HasPlatformRole check failed", slog.Any("error", checkErr))
				WriteAppError(w, domain.ErrInternal)
				return
			}
			if hasRole {
				WriteJSON(w, http.StatusOK, map[string]any{
					"message": "role already granted",
					"user_id": userIDStr,
					"role":    body.Role,
				})
				return
			}
			// INSERT didn't fire — user was concurrently deactivated
			WriteAppError(w, domain.BadRequest("user is not active"))
			return
		}
		slog.ErrorContext(ctx, "admin: GrantPlatformRole failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":         dbconv.PgUUIDToString(grant.ID),
		"user_id":    dbconv.PgUUIDToString(grant.UserID),
		"role":       grant.Role,
		"granted_by": pgUUIDToPtr(grant.GrantedBy),
		"created_at": pgTimestamptzToTime(grant.CreatedAt),
	})
}

// HandleRevokePlatformRole revokes a platform role from a user.
// DELETE /v1/platform-management/platform-roles/{userId}/{role}
func (h *AdminHandler) HandleRevokePlatformRole(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	userIDStr := strings.ToLower(chi.URLParam(r, "userId"))
	role := chi.URLParam(r, "role")

	errs := make(validate.Errors)
	userIDStr = validate.UUID(userIDStr, "userId", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	if !domain.PlatformRoleKnown(role) {
		WriteAppError(w, domain.BadRequest("unknown platform role"))
		return
	}

	pgUserID, _ := dbconv.StringToPgUUID(userIDStr)

	ctx := r.Context()

	// Self-check
	actingUser, _ := auth.UserFromContext(ctx)
	if actingUser.ID == userIDStr && role == domain.PlatformRoleAdmin {
		WriteAppError(w, domain.BadRequest("cannot revoke your own platform_admin role"))
		return
	}

	if role == domain.PlatformRoleAdmin {
		// All operations run inside one transaction. The sequence is:
		// 1. Lock all active admin rows (serializes concurrent revoke/deactivate)
		// 2. Delete the target's role
		// 3. Re-count active admins — if zero remain, roll back
		//
		// Re-counting after deletion is simpler and race-free compared to
		// reading is_active separately: it directly answers "are there still
		// active admins?" regardless of concurrent activate/deactivate.
		if err := h.db.WithTx(ctx, func(q *db.Queries) error {
			// Lock all active admin rows to serialize concurrent mutations.
			_, err := q.CountActivePlatformAdminsForUpdate(ctx)
			if err != nil {
				return err
			}

			rows, err := q.RevokePlatformRole(ctx, db.RevokePlatformRoleParams{
				UserID: pgUserID,
				Role:   role,
			})
			if err != nil {
				return err
			}
			if rows == 0 {
				return domain.NotFound("platform role assignment not found")
			}

			// After deletion, check if at least one active admin remains.
			// This uses the non-locking count since we already hold the locks.
			remaining, err := q.CountActivePlatformAdmins(ctx)
			if err != nil {
				return err
			}
			if remaining < 1 {
				return domain.Conflict("cannot revoke the last platform_admin")
			}

			return nil
		}); err != nil {
			var appErr *domain.AppError
			if errors.As(err, &appErr) {
				WriteAppError(w, appErr)
				return
			}
			slog.ErrorContext(ctx, "admin: revoke tx failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
	} else {
		rows, err := h.db.Queries.RevokePlatformRole(ctx, db.RevokePlatformRoleParams{
			UserID: pgUserID,
			Role:   role,
		})
		if err != nil {
			slog.ErrorContext(ctx, "admin: RevokePlatformRole failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		if rows == 0 {
			WriteAppError(w, domain.NotFound("platform role assignment not found"))
			return
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "platform role revoked",
		"user_id": userIDStr,
		"role":    role,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseUserID extracts and validates the {userId} URL parameter.
func (h *AdminHandler) parseUserID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	idStr := chi.URLParam(r, "userId")
	errs := make(validate.Errors)
	idStr = validate.UUID(idStr, "userId", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return pgtype.UUID{}, false
	}
	pgID, err := dbconv.StringToPgUUID(idStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return pgtype.UUID{}, false
	}
	return pgID, true
}

// pgTextToPtr converts a pgtype.Text to a *string (nil when not valid).
func pgTextToPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// pgUUIDToPtr converts a pgtype.UUID to a *string (nil when not valid).
func pgUUIDToPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := dbconv.PgUUIDToString(u)
	return &s
}

// pgTimestamptzToTime converts a pgtype.Timestamptz to a time.Time.
func pgTimestamptzToTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

// toPlatformRolesSlice converts the interface{} value from array_agg to []string.
func toPlatformRolesSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	switch roles := v.(type) {
	case []string:
		return roles
	case []interface{}:
		result := make([]string, 0, len(roles))
		for _, r := range roles {
			if s, ok := r.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return []string{}
	}
}
