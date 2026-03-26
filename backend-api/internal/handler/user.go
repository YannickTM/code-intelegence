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
	"myjungle/backend-api/internal/jobhealth"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserHandler manages user registration and identity.
type UserHandler struct {
	db        *postgres.DB
	jobHealth *jobhealth.Checker
}

// NewUserHandler creates a new UserHandler backed by the given database.
func NewUserHandler(pdb *postgres.DB, jh *jobhealth.Checker) *UserHandler {
	return &UserHandler{db: pdb, jobHealth: jh}
}

// ensureDB checks that the handler's database connection is usable.
// Returns false (and writes a 500 response) if the DB is nil.
func (h *UserHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// HandleRegister creates a new user (POST /v1/users).
func (h *UserHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	username := normalizeUsername(body.Username)
	errs := make(validate.Errors)
	validate.Required(username, "username", errs)
	validate.ReservedUsername(username, "username", errs)
	email := validate.Email(body.Email, "email", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	displayName := strings.TrimSpace(body.DisplayName)
	if displayName == "" {
		displayName = username
	}

	if !h.ensureDB(w) {
		return
	}

	dbUser, err := h.db.Queries.CreateUser(r.Context(), db.CreateUserParams{
		Username:    username,
		Email:       email,
		DisplayName: pgtype.Text{String: displayName, Valid: true},
	})
	if err != nil {
		if postgres.IsUniqueViolation(err) {
			if postgres.UniqueViolationConstraint(err) == "idx_users_email" {
				WriteAppError(w, domain.Conflict("email already taken"))
				return
			}
			WriteAppError(w, domain.Conflict("username already taken"))
			return
		}
		slog.ErrorContext(r.Context(), "user: CreateUser failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	u := dbconv.DBUserToDomain(dbUser)
	WriteJSON(w, http.StatusCreated, map[string]any{"user": u})
}

// HandleGetMe returns the current user profile (GET /v1/users/me).
func (h *UserHandler) HandleGetMe(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	// Include platform roles so the frontend can gate admin UI.
	var platformRoles []string
	if h.db != nil && h.db.Queries != nil {
		pgID, err := dbconv.StringToPgUUID(u.ID)
		if err == nil {
			roles, err := h.db.Queries.GetUserPlatformRoles(r.Context(), pgID)
			if err == nil {
				platformRoles = roles
			} else {
				slog.WarnContext(r.Context(), "user: GetUserPlatformRoles failed", slog.Any("error", err))
			}
		}
	}
	if platformRoles == nil {
		platformRoles = []string{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"user":           u,
		"platform_roles": platformRoles,
	})
}

// HandleUpdateMe updates the current user profile (PATCH /v1/users/me).
func (h *UserHandler) HandleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	var body struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
		Email       *string `json:"email"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	if body.DisplayName == nil && body.AvatarURL == nil && body.Email == nil {
		WriteAppError(w, domain.ValidationError(map[string]string{
			"body": "at least one field must be provided",
		}))
		return
	}

	if !h.ensureDB(w) {
		return
	}

	pgID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	params := db.UpdateUserProfileParams{ID: pgID}
	if body.DisplayName != nil {
		v := strings.TrimSpace(*body.DisplayName)
		if v == "" {
			WriteAppError(w, domain.ValidationError(map[string]string{
				"display_name": "display_name cannot be blank",
			}))
			return
		}
		params.DisplayName = pgtype.Text{String: v, Valid: true}
	}
	if body.AvatarURL != nil {
		errs := make(validate.Errors)
		v := validate.URL(*body.AvatarURL, "avatar_url", errs)
		if errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		// Valid=true tells the SQL CASE that a value was provided.
		// empty string → NULLIF('','') clears avatar to NULL;
		// non-empty → sets the new URL.
		// When omitted (Valid=false), the SQL ELSE preserves the existing avatar.
		params.AvatarUrl = pgtype.Text{String: v, Valid: true}
	}
	if body.Email != nil {
		errs := make(validate.Errors)
		v := validate.Email(*body.Email, "email", errs)
		if errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		params.Email = pgtype.Text{String: v, Valid: true}
	}

	dbUser, err := h.db.Queries.UpdateUserProfile(r.Context(), params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("user not found"))
			return
		}
		if postgres.IsUniqueViolation(err) {
			WriteAppError(w, domain.Conflict("email already taken"))
			return
		}
		WriteAppError(w, domain.ErrInternal)
		return
	}

	updated := dbconv.DBUserToDomain(dbUser)
	WriteJSON(w, http.StatusOK, map[string]any{"user": updated})
}

// HandleMyProjects lists projects for the current user (GET /v1/users/me/projects).
// Platform admins see all projects with synthetic owner role.
func (h *UserHandler) HandleMyProjects(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	if !h.ensureDB(w) {
		return
	}

	pgID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Platform admin short-circuit: return all projects with owner role.
	isPlatformAdmin := auth.IsPlatformAdmin(r.Context())
	if !isPlatformAdmin {
		hasRole, err := h.db.Queries.HasPlatformRole(r.Context(), db.HasPlatformRoleParams{
			UserID: pgID,
			Role:   domain.PlatformRoleAdmin,
		})
		if err != nil {
			slog.WarnContext(r.Context(), "user: HasPlatformRole check failed, falling through", slog.Any("error", err))
		} else {
			isPlatformAdmin = hasRole
		}
	}

	if isPlatformAdmin {
		rows, err := h.db.Queries.ListAllProjectsWithHealth(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "user: ListAllProjectsWithHealth failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			m := allProjectHealthRowToMap(row)
			// snapshotID not available in this query; orphaned snapshots
			// are cleaned up by the worker-side reaper's sweep pass.
			if row.ActiveJobID.Valid && row.ActiveJobStatus == "running" {
				startedAt := time.Time{}
				if row.ActiveJobStartedAt.Valid {
					startedAt = row.ActiveJobStartedAt.Time
				}
				corrected := h.jobHealth.CheckAndReapIfDead(r.Context(),
					dbconv.PgUUIDToString(row.ActiveJobID),
					row.ActiveJobWorkerID,
					row.ActiveJobStatus,
					startedAt,
					dbconv.PgUUIDToString(row.ID),
					"",
				)
				if corrected == "failed" {
					m["active_job_id"] = nil
					m["active_job_status"] = nil
				}
			}
			items = append(items, m)
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}

	rows, err := h.db.Queries.ListUserProjectsWithHealth(r.Context(), pgID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := projectHealthRowToMap(row)
		// snapshotID not available in this query; orphaned snapshots
		// are cleaned up by the worker-side reaper's sweep pass.
		if row.ActiveJobID.Valid && row.ActiveJobStatus == "running" {
			startedAt := time.Time{}
			if row.ActiveJobStartedAt.Valid {
				startedAt = row.ActiveJobStartedAt.Time
			}
			corrected := h.jobHealth.CheckAndReapIfDead(r.Context(),
				dbconv.PgUUIDToString(row.ActiveJobID),
				row.ActiveJobWorkerID,
				row.ActiveJobStatus,
				startedAt,
				dbconv.PgUUIDToString(row.ID),
				"",
			)
			if corrected == "failed" {
				m["active_job_id"] = nil
				m["active_job_status"] = nil
			}
		}
		items = append(items, m)
	}

	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleLookupUser resolves a username or email to a minimal user profile
// (GET /v1/users/lookup?q={query}).
func (h *UserHandler) HandleLookupUser(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		WriteAppError(w, domain.BadRequest("q query parameter is required"))
		return
	}

	// Try username first.
	user, err := h.db.Queries.GetUserByUsername(r.Context(), strings.ToLower(q))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.ErrorContext(r.Context(), "user: lookup by username failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		// Not found by username — try email.
		user, err = h.db.Queries.GetUserByEmail(r.Context(), strings.ToLower(q))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteAppError(w, domain.NotFound("user not found"))
				return
			}
			slog.ErrorContext(r.Context(), "user: lookup by email failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
	}

	u := map[string]any{
		"id":       dbconv.PgUUIDToString(user.ID),
		"username": user.Username,
		"email":    user.Email,
	}
	if user.DisplayName.Valid {
		u["display_name"] = user.DisplayName.String
	} else {
		u["display_name"] = ""
	}
	if user.AvatarUrl.Valid {
		u["avatar_url"] = user.AvatarUrl.String
	} else {
		u["avatar_url"] = ""
	}
	WriteJSON(w, http.StatusOK, map[string]any{"user": u})
}

func normalizeUsername(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func allProjectHealthRowToMap(row db.ListAllProjectsWithHealthRow) map[string]any {
	m := map[string]any{
		"id":             dbconv.PgUUIDToString(row.ID),
		"name":           row.Name,
		"repo_url":       row.RepoUrl,
		"default_branch": row.DefaultBranch,
		"status":         row.Status,
		"role":           row.Role,
	}
	if row.CreatedBy.Valid {
		m["created_by"] = dbconv.PgUUIDToString(row.CreatedBy)
	}
	if row.CreatedAt.Valid {
		m["created_at"] = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		m["updated_at"] = row.UpdatedAt.Time
	}

	if row.IndexSnapshotID.Valid {
		m["index_git_commit"] = row.IndexGitCommit
		m["index_branch"] = row.IndexBranch
		if row.IndexActivatedAt.Valid {
			m["index_activated_at"] = row.IndexActivatedAt.Time
		} else {
			m["index_activated_at"] = nil
		}
	} else {
		m["index_git_commit"] = nil
		m["index_branch"] = nil
		m["index_activated_at"] = nil
	}

	if row.ActiveJobID.Valid {
		m["active_job_id"] = dbconv.PgUUIDToString(row.ActiveJobID)
		m["active_job_status"] = row.ActiveJobStatus
	} else {
		m["active_job_id"] = nil
		m["active_job_status"] = nil
	}

	if row.FailedJobID.Valid {
		m["failed_job_id"] = dbconv.PgUUIDToString(row.FailedJobID)
		if row.FailedJobFinishedAt.Valid {
			m["failed_job_finished_at"] = row.FailedJobFinishedAt.Time
		} else {
			m["failed_job_finished_at"] = nil
		}
		m["failed_job_type"] = row.FailedJobType
	} else {
		m["failed_job_id"] = nil
		m["failed_job_finished_at"] = nil
		m["failed_job_type"] = nil
	}

	return m
}

func projectHealthRowToMap(row db.ListUserProjectsWithHealthRow) map[string]any {
	m := map[string]any{
		"id":             dbconv.PgUUIDToString(row.ID),
		"name":           row.Name,
		"repo_url":       row.RepoUrl,
		"default_branch": row.DefaultBranch,
		"status":         row.Status,
		"role":           row.Role,
	}
	if row.CreatedBy.Valid {
		m["created_by"] = dbconv.PgUUIDToString(row.CreatedBy)
	}
	if row.CreatedAt.Valid {
		m["created_at"] = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		m["updated_at"] = row.UpdatedAt.Time
	}

	// Index snapshot health — use IndexSnapshotID.Valid as presence sentinel
	// (PK is NOT NULL, so Valid=false means the LATERAL JOIN returned no row).
	if row.IndexSnapshotID.Valid {
		m["index_git_commit"] = row.IndexGitCommit
		m["index_branch"] = row.IndexBranch
		if row.IndexActivatedAt.Valid {
			m["index_activated_at"] = row.IndexActivatedAt.Time
		} else {
			m["index_activated_at"] = nil
		}
	} else {
		m["index_git_commit"] = nil
		m["index_branch"] = nil
		m["index_activated_at"] = nil
	}

	// Active job (queued or running)
	if row.ActiveJobID.Valid {
		m["active_job_id"] = dbconv.PgUUIDToString(row.ActiveJobID)
		m["active_job_status"] = row.ActiveJobStatus
	} else {
		m["active_job_id"] = nil
		m["active_job_status"] = nil
	}

	// Recent failed job — use FailedJobID.Valid as null indicator.
	if row.FailedJobID.Valid {
		m["failed_job_id"] = dbconv.PgUUIDToString(row.FailedJobID)
		if row.FailedJobFinishedAt.Valid {
			m["failed_job_finished_at"] = row.FailedJobFinishedAt.Time
		} else {
			m["failed_job_finished_at"] = nil
		}
		m["failed_job_type"] = row.FailedJobType
	} else {
		m["failed_job_id"] = nil
		m["failed_job_finished_at"] = nil
		m["failed_job_type"] = nil
	}

	return m
}
