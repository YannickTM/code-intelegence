package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/embedding"
	"myjungle/backend-api/internal/jobhealth"
	"myjungle/backend-api/internal/llm"
	"myjungle/backend-api/internal/queue"
	"myjungle/backend-api/internal/sshkey"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ProjectHandler serves project CRUD and sub-resource endpoints.
type ProjectHandler struct {
	db           *postgres.DB
	sshSvc       *sshkey.Service
	embeddingSvc *embedding.Service
	llmSvc       *llm.Service
	publisher    queue.JobEnqueuer
	jobHealth    *jobhealth.Checker
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(pdb *postgres.DB, sshSvc *sshkey.Service, embeddingSvc *embedding.Service, llmSvc *llm.Service, publisher queue.JobEnqueuer, jh *jobhealth.Checker) *ProjectHandler {
	return &ProjectHandler{db: pdb, sshSvc: sshSvc, embeddingSvc: embeddingSvc, llmSvc: llmSvc, publisher: publisher, jobHealth: jh}
}

// SetPublisher replaces the queue publisher (used by tests to inject fakes).
func (h *ProjectHandler) SetPublisher(p queue.JobEnqueuer) { h.publisher = p }

func (h *ProjectHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// HandleCreate creates a project (POST /v1/projects).
func (h *ProjectHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	var body struct {
		Name          string `json:"name"`
		RepoURL       string `json:"repo_url"`
		DefaultBranch string `json:"default_branch,omitempty"`
		Status        string `json:"status,omitempty"`
		SSHKeyID      string `json:"ssh_key_id"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	errs := make(validate.Errors)
	name := validate.Required(strings.TrimSpace(body.Name), "name", errs)
	validate.MaxLength(name, 100, "name", errs)
	repoURL := validate.Required(strings.TrimSpace(body.RepoURL), "repo_url", errs)
	validate.RepoURL(repoURL, "repo_url", errs)
	sshKeyIDStr := validate.Required(body.SSHKeyID, "ssh_key_id", errs)
	validate.UUID(sshKeyIDStr, "ssh_key_id", errs)
	if s := strings.TrimSpace(body.Status); s != "" {
		body.Status = s
		validate.OneOf(s, []string{"active", "paused"}, "status", errs)
	}
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}
	sshKeyID, err := dbconv.StringToPgUUID(sshKeyIDStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	var project db.Project
	var sshKeyRow db.GetProjectSSHKeyWithDetailsRow

	err = h.db.WithTx(r.Context(), func(q *db.Queries) error {
		// 1. Verify SSH key exists and belongs to user.
		_, err := q.GetSSHKey(r.Context(), db.GetSSHKeyParams{
			ID:        sshKeyID,
			CreatedBy: userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("ssh key not found")
			}
			return err
		}

		// 2. Insert project.
		var defaultBranch, status interface{}
		if trimmed := strings.TrimSpace(body.DefaultBranch); trimmed != "" {
			defaultBranch = trimmed
		}
		if body.Status != "" {
			status = body.Status
		}
		project, err = q.CreateProject(r.Context(), db.CreateProjectParams{
			Name:          name,
			RepoUrl:       repoURL,
			CreatedBy:     userID,
			DefaultBranch: defaultBranch,
			Status:        status,
		})
		if err != nil {
			return err
		}

		// 3. Add creator as owner.
		_, err = q.CreateProjectMember(r.Context(), db.CreateProjectMemberParams{
			ProjectID: project.ID,
			UserID:    userID,
			Role:      domain.RoleOwner,
			InvitedBy: userID,
		})
		if err != nil {
			return err
		}

		// 4. Assign SSH key.
		_, err = q.AssignProjectSSHKey(r.Context(), db.AssignProjectSSHKeyParams{
			ProjectID: project.ID,
			SshKeyID:  sshKeyID,
		})
		if err != nil {
			return err
		}

		// 5. Fetch SSH key details for response.
		sshKeyRow, err = q.GetProjectSSHKeyWithDetails(r.Context(), project.ID)
		return err
	})
	if err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) {
			WriteAppError(w, appErr)
			return
		}
		if postgres.IsUniqueViolation(err) {
			WriteAppError(w, domain.Conflict("project with this name already exists"))
			return
		}
		slog.ErrorContext(r.Context(), "project: create failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	resp := dbconv.DBProjectToDomain(project)
	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":             resp.ID,
		"name":           resp.Name,
		"repo_url":       resp.RepoURL,
		"default_branch": resp.DefaultBranch,
		"status":         resp.Status,
		"created_by":     resp.CreatedBy,
		"created_at":     resp.CreatedAt,
		"updated_at":     resp.UpdatedAt,
		"ssh_key":        dbconv.DBSSHKeySummaryToDomain(sshKeyRow),
	})
}

// HandleList returns the authenticated user's projects (GET /v1/projects).
func (h *ProjectHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	limit := 20
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if offset > math.MaxInt32 {
		WriteAppError(w, domain.BadRequest("offset exceeds maximum allowed value"))
		return
	}

	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	total, err := h.db.Queries.CountProjectsByMember(r.Context(), userID)
	if err != nil {
		slog.ErrorContext(r.Context(), "project: count failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.ListProjectsByMember(r.Context(), db.ListProjectsByMemberParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "project: list failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]domain.Project, 0, len(rows))
	for _, row := range rows {
		items = append(items, dbconv.DBProjectToDomain(row))
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"data":   items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// HandleGet returns a project by ID (GET /v1/projects/{projectID}).
func (h *ProjectHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	row, err := h.db.Queries.GetProjectWithHealth(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("project not found"))
			return
		}
		slog.ErrorContext(r.Context(), "project: get failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	m := projectHealthDetailToMap(row)
	if row.ActiveJobID.Valid && row.ActiveJobStatus == "running" {
		startedAt := time.Time{}
		if row.ActiveJobStartedAt.Valid {
			startedAt = row.ActiveJobStartedAt.Time
		}
		// snapshotID not available in this query; orphaned snapshots
		// are cleaned up by the worker-side reaper's sweep pass.
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
	WriteJSON(w, http.StatusOK, m)
}

// projectHealthDetailToMap serializes a GetProjectWithHealthRow to a JSON-ready map.
func projectHealthDetailToMap(row db.GetProjectWithHealthRow) map[string]any {
	m := map[string]any{
		"id":             dbconv.PgUUIDToString(row.ID),
		"name":           row.Name,
		"repo_url":       row.RepoUrl,
		"default_branch": row.DefaultBranch,
		"status":         row.Status,
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

	// Index snapshot health
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

	// Active job
	if row.ActiveJobID.Valid {
		m["active_job_id"] = dbconv.PgUUIDToString(row.ActiveJobID)
		m["active_job_status"] = row.ActiveJobStatus
	} else {
		m["active_job_id"] = nil
		m["active_job_status"] = nil
	}

	// Recent failed job
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

// HandleUpdate updates a project (PATCH /v1/projects/{projectID}).
func (h *ProjectHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	var body struct {
		Name          *string `json:"name"`
		RepoURL       *string `json:"repo_url"`
		DefaultBranch *string `json:"default_branch"`
		Status        *string `json:"status"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	errs := make(validate.Errors)
	params := db.UpdateProjectParams{ID: projectID}

	if body.Name != nil {
		v := strings.TrimSpace(*body.Name)
		validate.Required(v, "name", errs)
		validate.MaxLength(v, 100, "name", errs)
		params.Name = pgtype.Text{String: v, Valid: true}
	}
	if body.RepoURL != nil {
		v := strings.TrimSpace(*body.RepoURL)
		validate.Required(v, "repo_url", errs)
		validate.RepoURL(v, "repo_url", errs)
		params.RepoUrl = pgtype.Text{String: v, Valid: true}
	}
	if body.DefaultBranch != nil {
		v := strings.TrimSpace(*body.DefaultBranch)
		validate.Required(v, "default_branch", errs)
		params.DefaultBranch = pgtype.Text{String: v, Valid: true}
	}
	if body.Status != nil {
		s := strings.TrimSpace(*body.Status)
		validate.OneOf(s, []string{"active", "paused"}, "status", errs)
		params.Status = pgtype.Text{String: s, Valid: true}
	}
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	project, err := h.db.Queries.UpdateProject(r.Context(), params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("project not found"))
			return
		}
		if postgres.IsUniqueViolation(err) {
			WriteAppError(w, domain.Conflict("project with this name already exists"))
			return
		}
		slog.ErrorContext(r.Context(), "project: update failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, dbconv.DBProjectToDomain(project))
}

// HandleDelete deletes a project (DELETE /v1/projects/{projectID}).
func (h *ProjectHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	err := h.db.Queries.DeleteProject(r.Context(), projectID)
	if err != nil {
		slog.ErrorContext(r.Context(), "project: delete failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetSSHKey returns the SSH key assigned to a project (GET /v1/projects/{projectID}/ssh-key).
func (h *ProjectHandler) HandleGetSSHKey(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	row, err := h.db.Queries.GetProjectSSHKeyWithDetails(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("no ssh key assigned to this project"))
			return
		}
		slog.ErrorContext(r.Context(), "project: get ssh key failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, dbconv.DBSSHKeySummaryToDomain(row))
}

// HandleSetSSHKey assigns an SSH key to a project (PUT /v1/projects/{projectID}/ssh-key).
// Accepts either {"ssh_key_id": "..."} to reassign an existing key,
// or {"generate": true, "name": "..."} to generate and assign a new key.
func (h *ProjectHandler) HandleSetSSHKey(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	projectID, pOk := h.parseProjectID(w, r)
	if !pOk {
		return
	}

	var body struct {
		SSHKeyID string `json:"ssh_key_id"`
		Generate bool   `json:"generate"`
		Name     string `json:"name"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	// Reject ambiguous requests that specify both generate and ssh_key_id.
	if body.Generate && body.SSHKeyID != "" {
		errs := make(validate.Errors)
		errs.Add("ssh_key_id", "cannot provide ssh_key_id when generate is true")
		WriteAppError(w, errs.ToAppError())
		return
	}

	if body.Generate {
		errs := make(validate.Errors)
		validate.Required(strings.TrimSpace(body.Name), "name", errs)
		if errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		h.setSSHKeyGenerate(w, r, projectID, u, strings.TrimSpace(body.Name))
	} else {
		errs := make(validate.Errors)
		sshKeyIDStr := validate.Required(body.SSHKeyID, "ssh_key_id", errs)
		validate.UUID(sshKeyIDStr, "ssh_key_id", errs)
		if errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		h.setSSHKeyReassign(w, r, projectID, u, sshKeyIDStr)
	}
}

func (h *ProjectHandler) setSSHKeyReassign(w http.ResponseWriter, r *http.Request, projectID pgtype.UUID, u *domain.User, sshKeyIDStr string) {
	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}
	sshKeyID, err := dbconv.StringToPgUUID(sshKeyIDStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	var sshKeyRow db.GetProjectSSHKeyWithDetailsRow

	err = h.db.WithTx(r.Context(), func(q *db.Queries) error {
		// Verify key exists and belongs to user.
		_, err := q.GetSSHKey(r.Context(), db.GetSSHKeyParams{
			ID:        sshKeyID,
			CreatedBy: userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("ssh key not found")
			}
			return err
		}

		// Lock current active assignment to serialize concurrent reassignments.
		if _, err := q.LockActiveProjectSSHKeyAssignment(r.Context(), projectID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Deactivate current assignment (if any).
		if err := q.DeactivateProjectSSHKeyAssignments(r.Context(), projectID); err != nil {
			return err
		}

		// Assign new key.
		if _, err := q.AssignProjectSSHKey(r.Context(), db.AssignProjectSSHKeyParams{
			ProjectID: projectID,
			SshKeyID:  sshKeyID,
		}); err != nil {
			return err
		}

		sshKeyRow, err = q.GetProjectSSHKeyWithDetails(r.Context(), projectID)
		return err
	})
	if err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) {
			WriteAppError(w, appErr)
			return
		}
		slog.ErrorContext(r.Context(), "project: set ssh key (reassign) failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, dbconv.DBSSHKeySummaryToDomain(sshKeyRow))
}

func (h *ProjectHandler) setSSHKeyGenerate(w http.ResponseWriter, r *http.Request, projectID pgtype.UUID, u *domain.User, name string) {
	if h.sshSvc == nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	pub, fingerprint, keyType, encryptedPriv, err := h.sshSvc.Create()
	if err != nil {
		slog.ErrorContext(r.Context(), "project: generate ssh key failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	var sshKeyRow db.GetProjectSSHKeyWithDetailsRow

	err = h.db.WithTx(r.Context(), func(q *db.Queries) error {
		// Create the SSH key.
		dbKey, err := q.CreateSSHKey(r.Context(), db.CreateSSHKeyParams{
			Name:                name,
			PublicKey:           pub,
			PrivateKeyEncrypted: encryptedPriv,
			KeyType:             keyType,
			Fingerprint:         fingerprint,
			CreatedBy:           userID,
		})
		if err != nil {
			return err
		}

		// Lock current active assignment to serialize concurrent reassignments.
		if _, err := q.LockActiveProjectSSHKeyAssignment(r.Context(), projectID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Deactivate current assignment (if any).
		if err := q.DeactivateProjectSSHKeyAssignments(r.Context(), projectID); err != nil {
			return err
		}

		// Assign new key.
		if _, err := q.AssignProjectSSHKey(r.Context(), db.AssignProjectSSHKeyParams{
			ProjectID: projectID,
			SshKeyID:  dbKey.ID,
		}); err != nil {
			return err
		}

		sshKeyRow, err = q.GetProjectSSHKeyWithDetails(r.Context(), projectID)
		return err
	})
	if err != nil {
		if postgres.IsUniqueViolation(err) {
			WriteAppError(w, domain.Conflict("ssh key with this fingerprint already exists"))
			return
		}
		slog.ErrorContext(r.Context(), "project: set ssh key (generate) failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, dbconv.DBSSHKeySummaryToDomain(sshKeyRow))
}

// HandleRemoveSSHKey removes the SSH key assignment from a project
// (DELETE /v1/projects/{projectID}/ssh-key).
func (h *ProjectHandler) HandleRemoveSSHKey(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	err := h.db.Queries.DeactivateProjectSSHKeyAssignments(r.Context(), projectID)
	if err != nil {
		slog.ErrorContext(r.Context(), "project: remove ssh key failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseProjectID extracts and validates the projectID URL parameter.
func (h *ProjectHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	idStr := chi.URLParam(r, "projectID")
	errs := make(validate.Errors)
	validate.UUID(idStr, "project_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return pgtype.UUID{}, false
	}
	id, err := dbconv.StringToPgUUID(idStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return pgtype.UUID{}, false
	}
	return id, true
}

// HandleIndex triggers indexing for a project (POST /v1/projects/{projectID}/index).
func (h *ProjectHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}
	projectIDStr := chi.URLParam(r, "projectID")

	// Parse optional body for job_type (default: "full").
	jobType := "full"
	if r.Body != nil {
		var body struct {
			JobType string `json:"job_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			// Empty body or EOF is fine — use default job_type.
			if !errors.Is(err, io.EOF) {
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					_, _ = io.Copy(io.Discard, r.Body)
					_ = r.Body.Close()
					WriteAppError(w, domain.ErrPayloadTooLarge)
				} else {
					WriteAppError(w, domain.BadRequest("invalid JSON body"))
				}
				return
			}
		} else if body.JobType != "" {
			jobType = body.JobType
		}
	}
	// Validate job_type via MapJobTypeToWorkflow — single source of truth for
	// both validation and the workflow name used during enqueue.
	workflow, wfErr := queue.MapJobTypeToWorkflow(jobType)
	if wfErr != nil {
		WriteAppError(w, domain.ValidationError(map[string]string{
			"job_type": "must be \"full\" or \"incremental\"",
		}))
		return
	}

	// Check SSH key assignment.
	if _, err := h.db.Queries.GetProjectSSHKeyAssignment(r.Context(), projectID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.Conflict("project has no active SSH key assignment"))
			return
		}
		slog.ErrorContext(r.Context(), "project: index: ssh key check failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Resolve embedding config (required).
	embeddingResolved, err := h.embeddingSvc.GetResolvedConfig(r.Context(), projectIDStr)
	if err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) && appErr.Status == http.StatusNotFound {
			WriteAppError(w, domain.Conflict("project has no embedding provider configured"))
			return
		}
		slog.ErrorContext(r.Context(), "project: index: resolve embedding failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}
	embeddingConfigUUID, err := dbconv.StringToPgUUID(embeddingResolved.Config.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "project: index: parse embedding config id failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Resolve LLM config (optional — not required for indexing).
	var llmConfigUUID pgtype.UUID // zero value = NULL
	llmResolved, err := h.llmSvc.GetResolvedConfig(r.Context(), projectIDStr)
	if err == nil {
		if parsed, parseErr := dbconv.StringToPgUUID(llmResolved.Config.ID); parseErr == nil {
			llmConfigUUID = parsed
		}
	} else {
		var appErr *domain.AppError
		if !errors.As(err, &appErr) || appErr.Status != http.StatusNotFound {
			slog.ErrorContext(r.Context(), "project: index: resolve llm config failed", slog.Any("error", err))
		}
	}

	// Dedup: check for existing active job.
	existing, err := h.db.Queries.FindActiveIndexingJobForProjectAndType(r.Context(), db.FindActiveIndexingJobForProjectAndTypeParams{
		ProjectID: projectID,
		JobType:   jobType,
	})
	if err == nil {
		WriteJSON(w, http.StatusAccepted, indexingJobToMap(existing))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		slog.ErrorContext(r.Context(), "project: index: find active job failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Create new job.
	job, err := h.db.Queries.CreateIndexingJob(r.Context(), db.CreateIndexingJobParams{
		ProjectID:                 projectID,
		IndexSnapshotID:           pgtype.UUID{}, // nil at creation
		JobType:                   jobType,
		EmbeddingProviderConfigID: embeddingConfigUUID,
		LlmProviderConfigID:      llmConfigUUID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "project: index: create job failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Enqueue to Redis/asynq (workflow was resolved earlier during validation).
	if h.publisher != nil {
		requestedBy := resolveRequestedBy(r)
		jobIDStr := dbconv.PgUUIDToString(job.ID)

		if enqErr := h.publisher.EnqueueIndexJob(r.Context(), jobIDStr, workflow, projectIDStr, requestedBy); enqErr != nil {
			// Truncate error to avoid leaking sensitive data (e.g. Redis credentials)
			// in both logs and persisted error details.
			msg := enqErr.Error()
			if len(msg) > 256 {
				msg = msg[:256]
			}

			slog.ErrorContext(r.Context(), "project: index: enqueue failed",
				slog.String("job_id", jobIDStr), slog.String("error", msg))

			// Mark the job as failed so it doesn't sit in "queued" forever.
			// Use a detached context: the compensating write must succeed even
			// if the original request context was cancelled.
			failCtx, failCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer failCancel()
			errDetails, _ := json.Marshal([]map[string]string{{
				"stage":     "enqueue",
				"message":   fmt.Sprintf("failed to enqueue to Redis: %s", msg),
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}})
			if failErr := h.db.Queries.SetIndexingJobFailed(failCtx, db.SetIndexingJobFailedParams{
				ID:           job.ID,
				ErrorDetails: errDetails,
			}); failErr != nil {
				slog.ErrorContext(r.Context(), "project: index: failed to mark job as failed",
					slog.String("job_id", jobIDStr), slog.Any("error", failErr))
			}

			WriteAppError(w, domain.ErrInternal)
			return
		}
	}

	WriteJSON(w, http.StatusAccepted, indexingJobToMap(job))
}

// HandleListJobs lists indexing jobs for a project (GET /v1/projects/{projectID}/jobs).
func (h *ProjectHandler) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if offset > math.MaxInt32 {
		WriteAppError(w, domain.BadRequest("offset exceeds maximum allowed value"))
		return
	}

	total, err := h.db.Queries.CountProjectJobs(r.Context(), projectID)
	if err != nil {
		slog.ErrorContext(r.Context(), "project: count jobs failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	jobs, err := h.db.Queries.ListProjectJobs(r.Context(), db.ListProjectJobsParams{
		ProjectID: projectID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "project: list jobs failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]map[string]any, len(jobs))
	for i, j := range jobs {
		item := indexingJobToMap(j)
		if j.Status == "running" {
			workerID := ""
			if j.WorkerID.Valid {
				workerID = j.WorkerID.String
			}
			startedAt := time.Time{}
			if j.StartedAt.Valid {
				startedAt = j.StartedAt.Time
			}
			snapshotID := ""
			if j.IndexSnapshotID.Valid {
				snapshotID = dbconv.PgUUIDToString(j.IndexSnapshotID)
			}
			corrected := h.jobHealth.CheckAndReapIfDead(r.Context(),
				dbconv.PgUUIDToString(j.ID),
				workerID,
				j.Status,
				startedAt,
				dbconv.PgUUIDToString(j.ProjectID),
				snapshotID,
			)
			item["status"] = corrected
		}
		items[i] = item
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// resolveRequestedBy builds an observability hint from the request context.
func resolveRequestedBy(r *http.Request) string {
	if u, ok := auth.UserFromContext(r.Context()); ok {
		return "user:" + u.ID
	}
	if k, ok := auth.APIKeyFromContext(r.Context()); ok {
		return "api-key:" + k.KeyType
	}
	return ""
}

// indexingJobToMap converts a sqlc IndexingJob to a JSON-ready map.
func indexingJobToMap(j db.IndexingJob) map[string]any {
	m := map[string]any{
		"id":              dbconv.PgUUIDToString(j.ID),
		"project_id":      dbconv.PgUUIDToString(j.ProjectID),
		"job_type":        j.JobType,
		"status":          j.Status,
		"files_processed": j.FilesProcessed,
		"chunks_upserted": j.ChunksUpserted,
		"vectors_deleted": j.VectorsDeleted,
	}

	// Nullable UUID fields
	if j.IndexSnapshotID.Valid {
		m["index_snapshot_id"] = dbconv.PgUUIDToString(j.IndexSnapshotID)
	} else {
		m["index_snapshot_id"] = nil
	}
	if j.EmbeddingProviderConfigID.Valid {
		m["embedding_provider_config_id"] = dbconv.PgUUIDToString(j.EmbeddingProviderConfigID)
	} else {
		m["embedding_provider_config_id"] = nil
	}
	if j.LlmProviderConfigID.Valid {
		m["llm_provider_config_id"] = dbconv.PgUUIDToString(j.LlmProviderConfigID)
	} else {
		m["llm_provider_config_id"] = nil
	}

	// error_details: JSONB stored as []byte, decode to []any for JSON output.
	var details []any
	if len(j.ErrorDetails) > 0 {
		if err := json.Unmarshal(j.ErrorDetails, &details); err != nil {
			details = []any{}
		}
	}
	if details == nil {
		details = []any{}
	}
	m["error_details"] = details

	// Nullable timestamps
	if j.StartedAt.Valid {
		m["started_at"] = j.StartedAt.Time
	} else {
		m["started_at"] = nil
	}
	if j.FinishedAt.Valid {
		m["finished_at"] = j.FinishedAt.Time
	} else {
		m["finished_at"] = nil
	}
	if j.CreatedAt.Valid {
		m["created_at"] = j.CreatedAt.Time
	} else {
		m["created_at"] = nil
	}

	return m
}

// ── Code search types ───────────────────────────────────────────────

type codeSearchRequest struct {
	Query       string `json:"query"`
	SearchMode  string `json:"search_mode"`
	Language    string `json:"language"`
	FilePattern string `json:"file_pattern"`
	IncludeDir  string `json:"include_dir"`
	ExcludeDir  string `json:"exclude_dir"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
}

type codeSearchMatch struct {
	ChunkID    string  `json:"chunk_id"`
	FilePath   string  `json:"file_path"`
	Language   *string `json:"language,omitempty"`
	StartLine  int32   `json:"start_line"`
	EndLine    int32   `json:"end_line"`
	Content    string  `json:"content"`
	MatchCount int     `json:"match_count"`
}

type codeSearchResponse struct {
	Items      []codeSearchMatch `json:"items"`
	Total      int64             `json:"total"`
	SnapshotID string            `json:"snapshot_id"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}

// HandleSearch searches through code chunks (POST /v1/projects/{projectID}/query/search).
func (h *ProjectHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	var body codeSearchRequest
	if !DecodeJSON(w, r, &body) {
		return
	}

	// Validate required query field
	if strings.TrimSpace(body.Query) == "" {
		WriteAppError(w, domain.BadRequest("query is required"))
		return
	}
	if len(body.Query) > 1000 {
		WriteAppError(w, domain.BadRequest("query exceeds maximum length of 1000"))
		return
	}

	// Parse search mode
	sm, smErr := parseSearchMode(body.SearchMode)
	if smErr != nil {
		WriteAppError(w, domain.BadRequest(smErr.Error()))
		return
	}

	// Parse file pattern
	filePatterns, fpErr := parseFilePattern(body.FilePattern)
	if fpErr != nil {
		WriteAppError(w, fpErr)
		return
	}

	// Parse directory filters
	includeDirs, dirErr := parseDirFilter(body.IncludeDir, "include_dir")
	if dirErr != nil {
		WriteAppError(w, dirErr)
		return
	}
	excludeDirs, dirErr := parseDirFilter(body.ExcludeDir, "exclude_dir")
	if dirErr != nil {
		WriteAppError(w, dirErr)
		return
	}

	// Apply pagination defaults
	limit := body.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := body.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > math.MaxInt32 {
		WriteAppError(w, domain.BadRequest("offset exceeds maximum allowed value"))
		return
	}

	// Get active snapshot
	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, codeSearchResponse{
			Items:  []codeSearchMatch{},
			Limit:  limit,
			Offset: offset,
		})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	qp := codeSearchParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
		Query:           body.Query,
		SearchMode:      sm,
		Language:        body.Language,
		FilePatterns:    filePatterns,
		IncludeDirs:     includeDirs,
		ExcludeDirs:     excludeDirs,
		Limit:           int32(limit),
		Offset:          int32(offset),
	}

	dataSQL, countSQL, dataArgs, countArgs, buildErr := buildCodeSearchQuery(qp)
	if buildErr != nil {
		WriteAppError(w, domain.NewAppError(422, "validation_error", buildErr.Error()))
		return
	}

	// Execute data query
	rows, err := h.db.Pool.Query(r.Context(), dataSQL, dataArgs...)
	if err != nil {
		if isInvalidRegexError(err) {
			WriteAppError(w, domain.NewAppError(422, "validation_error",
				"invalid regex pattern: "+err.Error()))
			return
		}
		slog.ErrorContext(r.Context(), "code search query failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}
	defer rows.Close()

	var items []codeSearchMatch
	for rows.Next() {
		var cr codeSearchResult
		if scanErr := rows.Scan(
			&cr.ID, &cr.FilePath, &cr.Language,
			&cr.StartLine, &cr.EndLine, &cr.Content,
		); scanErr != nil {
			slog.ErrorContext(r.Context(), "scan code search row", slog.Any("error", scanErr))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		items = append(items, codeSearchMatch{
			ChunkID:    dbconv.PgUUIDToString(cr.ID),
			FilePath:   cr.FilePath,
			Language:   nullableString(cr.Language),
			StartLine:  cr.StartLine,
			EndLine:    cr.EndLine,
			Content:    cr.Content,
			MatchCount: countMatches(cr.Content, body.Query, sm),
		})
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(r.Context(), "code search rows error", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Execute count query
	var total int64
	if countErr := h.db.Pool.QueryRow(r.Context(), countSQL, countArgs...).Scan(&total); countErr != nil {
		slog.ErrorContext(r.Context(), "code search count failed", slog.Any("error", countErr))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	if items == nil {
		items = []codeSearchMatch{}
	}

	WriteJSON(w, http.StatusOK, codeSearchResponse{
		Items:      items,
		Total:      total,
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
		Limit:      limit,
		Offset:     offset,
	})
}

// ── Symbol response types ────────────────────────────────────────────

type symbolResponse struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	QualifiedName  *string         `json:"qualified_name,omitempty"`
	Kind           string          `json:"kind"`
	Signature      *string         `json:"signature,omitempty"`
	StartLine      *int32          `json:"start_line,omitempty"`
	EndLine        *int32          `json:"end_line,omitempty"`
	DocText        *string         `json:"doc_text,omitempty"`
	FilePath       string          `json:"file_path"`
	Language       string          `json:"language,omitempty"`
	Flags          json.RawMessage `json:"flags,omitempty"`
	Modifiers      []string        `json:"modifiers"`
	ReturnType     *string         `json:"return_type,omitempty"`
	ParameterTypes []string        `json:"parameter_types"`
}

type symbolListResponse struct {
	Items      []symbolResponse `json:"items"`
	Total      int64            `json:"total"`
	SnapshotID string           `json:"snapshot_id"`
	Limit      int32            `json:"limit"`
	Offset     int32            `json:"offset"`
}

func nullableInt32(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

func makeSymbolResponse(
	id pgtype.UUID, name string, qualifiedName pgtype.Text, kind string,
	signature pgtype.Text, startLine, endLine pgtype.Int4,
	docText pgtype.Text, filePath string, language pgtype.Text,
	flags []byte, modifiers []string, returnType pgtype.Text, parameterTypes []string,
) symbolResponse {
	if modifiers == nil {
		modifiers = []string{}
	}
	if parameterTypes == nil {
		parameterTypes = []string{}
	}
	var flagsJSON json.RawMessage
	if len(flags) > 0 {
		flagsJSON = json.RawMessage(flags)
	}
	return symbolResponse{
		ID:             dbconv.PgUUIDToString(id),
		Name:           name,
		QualifiedName:  nullableString(qualifiedName),
		Kind:           kind,
		Signature:      nullableString(signature),
		StartLine:      nullableInt32(startLine),
		EndLine:        nullableInt32(endLine),
		DocText:        nullableString(docText),
		FilePath:       filePath,
		Language:       language.String,
		Flags:          flagsJSON,
		Modifiers:      modifiers,
		ReturnType:     nullableString(returnType),
		ParameterTypes: parameterTypes,
	}
}

// HandleListSymbols lists symbols for a project (GET /v1/projects/{projectID}/symbols).
func (h *ProjectHandler) HandleListSymbols(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	limit, offset, pgErr := parsePagination(r, 50, 200)
	if pgErr != nil {
		WriteAppError(w, pgErr)
		return
	}

	nameFilter := r.URL.Query().Get("name")
	kindFilter := r.URL.Query().Get("kind")
	searchModeStr := r.URL.Query().Get("search_mode")
	includeDirRaw := r.URL.Query().Get("include_dir")
	excludeDirRaw := r.URL.Query().Get("exclude_dir")

	sm, smErr := parseSearchMode(searchModeStr)
	if smErr != nil {
		WriteAppError(w, domain.BadRequest(smErr.Error()))
		return
	}

	includeDirs, dirErr := parseDirFilter(includeDirRaw, "include_dir")
	if dirErr != nil {
		WriteAppError(w, dirErr)
		return
	}
	excludeDirs, dirErr := parseDirFilter(excludeDirRaw, "exclude_dir")
	if dirErr != nil {
		WriteAppError(w, dirErr)
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, symbolListResponse{
			Items:  []symbolResponse{},
			Limit:  int32(limit),
			Offset: int32(offset),
		})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	qp := symbolQueryParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
		NameFilter:      nameFilter,
		KindFilter:      kindFilter,
		SearchMode:      sm,
		IncludeDirs:     includeDirs,
		ExcludeDirs:     excludeDirs,
		Limit:           int32(limit),
		Offset:          int32(offset),
	}

	dataSQL, countSQL, dataArgs, countArgs, buildErr := buildSymbolListQuery(qp)
	if buildErr != nil {
		WriteAppError(w, domain.NewAppError(422, "validation_error", buildErr.Error()))
		return
	}

	// Execute data query
	rows, err := h.db.Pool.Query(r.Context(), dataSQL, dataArgs...)
	if err != nil {
		if isInvalidRegexError(err) {
			WriteAppError(w, domain.NewAppError(422, "validation_error",
				"invalid regex pattern: "+err.Error()))
			return
		}
		slog.ErrorContext(r.Context(), "list symbols failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}
	defer rows.Close()

	var items []symbolResponse
	for rows.Next() {
		var qr symbolQueryResult
		if scanErr := rows.Scan(
			&qr.ID, &qr.Name, &qr.QualifiedName, &qr.Kind, &qr.Signature,
			&qr.StartLine, &qr.EndLine, &qr.DocText, &qr.FilePath, &qr.Language,
			&qr.Flags, &qr.Modifiers, &qr.ReturnType, &qr.ParameterTypes,
		); scanErr != nil {
			slog.ErrorContext(r.Context(), "scan symbol row", slog.Any("error", scanErr))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		items = append(items, makeSymbolResponse(qr.ID, qr.Name, qr.QualifiedName,
			qr.Kind, qr.Signature, qr.StartLine, qr.EndLine, qr.DocText,
			qr.FilePath, qr.Language,
			qr.Flags, qr.Modifiers, qr.ReturnType, qr.ParameterTypes))
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(r.Context(), "list symbols rows error", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Execute count query
	var total int64
	if countErr := h.db.Pool.QueryRow(r.Context(), countSQL, countArgs...).Scan(&total); countErr != nil {
		slog.ErrorContext(r.Context(), "count symbols failed", slog.Any("error", countErr))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	if items == nil {
		items = []symbolResponse{}
	}

	WriteJSON(w, http.StatusOK, symbolListResponse{
		Items:      items,
		Total:      total,
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
}

// HandleGetSymbol returns a symbol by ID (GET /v1/projects/{projectID}/symbols/{symbolID}).
func (h *ProjectHandler) HandleGetSymbol(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	errs := make(validate.Errors)
	validate.UUID(chi.URLParam(r, "symbolID"), "symbol_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}
	symbolID, err := dbconv.StringToPgUUID(chi.URLParam(r, "symbolID"))
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	row, err := h.db.Queries.GetSymbolByID(r.Context(), db.GetSymbolByIDParams{
		ID:        symbolID,
		ProjectID: projectID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteAppError(w, domain.NotFound("symbol not found"))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get symbol failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, makeSymbolResponse(row.ID, row.Name, row.QualifiedName, row.Kind,
		row.Signature, row.StartLine, row.EndLine, row.DocText, row.FilePath, row.Language,
		row.Flags, row.Modifiers, row.ReturnType, row.ParameterTypes))
}

// ── Dependency response types ────────────────────────────────────────

type externalPackage struct {
	PackageName    string `json:"package_name"`
	PackageVersion string `json:"package_version,omitempty"`
	ImportCount    int64  `json:"import_count"`
	FileCount      int64  `json:"file_count"`
}

type fileDependencyCount struct {
	FilePath        string `json:"file_path"`
	ImportsCount    int64  `json:"imports_count"`
	ImportedByCount int64  `json:"imported_by_count"`
}

type dependenciesOverviewResponse struct {
	ExternalPackages     []externalPackage     `json:"external_packages"`
	FileDependencyCounts []fileDependencyCount `json:"file_dependency_counts"`
	TotalFiles           int64                 `json:"total_files"`
	SnapshotID           string                `json:"snapshot_id"`
	Limit                int32                 `json:"limit"`
	Offset               int32                 `json:"offset"`
}

type dependencyEdge struct {
	ID             string  `json:"id"`
	SourceFilePath string  `json:"source_file_path"`
	TargetFilePath *string `json:"target_file_path"`
	ImportName     string  `json:"import_name"`
	ImportType     string  `json:"import_type"`
	PackageName    *string `json:"package_name,omitempty"`
	PackageVersion *string `json:"package_version,omitempty"`
}

type fileDependenciesResponse struct {
	FilePath   string           `json:"file_path"`
	Imports    []dependencyEdge `json:"imports"`
	ImportedBy []dependencyEdge `json:"imported_by"`
	SnapshotID string           `json:"snapshot_id"`
}

type graphNode struct {
	FilePath   string `json:"file_path"`
	Language   string `json:"language,omitempty"`
	IsExternal bool   `json:"is_external"`
	Depth      int    `json:"depth"`
}

type graphEdge struct {
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	ImportName  string  `json:"import_name"`
	ImportType  string  `json:"import_type"`
	PackageName *string `json:"package_name,omitempty"`
}

type dependencyGraphResponse struct {
	Nodes      []graphNode `json:"nodes"`
	Edges      []graphEdge `json:"edges"`
	Root       string      `json:"root"`
	Depth      int         `json:"depth"`
	Truncated  bool        `json:"truncated"`
	SnapshotID string      `json:"snapshot_id"`
}

// ── Dependency helpers ───────────────────────────────────────────────

func toDependencyEdge(id pgtype.UUID, src string, tgt pgtype.Text,
	importName, importType string, pkgName, pkgVersion pgtype.Text) dependencyEdge {
	edge := dependencyEdge{
		ID:             dbconv.PgUUIDToString(id),
		SourceFilePath: src,
		ImportName:     importName,
		ImportType:     importType,
	}
	if tgt.Valid {
		edge.TargetFilePath = &tgt.String
	}
	if pkgName.Valid {
		edge.PackageName = &pkgName.String
	}
	if pkgVersion.Valid {
		edge.PackageVersion = &pkgVersion.String
	}
	return edge
}

func nullableString(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func deduplicateEdges(edges []graphEdge) []graphEdge {
	type edgeKey struct{ source, target, importName string }
	seen := make(map[edgeKey]struct{}, len(edges))
	out := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		k := edgeKey{e.Source, e.Target, e.ImportName}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, e)
	}
	return out
}

// ── HandleDependencies ───────────────────────────────────────────────

// HandleDependencies returns dependency overview for a project (GET /v1/projects/{projectID}/dependencies).
func (h *ProjectHandler) HandleDependencies(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	limit, offset, pgErr := parsePagination(r, 50, 200)
	if pgErr != nil {
		WriteAppError(w, pgErr)
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, dependenciesOverviewResponse{
			ExternalPackages:     []externalPackage{},
			FileDependencyCounts: []fileDependencyCount{},
		})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	extPkgs, err := h.db.Queries.ListExternalDependencies(r.Context(), db.ListExternalDependenciesParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "list external deps failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	fileCounts, err := h.db.Queries.ListFileDependencyCounts(r.Context(), db.ListFileDependencyCountsParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
		Limit:           int32(limit),
		Offset:          int32(offset),
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "list file dep counts failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	totalFiles, err := h.db.Queries.CountFilesWithDependencies(r.Context(), db.CountFilesWithDependenciesParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "count files with deps failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	extItems := make([]externalPackage, len(extPkgs))
	for i, ep := range extPkgs {
		extItems[i] = externalPackage{
			PackageName:    ep.PackageName.String,
			PackageVersion: ep.PackageVersion.String,
			ImportCount:    ep.ImportCount,
			FileCount:      ep.FileCount,
		}
	}

	fileItems := make([]fileDependencyCount, len(fileCounts))
	for i, fc := range fileCounts {
		fileItems[i] = fileDependencyCount{
			FilePath:        fc.FilePath,
			ImportsCount:    fc.ImportsCount,
			ImportedByCount: fc.ImportedByCount,
		}
	}

	WriteJSON(w, http.StatusOK, dependenciesOverviewResponse{
		ExternalPackages:     extItems,
		FileDependencyCounts: fileItems,
		TotalFiles:           totalFiles,
		SnapshotID:           dbconv.PgUUIDToString(snap.ID),
		Limit:                int32(limit),
		Offset:               int32(offset),
	})
}

// ── HandleFileDependencies ───────────────────────────────────────────

// HandleFileDependencies returns bidirectional dependencies for a single file
// (GET /v1/projects/{projectID}/files/dependencies?file_path=...).
func (h *ProjectHandler) HandleFileDependencies(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteAppError(w, domain.NotFound("no active snapshot"))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	imports, err := h.db.Queries.ListDependenciesFromFile(r.Context(), db.ListDependenciesFromFileParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
		SourceFilePath:  filePath,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "list deps from file failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	importedBy, err := h.db.Queries.ListDependenciesToFile(r.Context(), db.ListDependenciesToFileParams{
		ProjectID:       projectID,
		IndexSnapshotID: snap.ID,
		TargetFilePath:  pgtype.Text{String: filePath, Valid: true},
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "list deps to file failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	importEdges := make([]dependencyEdge, len(imports))
	for i, d := range imports {
		importEdges[i] = toDependencyEdge(d.ID, d.SourceFilePath, d.TargetFilePath,
			d.ImportName, d.ImportType, d.PackageName, d.PackageVersion)
	}

	importedByEdges := make([]dependencyEdge, len(importedBy))
	for i, d := range importedBy {
		importedByEdges[i] = toDependencyEdge(d.ID, d.SourceFilePath, d.TargetFilePath,
			d.ImportName, d.ImportType, d.PackageName, d.PackageVersion)
	}

	WriteJSON(w, http.StatusOK, fileDependenciesResponse{
		FilePath:   filePath,
		Imports:    importEdges,
		ImportedBy: importedByEdges,
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
	})
}

// ── HandleDependencyGraph ────────────────────────────────────────────

const (
	defaultGraphDepth = 2
	maxGraphDepth     = 5
	maxGraphNodes     = 200
)

// HandleDependencyGraph returns a BFS dependency graph rooted at a file
// (GET /v1/projects/{projectID}/dependencies/graph?root=...&depth=2).
func (h *ProjectHandler) HandleDependencyGraph(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	root := r.URL.Query().Get("root")
	if root == "" {
		WriteAppError(w, domain.BadRequest("root query param required"))
		return
	}

	depth := defaultGraphDepth
	if v := r.URL.Query().Get("depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			depth = n
		}
	}
	if depth > maxGraphDepth {
		depth = maxGraphDepth
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteAppError(w, domain.NotFound("no active snapshot"))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// BFS traversal
	visited := map[string]int{root: 0} // file_path → depth
	var allEdges []graphEdge
	frontier := []string{root}
	truncated := false

	for level := 0; level < depth && len(frontier) > 0; level++ {
		fwdDeps, err := h.db.Queries.ListDependenciesFromFiles(r.Context(),
			db.ListDependenciesFromFilesParams{
				ProjectID:       projectID,
				IndexSnapshotID: snap.ID,
				Column3:         frontier,
			})
		if err != nil {
			slog.ErrorContext(r.Context(), "graph bfs forward failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		revDeps, err := h.db.Queries.ListDependenciesToFiles(r.Context(),
			db.ListDependenciesToFilesParams{
				ProjectID:       projectID,
				IndexSnapshotID: snap.ID,
				Column3:         frontier,
			})
		if err != nil {
			slog.ErrorContext(r.Context(), "graph bfs reverse failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		var nextFrontier []string

		// Process forward edges
		for _, d := range fwdDeps {
			target := ""
			isExternal := false
			if d.TargetFilePath.Valid {
				target = d.TargetFilePath.String
			} else if d.PackageName.Valid {
				target = "ext:" + d.PackageName.String
				isExternal = true
			} else {
				target = "ext:" + d.ImportName
				isExternal = true
			}

			allEdges = append(allEdges, graphEdge{
				Source:      d.SourceFilePath,
				Target:      target,
				ImportName:  d.ImportName,
				ImportType:  d.ImportType,
				PackageName: nullableString(d.PackageName),
			})

			if _, seen := visited[target]; !seen {
				if len(visited) >= maxGraphNodes {
					truncated = true
					continue
				}
				visited[target] = level + 1
				if !isExternal {
					nextFrontier = append(nextFrontier, target)
				}
			}
		}

		// Process reverse edges
		for _, d := range revDeps {
			allEdges = append(allEdges, graphEdge{
				Source:      d.SourceFilePath,
				Target:      d.TargetFilePath.String,
				ImportName:  d.ImportName,
				ImportType:  d.ImportType,
				PackageName: nullableString(d.PackageName),
			})

			if _, seen := visited[d.SourceFilePath]; !seen {
				if len(visited) >= maxGraphNodes {
					truncated = true
					continue
				}
				visited[d.SourceFilePath] = level + 1
				nextFrontier = append(nextFrontier, d.SourceFilePath)
			}
		}

		if truncated {
			break
		}

		frontier = nextFrontier
	}

	// Build node list
	nodes := make([]graphNode, 0, len(visited))
	for fp, d := range visited {
		isExt := strings.HasPrefix(fp, "ext:")
		nodes = append(nodes, graphNode{
			FilePath:   fp,
			IsExternal: isExt,
			Depth:      d,
		})
	}

	allEdges = deduplicateEdges(allEdges)

	// Drop edges that reference nodes not in visited (can happen when
	// the node cap truncates the graph mid-BFS level).
	if truncated {
		filtered := make([]graphEdge, 0, len(allEdges))
		for _, e := range allEdges {
			if _, srcOK := visited[e.Source]; srcOK {
				if _, tgtOK := visited[e.Target]; tgtOK {
					filtered = append(filtered, e)
				}
			}
		}
		allEdges = filtered
	}

	WriteJSON(w, http.StatusOK, dependencyGraphResponse{
		Nodes:      nodes,
		Edges:      allEdges,
		Root:       root,
		Depth:      depth,
		Truncated:  truncated,
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
	})
}

// ── File tree types ──────────────────────────────────────────────────

// FileNode represents a single node in the file tree (file or directory).
type FileNode struct {
	Path     string      `json:"path"`
	Name     string      `json:"name"`
	NodeType string      `json:"node_type"` // "file" or "directory"
	Language string      `json:"language,omitempty"`
	Size     int64       `json:"size_bytes,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// structureResponse is the JSON envelope for GET /structure.
type structureResponse struct {
	Root       *FileNode `json:"root"`
	SnapshotID string    `json:"snapshot_id"`
	GitCommit  string    `json:"git_commit"`
	Branch     string    `json:"branch"`
	FileCount  int       `json:"file_count"`
}

// fileContextResponse is the JSON envelope for GET /files/context.
type fileContextResponse struct {
	FilePath      string          `json:"file_path"`
	Language      string          `json:"language"`
	SizeBytes     int64           `json:"size_bytes"`
	LineCount     int32           `json:"line_count"`
	ContentHash   string          `json:"content_hash"`
	Content       string          `json:"content"`
	TreeSitterAST     json.RawMessage `json:"tree_sitter_ast,omitempty"`
	FileFacts         json.RawMessage `json:"file_facts,omitempty"`
	Issues            json.RawMessage `json:"issues,omitempty"`
	ParserMeta        json.RawMessage `json:"parser_meta,omitempty"`
	ExtractorStatuses json.RawMessage `json:"extractor_statuses,omitempty"`
	SnapshotID        string          `json:"snapshot_id"`
	LastIndexedAt     string          `json:"last_indexed_at"`
}

// ── HandleStructure ──────────────────────────────────────────────────

// HandleStructure returns the file tree for a project (GET /v1/projects/{projectID}/structure).
func (h *ProjectHandler) HandleStructure(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, structureResponse{
			Root: &FileNode{Path: ".", Name: ".", NodeType: "directory", Children: []*FileNode{}},
		})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "project: get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	files, err := h.db.Queries.ListSnapshotFiles(r.Context(), snap.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "project: list snapshot files failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	root := buildFileTree(files)

	WriteJSON(w, http.StatusOK, structureResponse{
		Root:       root,
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
		GitCommit:  snap.GitCommit,
		Branch:     snap.Branch,
		FileCount:  len(files),
	})
}

// ── HandleFileContext ────────────────────────────────────────────────

// HandleFileContext returns file context (GET /v1/projects/{projectID}/files/context).
func (h *ProjectHandler) HandleFileContext(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}
	includeAST := r.URL.Query().Get("include_ast") == "true"

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteAppError(w, domain.NotFound("no active snapshot"))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "project: get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	var resp fileContextResponse

	if includeAST {
		row, err := h.db.Queries.GetFileWithContentAndAST(r.Context(), db.GetFileWithContentAndASTParams{
			IndexSnapshotID: snap.ID,
			FilePath:        filePath,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("file not found"))
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "project: get file content failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		if !row.Content.Valid {
			slog.ErrorContext(r.Context(), "project: file content unavailable", slog.String("file_path", filePath))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		resp = fileContextResponse{
			FilePath:    row.FilePath,
			Language:    row.Language.String,
			SizeBytes:   row.SizeBytes.Int64,
			LineCount:   row.LineCount.Int32,
			ContentHash: row.ContentHash.String,
			Content:     row.Content.String,
			SnapshotID:  dbconv.PgUUIDToString(snap.ID),
		}
		if row.TreeSitterAst != nil {
			resp.TreeSitterAST = json.RawMessage(row.TreeSitterAst)
		}
		if row.FileFacts != nil {
			resp.FileFacts = json.RawMessage(row.FileFacts)
		}
		if row.Issues != nil {
			resp.Issues = json.RawMessage(row.Issues)
		}
		if row.ParserMeta != nil {
			resp.ParserMeta = json.RawMessage(row.ParserMeta)
		}
		if row.ExtractorStatuses != nil {
			resp.ExtractorStatuses = json.RawMessage(row.ExtractorStatuses)
		}
	} else {
		row, err := h.db.Queries.GetFileWithContent(r.Context(), db.GetFileWithContentParams{
			IndexSnapshotID: snap.ID,
			FilePath:        filePath,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("file not found"))
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "project: get file content failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		if !row.Content.Valid {
			slog.ErrorContext(r.Context(), "project: file content unavailable", slog.String("file_path", filePath))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		resp = fileContextResponse{
			FilePath:    row.FilePath,
			Language:    row.Language.String,
			SizeBytes:   row.SizeBytes.Int64,
			LineCount:   row.LineCount.Int32,
			ContentHash: row.ContentHash.String,
			Content:     row.Content.String,
			SnapshotID:  dbconv.PgUUIDToString(snap.ID),
		}
		if row.FileFacts != nil {
			resp.FileFacts = json.RawMessage(row.FileFacts)
		}
		if row.Issues != nil {
			resp.Issues = json.RawMessage(row.Issues)
		}
		if row.ParserMeta != nil {
			resp.ParserMeta = json.RawMessage(row.ParserMeta)
		}
		if row.ExtractorStatuses != nil {
			resp.ExtractorStatuses = json.RawMessage(row.ExtractorStatuses)
		}
	}

	if snap.ActivatedAt.Valid {
		resp.LastIndexedAt = snap.ActivatedAt.Time.Format(time.RFC3339)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// ── File history ─────────────────────────────────────────────────────

// FileHistoryEntry is a single commit that modified a file.
type FileHistoryEntry struct {
	DiffID         string `json:"diff_id"`
	CommitHash     string `json:"commit_hash"`
	ShortHash      string `json:"short_hash"`
	AuthorName     string `json:"author_name"`
	CommitterDate  string `json:"committer_date"`
	MessageSubject string `json:"message_subject"`
	ChangeType     string `json:"change_type"`
	Additions      int32  `json:"additions"`
	Deletions      int32  `json:"deletions"`
}

// FileHistoryResponse is the paginated file history.
type FileHistoryResponse struct {
	Items  []FileHistoryEntry `json:"items"`
	Total  int64              `json:"total"`
	Limit  int32              `json:"limit"`
	Offset int32              `json:"offset"`
}

// HandleFileHistory returns paginated commit history for a specific file.
// GET /v1/projects/{projectID}/files/history?file_path=...&limit=10&offset=0
func (h *ProjectHandler) HandleFileHistory(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}

	limit, offset, pgErr := parsePagination(r, 10, 50)
	if pgErr != nil {
		WriteAppError(w, pgErr)
		return
	}

	fp := pgtype.Text{String: filePath, Valid: true}

	rows, err := h.db.Queries.ListFileDiffsByPathWithCommit(r.Context(), db.ListFileDiffsByPathWithCommitParams{
		ProjectID:   projectID,
		FilePath:    fp,
		QueryLimit:  int32(limit),
		QueryOffset: int32(offset),
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "files: list history failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	total, err := h.db.Queries.CountFileDiffsByPath(r.Context(), db.CountFileDiffsByPathParams{
		ProjectID: projectID,
		FilePath:  fp,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "files: count history failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]FileHistoryEntry, len(rows))
	for i, row := range rows {
		items[i] = toFileHistoryEntry(row)
	}

	WriteJSON(w, http.StatusOK, FileHistoryResponse{
		Items:  items,
		Total:  total,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
}

func toFileHistoryEntry(row db.ListFileDiffsByPathWithCommitRow) FileHistoryEntry {
	hash := row.CommitHash
	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	subject := row.Message
	if idx := strings.IndexByte(row.Message, '\n'); idx >= 0 {
		subject = row.Message[:idx]
	}
	return FileHistoryEntry{
		DiffID:         dbconv.PgUUIDToString(row.DiffID),
		CommitHash:     hash,
		ShortHash:      shortHash,
		AuthorName:     row.AuthorName,
		CommitterDate:  row.CommitterDate.Time.Format(time.RFC3339),
		MessageSubject: subject,
		ChangeType:     row.ChangeType,
		Additions:      row.Additions,
		Deletions:      row.Deletions,
	}
}

// ── buildFileTree ────────────────────────────────────────────────────

// buildFileTree converts a flat list of files (ordered by path) into a nested
// directory tree. Directories are sorted before files at each level, both
// alphabetically.
func buildFileTree(files []db.File) *FileNode {
	root := &FileNode{
		Path:     ".",
		Name:     ".",
		NodeType: "directory",
		Children: []*FileNode{},
	}

	// Map from directory path → *FileNode for quick lookup.
	dirs := map[string]*FileNode{".": root, "": root}

	for _, f := range files {
		parts := strings.Split(f.FilePath, "/")

		// Ensure all intermediate directories exist.
		for i := 0; i < len(parts)-1; i++ {
			dirPath := strings.Join(parts[:i+1], "/")
			if _, exists := dirs[dirPath]; exists {
				continue
			}
			parentPath := "."
			if i > 0 {
				parentPath = strings.Join(parts[:i], "/")
			}
			dirNode := &FileNode{
				Path:     dirPath,
				Name:     parts[i],
				NodeType: "directory",
				Children: []*FileNode{},
			}
			dirs[dirPath] = dirNode
			parent := dirs[parentPath]
			parent.Children = append(parent.Children, dirNode)
		}

		// Attach the file node to its parent directory.
		parentPath := "."
		if len(parts) > 1 {
			parentPath = strings.Join(parts[:len(parts)-1], "/")
		}

		fileNode := &FileNode{
			Path:     f.FilePath,
			Name:     parts[len(parts)-1],
			NodeType: "file",
			Language: f.Language.String,
			Size:     f.SizeBytes.Int64,
		}
		parent := dirs[parentPath]
		parent.Children = append(parent.Children, fileNode)
	}

	// Sort each directory: directories first (alpha), then files (alpha).
	sortFileNodes(root)

	return root
}

// sortFileNodes recursively sorts children at each level: directories first
// (alphabetical), then files (alphabetical).
func sortFileNodes(node *FileNode) {
	if node.Children == nil {
		return
	}
	sort.Slice(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]
		if a.NodeType != b.NodeType {
			return a.NodeType == "directory" // directories first
		}
		return a.Name < b.Name
	})
	for _, child := range node.Children {
		if child.NodeType == "directory" {
			sortFileNodes(child)
		}
	}
}

// HandleConventions returns conventions for a project (GET /v1/projects/{projectID}/conventions).
func (h *ProjectHandler) HandleConventions(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{},
	})
}

// ── File Analysis Endpoints ──────────────────────────────────────────

// --- Response types ---

type fileExportResponse struct {
	ID           string  `json:"id"`
	ExportKind   string  `json:"export_kind"`
	ExportedName string  `json:"exported_name"`
	LocalName    *string `json:"local_name,omitempty"`
	SourceModule *string `json:"source_module,omitempty"`
	Line         int32   `json:"line"`
	Column       int32   `json:"column"`
}

type fileExportsResponse struct {
	FilePath   string               `json:"file_path"`
	Items      []fileExportResponse `json:"items"`
	Total      int                  `json:"total"`
	SnapshotID string               `json:"snapshot_id"`
}

type symbolReferenceResponse struct {
	ID                  string  `json:"id"`
	ReferenceKind       string  `json:"reference_kind"`
	TargetName          string  `json:"target_name"`
	QualifiedTargetHint *string `json:"qualified_target_hint,omitempty"`
	RawText             *string `json:"raw_text,omitempty"`
	StartLine           int32   `json:"start_line"`
	StartColumn         int32   `json:"start_column"`
	EndLine             int32   `json:"end_line"`
	EndColumn           int32   `json:"end_column"`
	ResolutionScope     *string `json:"resolution_scope,omitempty"`
	Confidence          *string `json:"confidence,omitempty"`
}

type fileReferencesResponse struct {
	FilePath   string                    `json:"file_path"`
	Items      []symbolReferenceResponse `json:"items"`
	Total      int                       `json:"total"`
	SnapshotID string                    `json:"snapshot_id"`
}

type jsxUsageResponse struct {
	ID            string  `json:"id"`
	ComponentName string  `json:"component_name"`
	IsIntrinsic   bool    `json:"is_intrinsic"`
	IsFragment    bool    `json:"is_fragment"`
	Line          int32   `json:"line"`
	Column        int32   `json:"column"`
	Confidence    *string `json:"confidence,omitempty"`
}

type fileJsxUsagesResponse struct {
	FilePath   string             `json:"file_path"`
	Items      []jsxUsageResponse `json:"items"`
	Total      int                `json:"total"`
	SnapshotID string             `json:"snapshot_id"`
}

type networkCallResponse struct {
	ID          string  `json:"id"`
	ClientKind  string  `json:"client_kind"`
	Method      string  `json:"method"`
	URLLiteral  *string `json:"url_literal,omitempty"`
	URLTemplate *string `json:"url_template,omitempty"`
	IsRelative  bool    `json:"is_relative"`
	StartLine   int32   `json:"start_line"`
	StartColumn int32   `json:"start_column"`
	Confidence  *string `json:"confidence,omitempty"`
}

type fileNetworkCallsResponse struct {
	FilePath   string                `json:"file_path"`
	Items      []networkCallResponse `json:"items"`
	Total      int                   `json:"total"`
	SnapshotID string                `json:"snapshot_id"`
}

// --- Mapper helpers ---

func toFileExportResponse(e db.Export) fileExportResponse {
	return fileExportResponse{
		ID:           dbconv.PgUUIDToString(e.ID),
		ExportKind:   e.ExportKind,
		ExportedName: e.ExportedName,
		LocalName:    nullableString(e.LocalName),
		SourceModule: nullableString(e.SourceModule),
		Line:         e.Line,
		Column:       e.Column,
	}
}

func toSymbolReferenceResponse(r db.SymbolReference) symbolReferenceResponse {
	return symbolReferenceResponse{
		ID:                  dbconv.PgUUIDToString(r.ID),
		ReferenceKind:       r.ReferenceKind,
		TargetName:          r.TargetName,
		QualifiedTargetHint: nullableString(r.QualifiedTargetHint),
		RawText:             nullableString(r.RawText),
		StartLine:           r.StartLine,
		StartColumn:         r.StartColumn,
		EndLine:             r.EndLine,
		EndColumn:           r.EndColumn,
		ResolutionScope:     nullableString(r.ResolutionScope),
		Confidence:          nullableString(r.Confidence),
	}
}

func toJsxUsageResponse(j db.JsxUsage) jsxUsageResponse {
	return jsxUsageResponse{
		ID:            dbconv.PgUUIDToString(j.ID),
		ComponentName: j.ComponentName,
		IsIntrinsic:   j.IsIntrinsic,
		IsFragment:    j.IsFragment,
		Line:          j.Line,
		Column:        j.Column,
		Confidence:    nullableString(j.Confidence),
	}
}

func toNetworkCallResponse(n db.NetworkCall) networkCallResponse {
	return networkCallResponse{
		ID:          dbconv.PgUUIDToString(n.ID),
		ClientKind:  n.ClientKind,
		Method:      n.Method,
		URLLiteral:  nullableString(n.UrlLiteral),
		URLTemplate: nullableString(n.UrlTemplate),
		IsRelative:  n.IsRelative,
		StartLine:   n.StartLine,
		StartColumn: n.StartColumn,
		Confidence:  nullableString(n.Confidence),
	}
}

// --- Handlers ---

// HandleFileExports returns exports declared by a file
// (GET /v1/projects/{projectID}/files/exports?file_path=...).
func (h *ProjectHandler) HandleFileExports(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileExportsResponse{FilePath: filePath, Items: []fileExportResponse{}})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	fileID, err := h.db.Queries.GetFileBySnapshotAndPath(r.Context(), db.GetFileBySnapshotAndPathParams{
		IndexSnapshotID: snap.ID,
		FilePath:        filePath,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileExportsResponse{FilePath: filePath, Items: []fileExportResponse{}, SnapshotID: dbconv.PgUUIDToString(snap.ID)})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get file by path failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.ListExportsByFileID(r.Context(), fileID)
	if err != nil {
		slog.ErrorContext(r.Context(), "list exports failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]fileExportResponse, len(rows))
	for i, row := range rows {
		items[i] = toFileExportResponse(row)
	}

	WriteJSON(w, http.StatusOK, fileExportsResponse{
		FilePath:   filePath,
		Items:      items,
		Total:      len(items),
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
	})
}

// HandleFileReferences returns symbol references made within a file
// (GET /v1/projects/{projectID}/files/references?file_path=...).
func (h *ProjectHandler) HandleFileReferences(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileReferencesResponse{FilePath: filePath, Items: []symbolReferenceResponse{}})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	fileID, err := h.db.Queries.GetFileBySnapshotAndPath(r.Context(), db.GetFileBySnapshotAndPathParams{
		IndexSnapshotID: snap.ID,
		FilePath:        filePath,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileReferencesResponse{FilePath: filePath, Items: []symbolReferenceResponse{}, SnapshotID: dbconv.PgUUIDToString(snap.ID)})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get file by path failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.ListSymbolReferencesByFileID(r.Context(), fileID)
	if err != nil {
		slog.ErrorContext(r.Context(), "list symbol references failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]symbolReferenceResponse, len(rows))
	for i, row := range rows {
		items[i] = toSymbolReferenceResponse(row)
	}

	WriteJSON(w, http.StatusOK, fileReferencesResponse{
		FilePath:   filePath,
		Items:      items,
		Total:      len(items),
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
	})
}

// HandleFileJsxUsages returns JSX component usages in a file
// (GET /v1/projects/{projectID}/files/jsx-usages?file_path=...).
func (h *ProjectHandler) HandleFileJsxUsages(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileJsxUsagesResponse{FilePath: filePath, Items: []jsxUsageResponse{}})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	fileID, err := h.db.Queries.GetFileBySnapshotAndPath(r.Context(), db.GetFileBySnapshotAndPathParams{
		IndexSnapshotID: snap.ID,
		FilePath:        filePath,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileJsxUsagesResponse{FilePath: filePath, Items: []jsxUsageResponse{}, SnapshotID: dbconv.PgUUIDToString(snap.ID)})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get file by path failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.ListJsxUsagesByFileID(r.Context(), fileID)
	if err != nil {
		slog.ErrorContext(r.Context(), "list jsx usages failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]jsxUsageResponse, len(rows))
	for i, row := range rows {
		items[i] = toJsxUsageResponse(row)
	}

	WriteJSON(w, http.StatusOK, fileJsxUsagesResponse{
		FilePath:   filePath,
		Items:      items,
		Total:      len(items),
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
	})
}

// HandleFileNetworkCalls returns detected HTTP/fetch calls in a file
// (GET /v1/projects/{projectID}/files/network-calls?file_path=...).
func (h *ProjectHandler) HandleFileNetworkCalls(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		WriteAppError(w, domain.BadRequest("file_path query param required"))
		return
	}

	snap, err := h.db.Queries.GetActiveSnapshotForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileNetworkCallsResponse{FilePath: filePath, Items: []networkCallResponse{}})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get active snapshot failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	fileID, err := h.db.Queries.GetFileBySnapshotAndPath(r.Context(), db.GetFileBySnapshotAndPathParams{
		IndexSnapshotID: snap.ID,
		FilePath:        filePath,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteJSON(w, http.StatusOK, fileNetworkCallsResponse{FilePath: filePath, Items: []networkCallResponse{}, SnapshotID: dbconv.PgUUIDToString(snap.ID)})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get file by path failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.ListNetworkCallsByFileID(r.Context(), fileID)
	if err != nil {
		slog.ErrorContext(r.Context(), "list network calls failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]networkCallResponse, len(rows))
	for i, row := range rows {
		items[i] = toNetworkCallResponse(row)
	}

	WriteJSON(w, http.StatusOK, fileNetworkCallsResponse{
		FilePath:   filePath,
		Items:      items,
		Total:      len(items),
		SnapshotID: dbconv.PgUUIDToString(snap.ID),
	})
}

