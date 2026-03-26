package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/embedding"
	"myjungle/backend-api/internal/validate"

	"github.com/go-chi/chi/v5"
)

const maxEmbeddingTestBodyBytes = 64 * 1024

// EmbeddingHandler serves embedding provider configuration endpoints.
type EmbeddingHandler struct {
	svc *embedding.Service
}

// NewEmbeddingHandler creates a new EmbeddingHandler backed by the given service.
func NewEmbeddingHandler(svc *embedding.Service) *EmbeddingHandler {
	return &EmbeddingHandler{svc: svc}
}

func (h *EmbeddingHandler) ensureSvc(w http.ResponseWriter) bool {
	if h.svc == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

type embeddingGlobalUpdateRequest struct {
	Name                  string          `json:"name"`
	Provider              string          `json:"provider"`
	EndpointURL           string          `json:"endpoint_url"`
	Model                 string          `json:"model"`
	Dimensions            int             `json:"dimensions"`
	MaxTokens             int             `json:"max_tokens"`
	Settings              map[string]any  `json:"settings"`
	Credentials           json.RawMessage `json:"credentials"`
	IsAvailableToProjects *bool           `json:"is_available_to_projects"`
}

func (req *embeddingGlobalUpdateRequest) validate() validate.Errors {
	errs := validate.Errors{}
	req.Name = validate.Required(req.Name, "name", errs)
	req.Provider = validate.Required(req.Provider, "provider", errs)
	req.EndpointURL = validate.Required(req.EndpointURL, "endpoint_url", errs)
	req.EndpointURL = validateProviderEndpointURL(req.EndpointURL, "endpoint_url", errs)
	req.Model = validate.Required(req.Model, "model", errs)
	validate.MinMax(req.Dimensions, 1, 65536, "dimensions", errs)
	validate.MinMax(req.MaxTokens, 1, 131072, "max_tokens", errs)
	return errs
}

type embeddingGlobalPatchRequest struct {
	Name                  *string         `json:"name"`
	Provider              *string         `json:"provider"`
	EndpointURL           *string         `json:"endpoint_url"`
	Model                 *string         `json:"model"`
	Dimensions            *int            `json:"dimensions"`
	MaxTokens             *int            `json:"max_tokens"`
	Settings              map[string]any  `json:"settings"`
	Credentials           json.RawMessage `json:"credentials"`
	IsAvailableToProjects *bool           `json:"is_available_to_projects"`
}

func (req *embeddingGlobalPatchRequest) validate() validate.Errors {
	errs := validate.Errors{}
	if req.Name != nil {
		*req.Name = validate.Required(*req.Name, "name", errs)
	}
	if req.Provider != nil {
		*req.Provider = validate.Required(*req.Provider, "provider", errs)
	}
	if req.EndpointURL != nil {
		*req.EndpointURL = validate.Required(*req.EndpointURL, "endpoint_url", errs)
		*req.EndpointURL = validateProviderEndpointURL(*req.EndpointURL, "endpoint_url", errs)
	}
	if req.Model != nil {
		*req.Model = validate.Required(*req.Model, "model", errs)
	}
	if req.Dimensions != nil {
		validate.MinMax(*req.Dimensions, 1, 65536, "dimensions", errs)
	}
	if req.MaxTokens != nil {
		validate.MinMax(*req.MaxTokens, 1, 131072, "max_tokens", errs)
	}
	return errs
}

func (req *embeddingGlobalPatchRequest) hasFields() bool {
	return req.Name != nil || req.Provider != nil || req.EndpointURL != nil ||
		req.Model != nil || req.Dimensions != nil || req.MaxTokens != nil ||
		req.Settings != nil || len(req.Credentials) > 0 || req.IsAvailableToProjects != nil
}

// HandleGlobalGet returns all global embedding provider configs.
func (h *EmbeddingHandler) HandleGlobalGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	items, err := h.svc.ListGlobalConfigs(r.Context())
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleGlobalUpdate creates or updates the global embedding default config.
func (h *EmbeddingHandler) HandleGlobalUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	var req embeddingGlobalUpdateRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(); errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}
	credentials, err := parseCredentialsUpdate(req.Credentials)
	if err != nil {
		WriteAppError(w, domain.ValidationError(map[string]string{
			"credentials": "credentials must be a JSON object, null, or omitted",
		}))
		return
	}

	cfg, err := h.svc.UpdateGlobalConfig(r.Context(), embedding.GlobalUpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Dimensions:  req.Dimensions,
		MaxTokens:   req.MaxTokens,
		Settings:    req.Settings,
		Credentials: embedding.CredentialsUpdate{
			Set:   credentials.set,
			Clear: credentials.clear,
			Data:  credentials.data,
		},
		IsAvailableToProjects: req.IsAvailableToProjects,
	})
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"config": cfg})
}

