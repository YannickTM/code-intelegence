package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/sshkey"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// SSHKeyHandler manages user-scoped SSH key endpoints.
type SSHKeyHandler struct {
	db  *postgres.DB
	svc *sshkey.Service
}

// NewSSHKeyHandler creates a new SSHKeyHandler.
func NewSSHKeyHandler(pdb *postgres.DB, svc *sshkey.Service) *SSHKeyHandler {
	return &SSHKeyHandler{db: pdb, svc: svc}
}

func (h *SSHKeyHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

func (h *SSHKeyHandler) ensureSvc(w http.ResponseWriter) bool {
	if h.svc == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// HandleCreate creates a new SSH key pair (POST /v1/ssh-keys).
func (h *SSHKeyHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}
	if !h.ensureSvc(w) {
		return
	}

	var body struct {
		Name       string `json:"name"`
		PrivateKey string `json:"private_key,omitempty"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	name := strings.TrimSpace(body.Name)
	errs := make(validate.Errors)
	validate.Required(name, "name", errs)
	validate.MaxLength(name, 100, "name", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	var pub, fingerprint, keyType string
	var encryptedPriv []byte
	var err error

	if body.PrivateKey != "" {
		pub, fingerprint, keyType, encryptedPriv, err = h.svc.CreateFromPrivateKey([]byte(body.PrivateKey))
		if err != nil {
			if errors.Is(err, sshkey.ErrPassphraseProtected) {
				WriteAppError(w, domain.BadRequest("private key is passphrase-protected"))
				return
			}
			if errors.Is(err, sshkey.ErrInvalidKey) {
				WriteAppError(w, domain.BadRequest("invalid private key format"))
				return
			}
			slog.ErrorContext(r.Context(), "sshkey: encrypt uploaded key failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
	} else {
		pub, fingerprint, keyType, encryptedPriv, err = h.svc.Create()
		if err != nil {
			slog.ErrorContext(r.Context(), "sshkey: create key pair failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
	}

	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	dbKey, err := h.db.Queries.CreateSSHKey(r.Context(), db.CreateSSHKeyParams{
		Name:                name,
		PublicKey:           pub,
		PrivateKeyEncrypted: encryptedPriv,
		KeyType:             keyType,
		Fingerprint:         fingerprint,
		CreatedBy:           userID,
	})
	if err != nil {
		if postgres.IsUniqueViolation(err) {
			WriteAppError(w, domain.Conflict("ssh key with this fingerprint already exists"))
			return
		}
		slog.ErrorContext(r.Context(), "sshkey: insert failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusCreated, dbconv.DBSSHKeyToDomain(dbKey))
}

// HandleList lists all SSH keys for the current user (GET /v1/ssh-keys).
func (h *SSHKeyHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	rows, err := h.db.Queries.ListSSHKeys(r.Context(), userID)
	if err != nil {
		slog.ErrorContext(r.Context(), "sshkey: list failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]domain.SSHKey, 0, len(rows))
	for _, row := range rows {
		items = append(items, dbconv.DBSSHKeyToDomain(row))
	}

	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleGet returns an SSH key by ID (GET /v1/ssh-keys/{keyID}).
func (h *SSHKeyHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	keyIDStr := chi.URLParam(r, "keyID")
	errs := make(validate.Errors)
	validate.UUID(keyIDStr, "ssh_key_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	keyID, err := dbconv.StringToPgUUID(keyIDStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}
	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	dbKey, err := h.db.Queries.GetSSHKey(r.Context(), db.GetSSHKeyParams{
		ID:        keyID,
		CreatedBy: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("ssh key not found"))
			return
		}
		slog.ErrorContext(r.Context(), "sshkey: get failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, dbconv.DBSSHKeyToDomain(dbKey))
}

// HandleListProjects lists projects assigned to an SSH key
// (GET /v1/ssh-keys/{keyID}/projects).
func (h *SSHKeyHandler) HandleListProjects(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	keyIDStr := chi.URLParam(r, "keyID")
	errs := make(validate.Errors)
	validate.UUID(keyIDStr, "ssh_key_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	keyID, err := dbconv.StringToPgUUID(keyIDStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}
	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Verify key ownership first.
	_, err = h.db.Queries.GetSSHKey(r.Context(), db.GetSSHKeyParams{
		ID:        keyID,
		CreatedBy: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.NotFound("ssh key not found"))
			return
		}
		slog.ErrorContext(r.Context(), "sshkey: get for list-projects failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	projects, err := h.db.Queries.ListProjectsBySSHKey(r.Context(), db.ListProjectsBySSHKeyParams{
		SshKeyID:  keyID,
		CreatedBy: userID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "sshkey: list projects failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	items := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		m := map[string]any{
			"id":             dbconv.PgUUIDToString(p.ID),
			"name":           p.Name,
			"repo_url":       p.RepoUrl,
			"default_branch": p.DefaultBranch,
			"status":         p.Status,
		}
		if p.CreatedBy.Valid {
			m["created_by"] = dbconv.PgUUIDToString(p.CreatedBy)
		}
		if p.CreatedAt.Valid {
			m["created_at"] = p.CreatedAt.Time
		}
		if p.UpdatedAt.Valid {
			m["updated_at"] = p.UpdatedAt.Time
		}
		items = append(items, m)
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// HandleRetire retires an SSH key (POST /v1/ssh-keys/{keyID}/retire).
func (h *SSHKeyHandler) HandleRetire(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}
	if !h.ensureDB(w) {
		return
	}

	keyIDStr := chi.URLParam(r, "keyID")
	errs := make(validate.Errors)
	validate.UUID(keyIDStr, "ssh_key_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	keyID, err := dbconv.StringToPgUUID(keyIDStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}
	userID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Atomic retire: only succeeds if the key exists, belongs to the user,
	// and has no active project assignments.
	dbKey, err := h.db.Queries.RetireSSHKeyIfNoAssignments(r.Context(), db.RetireSSHKeyIfNoAssignmentsParams{
		ID:        keyID,
		CreatedBy: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish 404 (not found / not owned) from 409 (has assignments).
			_, getErr := h.db.Queries.GetSSHKey(r.Context(), db.GetSSHKeyParams{
				ID:        keyID,
				CreatedBy: userID,
			})
			if getErr != nil {
				if errors.Is(getErr, pgx.ErrNoRows) {
					WriteAppError(w, domain.NotFound("ssh key not found"))
					return
				}
				slog.ErrorContext(r.Context(), "sshkey: get for retire fallback failed", slog.String("key_id", keyIDStr), slog.Any("error", getErr))
				WriteAppError(w, domain.ErrInternal)
				return
			}
			// Key exists and belongs to user → must have active assignments.
			count, countErr := h.db.Queries.CountActiveAssignmentsByKey(r.Context(), keyID)
			if countErr != nil {
				slog.ErrorContext(r.Context(), "sshkey: count assignments failed", slog.String("key_id", keyIDStr), slog.Any("error", countErr))
				WriteAppError(w, domain.ErrInternal)
				return
			}
			if count > 0 {
				WriteAppError(w, domain.Conflict(
					fmt.Sprintf("key is still assigned to %d project(s)", count)))
			} else {
				WriteAppError(w, domain.Conflict("key has active project assignments"))
			}
			return
		}
		slog.ErrorContext(r.Context(), "sshkey: retire failed", slog.String("key_id", keyIDStr), slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, dbconv.DBSSHKeyToDomain(dbKey))
}
