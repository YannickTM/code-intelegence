package handler

import (
	"errors"
	"net/http"
	"time"

	"myjungle/backend-api/internal/apikey"
	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/validate"

	"github.com/go-chi/chi/v5"
)

// APIKeyHandler serves API key management endpoints.
type APIKeyHandler struct {
	svc *apikey.Service
}

// NewAPIKeyHandler creates a new APIKeyHandler backed by the given service.
func NewAPIKeyHandler(svc *apikey.Service) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

func (h *APIKeyHandler) ensureSvc(w http.ResponseWriter) bool {
	if h.svc == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// ---------- Project key handlers ----------

// createKeyRequest is the JSON body for key creation (both types).
type createKeyRequest struct {
	Name      string  `json:"name"`
	Role      string  `json:"role"`
	ExpiresAt *string `json:"expires_at"` // ISO 8601
}

func (req *createKeyRequest) validate() validate.Errors {
	errs := validate.Errors{}

	if req.Name != "" {
		validate.MaxLength(req.Name, 100, "name", errs)
	}

	if req.Role == "" {
		req.Role = domain.KeyRoleRead
	}
	validate.OneOf(req.Role, []string{domain.KeyRoleRead, domain.KeyRoleWrite}, "role", errs)

	return errs
}

// parseExpiresAt parses and validates the optional expires_at string.
// Returns nil if the string is nil or empty.
func parseExpiresAt(w http.ResponseWriter, raw *string) (*time.Time, bool) {
	if raw == nil || *raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		WriteAppError(w, domain.BadRequest("expires_at must be a valid RFC 3339 timestamp"))
		return nil, false
	}
	if !t.After(time.Now()) {
		WriteAppError(w, domain.BadRequest("expires_at must be in the future"))
		return nil, false
	}
	return &t, true
}

// HandleCreateProjectKey creates a project-scoped API key (POST /v1/projects/{projectID}/keys).
func (h *APIKeyHandler) HandleCreateProjectKey(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	errs := validate.Errors{}
	projectID := validate.UUID(chi.URLParam(r, "projectID"), "project_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	var req createKeyRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if ve := req.validate(); ve.HasErrors() {
		WriteAppError(w, ve.ToAppError())
		return
	}

	expiresAt, ok := parseExpiresAt(w, req.ExpiresAt)
	if !ok {
		return
	}

	result, err := h.svc.CreateProjectKey(r.Context(), apikey.CreateKeyRequest{
		Name:      req.Name,
		Role:      req.Role,
		ExpiresAt: expiresAt,
	}, u.ID, projectID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":            result.ID,
		"key_type":      result.KeyType,
		"key_prefix":    result.KeyPrefix,
		"plaintext_key": result.PlaintextKey,
		"name":          result.Name,
		"role":          result.Role,
		"project_id":    result.ProjectID,
		"expires_at":    result.ExpiresAt,
		"created_at":    result.CreatedAt,
	})
}

// HandleListProjectKeys lists active project keys (GET /v1/projects/{projectID}/keys).
func (h *APIKeyHandler) HandleListProjectKeys(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	errs := validate.Errors{}
	projectID := validate.UUID(chi.URLParam(r, "projectID"), "project_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	keys, err := h.svc.ListProjectKeys(r.Context(), projectID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{"items": keys})
}

// HandleDeleteProjectKey soft-deletes a project key (DELETE /v1/projects/{projectID}/keys/{keyID}).
func (h *APIKeyHandler) HandleDeleteProjectKey(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	errs := validate.Errors{}
	projectID := validate.UUID(chi.URLParam(r, "projectID"), "project_id", errs)
	keyID := validate.UUID(chi.URLParam(r, "keyID"), "key_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	if err := h.svc.DeleteProjectKey(r.Context(), keyID, projectID); err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------- Personal key handlers ----------

// HandleCreatePersonalKey creates a personal API key (POST /v1/users/me/keys).
func (h *APIKeyHandler) HandleCreatePersonalKey(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	var req createKeyRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if ve := req.validate(); ve.HasErrors() {
		WriteAppError(w, ve.ToAppError())
		return
	}

	expiresAt, ok := parseExpiresAt(w, req.ExpiresAt)
	if !ok {
		return
	}

	result, err := h.svc.CreatePersonalKey(r.Context(), apikey.CreateKeyRequest{
		Name:      req.Name,
		Role:      req.Role,
		ExpiresAt: expiresAt,
	}, u.ID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":            result.ID,
		"key_type":      result.KeyType,
		"key_prefix":    result.KeyPrefix,
		"plaintext_key": result.PlaintextKey,
		"name":          result.Name,
		"role":          result.Role,
		"expires_at":    result.ExpiresAt,
		"created_at":    result.CreatedAt,
	})
}

// HandleListPersonalKeys lists active personal keys for the caller (GET /v1/users/me/keys).
func (h *APIKeyHandler) HandleListPersonalKeys(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	keys, err := h.svc.ListPersonalKeys(r.Context(), u.ID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{"items": keys})
}

// HandleDeletePersonalKey soft-deletes a personal key (DELETE /v1/users/me/keys/{keyID}).
func (h *APIKeyHandler) HandleDeletePersonalKey(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	errs := validate.Errors{}
	keyID := validate.UUID(chi.URLParam(r, "keyID"), "key_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}

	if err := h.svc.DeletePersonalKey(r.Context(), keyID, u.ID); err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// toAppError converts a service error to an *AppError for the handler.
func toAppError(err error) *domain.AppError {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return domain.ErrInternal
}