// HandleGlobalTest tests connectivity for the global embedding configuration.
func (h *EmbeddingHandler) HandleGlobalTest(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxEmbeddingTestBodyBytes+1))
	if err != nil {
		WriteAppError(w, domain.BadRequest("failed to read request body"))
		return
	}
	if len(body) > maxEmbeddingTestBodyBytes {
		WriteAppError(w, domain.ErrPayloadTooLarge)
		return
	}

	var cfg *domain.EmbeddingProviderConfig
	trimmed := bytes.TrimSpace(body)
	hasFields := false
	if len(trimmed) > 0 {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			WriteAppError(w, domain.BadRequest("invalid JSON body"))
			return
		} else {
			hasFields = len(probe) > 0
		}
	}

	if hasFields {
		var req embeddingConnectivityRequest
		if err := json.Unmarshal(trimmed, &req); err != nil {
			WriteAppError(w, domain.BadRequest("invalid JSON body"))
			return
		}
		if errs := req.validate(); errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		cfg = &domain.EmbeddingProviderConfig{
			Provider:    req.Provider,
			EndpointURL: req.EndpointURL,
			Model:       req.Model,
			Dimensions:  req.Dimensions,
		}
	}

	result, err := h.svc.TestGlobalConnectivity(r.Context(), cfg)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// HandleCreateGlobalConfig creates a new non-default global embedding config.
func (h *EmbeddingHandler) HandleCreateGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	var req embeddingGlobalUpdateRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(); errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}
	credentials, err := parseCredentialsUpdate(req.Credentials)
	if err != nil {
		WriteAppError(w, domain.ValidationError(map[string]string{
			"credentials": "credentials must be a JSON object, null, or omitted",
		}))
		return
	}

	cfg, err := h.svc.CreateAdditionalGlobalConfig(r.Context(), embedding.GlobalUpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Dimensions:  req.Dimensions,
		MaxTokens:   req.MaxTokens,
		Settings:    req.Settings,
		Credentials: embedding.CredentialsUpdate{
			Set:   credentials.set,
			Clear: credentials.clear,
			Data:  credentials.data,
		},
		IsAvailableToProjects: req.IsAvailableToProjects,
	})
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"config": cfg})
}

// HandleUpdateGlobalConfigByID partially updates a specific global embedding config.
func (h *EmbeddingHandler) HandleUpdateGlobalConfigByID(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	configID, ok := configIDParam(w, r)
	if !ok {
		return
	}

	var req embeddingGlobalPatchRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if !req.hasFields() {
		WriteAppError(w, domain.BadRequest("at least one field must be provided"))
		return
	}
	if errs := req.validate(); errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}
	credentials, err := parseCredentialsUpdate(req.Credentials)
	if err != nil {
		WriteAppError(w, domain.ValidationError(map[string]string{
			"credentials": "credentials must be a JSON object, null, or omitted",
		}))
		return
	}

	cfg, err := h.svc.UpdateGlobalConfigByID(r.Context(), configID, embedding.GlobalPatchRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Dimensions:  req.Dimensions,
		MaxTokens:   req.MaxTokens,
		Settings:    req.Settings,
		Credentials: embedding.CredentialsUpdate{
			Set:   credentials.set,
			Clear: credentials.clear,
			Data:  credentials.data,
		},
		IsAvailableToProjects: req.IsAvailableToProjects,
	})
	if err != nil {
		var appErr *domain.AppError
		if !errors.As(err, &appErr) {
			slog.Error("UpdateGlobalConfigByID failed", "config_id", configID, "error", err)
		}
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"config": cfg})
}

