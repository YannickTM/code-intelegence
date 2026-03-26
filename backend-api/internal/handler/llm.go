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
	"myjungle/backend-api/internal/llm"
	"myjungle/backend-api/internal/validate"
)

const maxLLMTestBodyBytes = 64 * 1024

// LLMHandler serves LLM provider configuration endpoints.
type LLMHandler struct {
	svc *llm.Service
}

// NewLLMHandler creates a new LLMHandler backed by the given service.
func NewLLMHandler(svc *llm.Service) *LLMHandler {
	return &LLMHandler{svc: svc}
}

func (h *LLMHandler) ensureSvc(w http.ResponseWriter) bool {
	if h.svc == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

type llmGlobalUpdateRequest struct {
	Name                  string          `json:"name"`
	Provider              string          `json:"provider"`
	EndpointURL           string          `json:"endpoint_url"`
	Model                 string          `json:"model"`
	Settings              map[string]any  `json:"settings"`
	Credentials           json.RawMessage `json:"credentials"`
	IsAvailableToProjects *bool           `json:"is_available_to_projects"`
}

func (req *llmGlobalUpdateRequest) validate() validate.Errors {
	errs := validate.Errors{}
	req.Name = validate.Required(req.Name, "name", errs)
	req.Provider = validate.Required(req.Provider, "provider", errs)
	req.EndpointURL = validate.Required(req.EndpointURL, "endpoint_url", errs)
	req.EndpointURL = validateProviderEndpointURL(req.EndpointURL, "endpoint_url", errs)
	return errs
}

type llmGlobalPatchRequest struct {
	Name                  *string         `json:"name"`
	Provider              *string         `json:"provider"`
	EndpointURL           *string         `json:"endpoint_url"`
	Model                 *string         `json:"model"`
	Settings              map[string]any  `json:"settings"`
	Credentials           json.RawMessage `json:"credentials"`
	IsAvailableToProjects *bool           `json:"is_available_to_projects"`
}

func (req *llmGlobalPatchRequest) validate() validate.Errors {
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
	return errs
}

func (req *llmGlobalPatchRequest) hasFields() bool {
	return req.Name != nil || req.Provider != nil || req.EndpointURL != nil ||
		req.Model != nil || req.Settings != nil || len(req.Credentials) > 0 ||
		req.IsAvailableToProjects != nil
}

// HandleGlobalGet returns all global LLM provider configs.
func (h *LLMHandler) HandleGlobalGet(w http.ResponseWriter, r *http.Request) {
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

// HandleGlobalUpdate creates or updates the global LLM default config.
func (h *LLMHandler) HandleGlobalUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	var req llmGlobalUpdateRequest
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

	cfg, err := h.svc.UpdateGlobalConfig(r.Context(), llm.GlobalUpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Settings:    req.Settings,
		Credentials: llm.CredentialsUpdate{
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

// HandleGlobalTest tests connectivity for the global LLM configuration.
func (h *LLMHandler) HandleGlobalTest(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxLLMTestBodyBytes+1))
	if err != nil {
		WriteAppError(w, domain.BadRequest("failed to read request body"))
		return
	}
	if len(body) > maxLLMTestBodyBytes {
		WriteAppError(w, domain.ErrPayloadTooLarge)
		return
	}

	var cfg *domain.LLMProviderConfig
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
		var req llmConnectivityRequest
		if err := json.Unmarshal(trimmed, &req); err != nil {
			WriteAppError(w, domain.BadRequest("invalid JSON body"))
			return
		}
		if errs := req.validate(); errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		cfg = &domain.LLMProviderConfig{
			Provider:    req.Provider,
			EndpointURL: req.EndpointURL,
			Model:       req.Model,
		}
	}

	result, err := h.svc.TestGlobalConnectivity(r.Context(), cfg)
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// HandleCreateGlobalConfig creates a new non-default global LLM config.
func (h *LLMHandler) HandleCreateGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}

	var req llmGlobalUpdateRequest
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

	cfg, err := h.svc.CreateAdditionalGlobalConfig(r.Context(), llm.GlobalUpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Settings:    req.Settings,
		Credentials: llm.CredentialsUpdate{
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

// HandleUpdateGlobalConfigByID partially updates a specific global LLM config.
func (h *LLMHandler) HandleUpdateGlobalConfigByID(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	configID, ok := configIDParam(w, r)
	if !ok {
		return
	}

	var req llmGlobalPatchRequest
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

	cfg, err := h.svc.UpdateGlobalConfigByID(r.Context(), configID, llm.GlobalPatchRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Settings:    req.Settings,
		Credentials: llm.CredentialsUpdate{
			Set:   credentials.set,
			Clear: credentials.clear,
			Data:  credentials.data,
		},
		IsAvailableToProjects: req.IsAvailableToProjects,
	})
	if err != nil {
		var appErr *domain.AppError
		if !errors.As(err, &appErr) {
			slog.Error("UpdateGlobalConfigByID (LLM) failed", "config_id", configID, "error", err)
		}
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"config": cfg})
}

// HandleDeleteGlobalConfig deactivates a specific global LLM config.
func (h *LLMHandler) HandleDeleteGlobalConfig(w http.ResponseWriter, r *http.Request) {
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

// HandlePromoteGlobalConfigToDefault promotes a specific LLM config to default.
func (h *LLMHandler) HandlePromoteGlobalConfigToDefault(w http.ResponseWriter, r *http.Request) {
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

// HandleTestGlobalConfigByID tests connectivity for a specific global LLM config.
func (h *LLMHandler) HandleTestGlobalConfigByID(w http.ResponseWriter, r *http.Request) {
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

type llmSettingRequest struct {
	Mode           string          `json:"mode"`
	GlobalConfigID string          `json:"global_config_id"`
	Name           string          `json:"name"`
	Provider       string          `json:"provider"`
	EndpointURL    string          `json:"endpoint_url"`
	Model          string          `json:"model"`
	Settings       map[string]any  `json:"settings"`
	Credentials    json.RawMessage `json:"credentials"`
}

type llmConnectivityRequest struct {
	Provider    string `json:"provider"`
	EndpointURL string `json:"endpoint_url"`
	Model       string `json:"model"`
}

func (req *llmSettingRequest) validate() validate.Errors {
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
	}
	return errs
}

func (req *llmConnectivityRequest) validate() validate.Errors {
	errs := validate.Errors{}
	req.Provider = validate.Required(req.Provider, "provider", errs)
	req.EndpointURL = validate.Required(req.EndpointURL, "endpoint_url", errs)
	req.EndpointURL = validateProviderEndpointURL(req.EndpointURL, "endpoint_url", errs)
	return errs
}

// HandleProjectAvailable lists selectable global LLM configs for the project.
func (h *LLMHandler) HandleProjectAvailable(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	if _, ok := projectIDParam(w, r); !ok {
		return
	}
	items, err := h.svc.ListAvailableGlobalConfigs(r.Context())
	if err != nil {
		WriteAppError(w, toAppError(err))
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleProjectGet returns the project's current LLM setting mode and effective config.
func (h *LLMHandler) HandleProjectGet(w http.ResponseWriter, r *http.Request) {
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

// HandleProjectUpdate updates the project's LLM setting.
func (h *LLMHandler) HandleProjectUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}

	var req llmSettingRequest
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

	setting, err := h.svc.UpdateProjectSetting(r.Context(), projectID, llm.UpdateRequest{
		Mode:           req.Mode,
		GlobalConfigID: req.GlobalConfigID,
		Name:           req.Name,
		Provider:       req.Provider,
		EndpointURL:    req.EndpointURL,
		Model:          req.Model,
		Settings:       req.Settings,
		Credentials: llm.CredentialsUpdate{
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

// HandleProjectDelete resets the project's LLM setting to default mode.
func (h *LLMHandler) HandleProjectDelete(w http.ResponseWriter, r *http.Request) {
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

// HandleProjectTest tests LLM provider connectivity.
func (h *LLMHandler) HandleProjectTest(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSvc(w) {
		return
	}
	projectID, ok := projectIDParam(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxLLMTestBodyBytes+1))
	if err != nil {
		WriteAppError(w, domain.BadRequest("failed to read request body"))
		return
	}
	if len(body) > maxLLMTestBodyBytes {
		WriteAppError(w, domain.ErrPayloadTooLarge)
		return
	}

	var cfg *domain.LLMProviderConfig
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
		var req llmConnectivityRequest
		if err := json.Unmarshal(trimmed, &req); err != nil {
			WriteAppError(w, domain.BadRequest("invalid JSON body"))
			return
		}
		if errs := req.validate(); errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		cfg = &domain.LLMProviderConfig{
			Provider:    req.Provider,
			EndpointURL: req.EndpointURL,
			Model:       req.Model,
		}
	} else {
		resolved, err := h.svc.GetResolvedConfig(r.Context(), projectID)
		if err != nil {
			WriteAppError(w, toAppError(err))
			return
		}
		if resolved == nil {
			WriteAppError(w, domain.NotFound("no llm configuration found for this project"))
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

// HandleProjectResolved returns the effective LLM config for a project.
func (h *LLMHandler) HandleProjectResolved(w http.ResponseWriter, r *http.Request) {
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
		WriteAppError(w, domain.NotFound("no llm configuration found for this project"))
		return
	}
	WriteJSON(w, http.StatusOK, resolved)
}