// HandleDeleteGlobalConfig deactivates a specific global embedding config.
func (h *EmbeddingHandler) HandleDeleteGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	configID, ok := configIDParam(w, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteGlobalConfig(r.Context(), configID); err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"message":   "provider config deactivated",
		"config_id": configID,
	})
}

// HandlePromoteGlobalConfigToDefault promotes a specific embedding config to default.
func (h *EmbeddingHandler) HandlePromoteGlobalConfigToDefault(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	configID, ok := configIDParam(w, r)
	if !ok {
		return
	}

	cfg, err := h.svc.PromoteToDefault(r.Context(), configID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "provider promoted to default",
		"config":  cfg,
	})
}

// HandleTestGlobalConfigByID tests connectivity for a specific global embedding config.
func (h *EmbeddingHandler) HandleTestGlobalConfigByID(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	configID, ok := configIDParam(w, r)
	if !ok {
		return
	}

	result, err := h.svc.TestGlobalConnectivityByID(r.Context(), configID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

type embeddingSettingRequest struct {
	Mode           string          `json:"mode"`
	GlobalConfigID string          `json:"global_config_id"`
	Name           string          `json:"name"`
	Provider       string          `json:"provider"`
	EndpointURL    string          `json:"endpoint_url"`
	Model          string          `json:"model"`
	Dimensions     int             `json:"dimensions"`
	MaxTokens      int             `json:"max_tokens"`
	Settings       map[string]any  `json:"settings"`
	Credentials    json.RawMessage `json:"credentials"`
}

type embeddingConnectivityRequest struct {
	Provider    string `json:"provider"`
	EndpointURL string `json:"endpoint_url"`
	Model       string `json:"model"`
	Dimensions  int    `json:"dimensions"`
}

func (req *embeddingSettingRequest) validate() validate.Errors {
	errs := validate.Errors{}
	req.Mode = strings.ToLower(validate.Required(req.Mode, "mode", errs))
	validate.OneOf(req.Mode, []string{"global", "custom"}, "mode", errs)

	switch req.Mode {
	case "global":
		req.GlobalConfigID = validate.UUID(req.GlobalConfigID, "global_config_id", errs)
	case "custom":
		req.Name = validate.Required(req.Name, "name", errs)
		req.Provider = validate.Required(req.Provider, "provider", errs)
		req.EndpointURL = validate.Required(req.EndpointURL, "endpoint_url", errs)
		req.EndpointURL = validateProviderEndpointURL(req.EndpointURL, "endpoint_url", errs)
		req.Model = validate.Required(req.Model, "model", errs)
		validate.MinMax(req.Dimensions, 1, 65536, "dimensions", errs)
		validate.MinMax(req.MaxTokens, 1, 131072, "max_tokens", errs)
	}
	return errs
}

func (req *embeddingConnectivityRequest) validate() validate.Errors {
	errs := validate.Errors{}
	req.Provider = validate.Required(req.Provider, "provider", errs)
	req.EndpointURL = validate.Required(req.EndpointURL, "endpoint_url", errs)
	req.EndpointURL = validateProviderEndpointURL(req.EndpointURL, "endpoint_url", errs)
	req.Model = validate.Required(req.Model, "model", errs)
	if req.Dimensions != 0 {
		validate.MinMax(req.Dimensions, 1, 65536, "dimensions", errs)
	}
	return errs
}

// HandleProjectAvailable lists selectable global embedding configs for the project.
func (h *EmbeddingHandler) HandleProjectAvailable(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	if !validateProjectID(w, r) {
		return
	}
	items, err := h.svc.ListAvailableGlobalConfigs(r.Context())
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleProjectGet returns the project's current embedding setting mode and effective config.
func (h *EmbeddingHandler) HandleProjectGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}
	setting, err := h.svc.GetProjectSetting(r.Context(), projectID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, setting)
}

// HandleProjectUpdate updates the project's embedding setting.
func (h *EmbeddingHandler) HandleProjectUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}

	var req embeddingSettingRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(); errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}
	credentials, err := parseCredentialsUpdate(req.Credentials)
	if err != nil {
		WriteAppError(w, domain.ValidationError(map[string]string{
			"credentials": "credentials must be a JSON object, null, or omitted",
		}))
		return
	}

	setting, err := h.svc.UpdateProjectSetting(r.Context(), projectID, embedding.UpdateRequest{
		Mode:           req.Mode,
		GlobalConfigID: req.GlobalConfigID,
		Name:           req.Name,
		Provider:       req.Provider,
		EndpointURL:    req.EndpointURL,
		Model:          req.Model,
		Dimensions:     req.Dimensions,
		MaxTokens:      req.MaxTokens,
		Settings:       req.Settings,
		Credentials: embedding.CredentialsUpdate{
			Set:   credentials.set,
			Clear: credentials.clear,
			Data:  credentials.data,
		},
	})
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}

	WriteJSON(w, http.StatusOK, setting)
}

// HandleProjectDelete resets the project's embedding setting to default mode.
func (h *EmbeddingHandler) HandleProjectDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.ResetProjectSetting(r.Context(), projectID); err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Project setting cleared. This project will now use the active global default configuration.",
	})
}

// HandleProjectTest tests embedding provider connectivity.
func (h *EmbeddingHandler) HandleProjectTest(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxEmbeddingTestBodyBytes+1))
	if err != nil {
		WriteAppError(w, domain.BadRequest("failed to read request body"))
		return
	}
	if len(body) > maxEmbeddingTestBodyBytes {
		WriteAppError(w, domain.ErrPayloadTooLarge)
		return
	}

	var cfg *domain.EmbeddingProviderConfig
	trimmed := bytes.TrimSpace(body)
	hasFields := false
	if len(trimmed) > 0 {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			WriteAppError(w, domain.BadRequest("invalid JSON body"))
			return
		} else {
			hasFields = len(probe) > 0
		}
	}

	if hasFields {
		var req embeddingConnectivityRequest
		if err := json.Unmarshal(trimmed, &req); err != nil {
			WriteAppError(w, domain.BadRequest("invalid JSON body"))
			return
		}
		if errs := req.validate(); errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		cfg = &domain.EmbeddingProviderConfig{
			Provider:    req.Provider,
			EndpointURL: req.EndpointURL,
			Model:       req.Model,
			Dimensions:  req.Dimensions,
		}
	} else {
		resolved, err := h.svc.GetResolvedConfig(r.Context(), projectID)
		if err != nil {
			WriteAppError(w, toAppError(err))
			return
		}
		if resolved == nil {
			WriteAppError(w, domain.NotFound("no embedding configuration found for this project"))
			return
		}
		cfg = &resolved.Config
	}

	result, err := h.svc.TestConnectivity(r.Context(), cfg)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// HandleProjectResolved returns the effective embedding config for a project.
func (h *EmbeddingHandler) HandleProjectResolved(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}
	resolved, err := h.svc.GetResolvedConfig(r.Context(), projectID)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	if resolved == nil {
		WriteAppError(w, domain.NotFound("no embedding configuration found for this project"))
		return
	}
	WriteJSON(w, http.StatusOK, resolved)
}

func projectIDParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	errs := validate.Errors{}
	projectID := validate.UUID(chi.URLParam(r, "projectID"), "project_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return "", false
	}
	return projectID, true
}

func validateProjectID(w http.ResponseWriter, r *http.Request) bool {
	_, ok := projectIDParam(w, r)
	return ok
}

func configIDParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	errs := validate.Errors{}
	configID := validate.UUID(chi.URLParam(r, "configId"), "config_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return "", false
	}
	return configID, true
}
