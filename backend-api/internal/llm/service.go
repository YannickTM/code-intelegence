package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/providers"
	"myjungle/backend-api/internal/providersetting"
	"myjungle/backend-api/internal/secrets"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CredentialsUpdate is an alias for the shared type.
type CredentialsUpdate = providersetting.CredentialsUpdate

// UpdateRequest is the input for creating or updating a project LLM setting.
type UpdateRequest struct {
	Mode           string
	GlobalConfigID string
	Name           string
	Provider       string
	EndpointURL    string
	Model          string
	Settings       map[string]any
	Credentials    CredentialsUpdate
}

// GlobalUpdateRequest is the input for creating or updating the global LLM default.
type GlobalUpdateRequest struct {
	Name                  string
	Provider              string
	EndpointURL           string
	Model                 string
	Settings              map[string]any
	Credentials           CredentialsUpdate
	IsAvailableToProjects *bool // defaults to true if nil
}

// GlobalPatchRequest is the input for partial-updating a specific global LLM config.
// All fields are pointers; nil means "keep current value".
type GlobalPatchRequest struct {
	Name                  *string
	Provider              *string
	EndpointURL           *string
	Model                 *string
	Settings              map[string]any // nil means keep existing
	Credentials           CredentialsUpdate
	IsAvailableToProjects *bool
}

type resolvedState struct {
	Mode           string
	Source         string
	GlobalConfigID string
	Config         domain.LLMProviderConfig
}

// Service provides LLM provider configuration business logic.
type Service struct {
	db         *postgres.DB
	httpClient *http.Client
	secretKey  []byte
}

// NewService creates a new LLM service.
// It returns nil, nil when the database is not configured.
func NewService(database *postgres.DB, httpClient *http.Client, encryptionSecret string) (*Service, error) {
	if database == nil {
		return nil, nil
	}
	if database.Queries == nil {
		return nil, fmt.Errorf("llm: database queries not initialized")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if strings.TrimSpace(encryptionSecret) == "" {
		return nil, fmt.Errorf("llm: encryption secret is required")
	}
	return &Service{
		db:         database,
		httpClient: httpClient,
		secretKey:  secrets.DeriveKey(encryptionSecret),
	}, nil
}

// BootstrapDefaultConfig seeds the default global LLM config if none exists yet.
func BootstrapDefaultConfig(ctx context.Context, database *postgres.DB, defaults config.LLMDefaults) error {
	if database == nil || database.Queries == nil {
		return nil
	}
	req := UpdateRequest{
		Name:        config.DefaultPlatformProviderConfigName,
		Provider:    defaults.Provider,
		EndpointURL: defaults.EndpointURL,
		Model:       defaults.Model,
		Settings:    map[string]any{},
	}
	if err := validateCustomUpdateRequest(&req); err != nil {
		return fmt.Errorf("bootstrap llm default: %w", err)
	}
	model := pgtype.Text{}
	if req.Model != "" {
		model = pgtype.Text{String: req.Model, Valid: true}
	}
	err := database.Queries.CreateLLMDefaultProviderConfigIfMissing(ctx, db.CreateLLMDefaultProviderConfigIfMissingParams{
		Name:         req.Name,
		Provider:     req.Provider,
		EndpointUrl:  req.EndpointURL,
		Model:        model,
		SettingsJson: []byte(`{}`),
	})
	if err != nil && postgres.IsUniqueViolation(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create llm default provider config: %w", err)
	}
	return nil
}

// GetProjectSetting returns the project's current LLM selection mode and effective config.
func (s *Service) GetProjectSetting(ctx context.Context, projectID string) (*domain.ProjectProviderSetting, error) {
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return nil, domain.BadRequest("invalid project ID")
	}
	state, err := s.resolveState(ctx, projectUUID)
	if err != nil {
		return nil, err
	}
	return &domain.ProjectProviderSetting{
		Mode:           state.Mode,
		GlobalConfigID: state.GlobalConfigID,
		Config:         state.Config,
	}, nil
}

// ListAvailableGlobalConfigs returns active global LLM configs that projects may select.
func (s *Service) ListAvailableGlobalConfigs(ctx context.Context) ([]domain.LLMProviderConfig, error) {
	rows, err := s.db.Queries.ListAvailableGlobalLLMProviderConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available llm configs: %w", err)
	}
	items := make([]domain.LLMProviderConfig, 0, len(rows))
	for _, row := range rows {
		item, err := dbconv.DBAvailableLLMProviderConfigToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("decode available llm config: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// UpdateProjectSetting changes the project's LLM selection.
func (s *Service) UpdateProjectSetting(ctx context.Context, projectID string, req UpdateRequest) (*domain.ProjectProviderSetting, error) {
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return nil, domain.BadRequest("invalid project ID")
	}

	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	switch req.Mode {
	case "global":
		return s.selectGlobalConfig(ctx, projectUUID, req.GlobalConfigID)
	case "custom":
		cfg, err := s.upsertCustomConfig(ctx, projectUUID, req)
		if err != nil {
			return nil, err
		}
		return &domain.ProjectProviderSetting{
			Mode:   "custom",
			Config: *cfg,
		}, nil
	default:
		return nil, domain.BadRequest("mode must be one of: global, custom")
	}
}

// ResetProjectSetting clears project-specific selection so the default global config is used.
func (s *Service) ResetProjectSetting(ctx context.Context, projectID string) error {
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return domain.BadRequest("invalid project ID")
	}

	return s.db.WithTx(ctx, func(q *db.Queries) error {
		if err := providersetting.LockProjectRow(ctx, q, projectUUID); err != nil {
			return err
		}
		if _, err := q.GetProjectProviderSelections(ctx, projectUUID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("project not found")
			}
			return fmt.Errorf("get project provider selections: %w", err)
		}
		if err := q.ClearSelectedLLMGlobalConfig(ctx, projectUUID); err != nil {
			return fmt.Errorf("clear selected llm config: %w", err)
		}
		if err := q.DeactivateProjectLLMProviderConfigs(ctx, projectUUID); err != nil {
			return fmt.Errorf("deactivate project llm configs: %w", err)
		}
		return nil
	})
}

// GetResolvedConfig returns the effective LLM config for the project.
func (s *Service) GetResolvedConfig(ctx context.Context, projectID string) (*domain.ResolvedLLMProviderSetting, error) {
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return nil, domain.BadRequest("invalid project ID")
	}
	state, err := s.resolveState(ctx, projectUUID)
	if err != nil {
		return nil, err
	}
	return &domain.ResolvedLLMProviderSetting{
		Source: state.Source,
		Config: state.Config,
	}, nil
}

// TestConnectivity tests connectivity to an LLM provider.
func (s *Service) TestConnectivity(ctx context.Context, cfg *domain.LLMProviderConfig) (*providers.ConnectivityResult, error) {
	if cfg == nil {
		return nil, domain.BadRequest("missing llm configuration to test")
	}
	result, err := providers.TestLLMConnectivity(ctx, s.httpClient, cfg)
	if err != nil {
		return nil, domain.BadRequest(err.Error())
	}
	return &result, nil
}

func (s *Service) resolveState(ctx context.Context, projectUUID pgtype.UUID) (*resolvedState, error) {
	if custom, found, err := s.getActiveCustomConfig(ctx, projectUUID); err != nil {
		return nil, err
	} else if found {
		cfg, err := dbconv.DBLLMProviderConfigToDomain(custom)
		if err != nil {
			return nil, fmt.Errorf("decode active project llm config: %w", err)
		}
		return &resolvedState{
			Mode:   "custom",
			Source: "custom",
			Config: cfg,
		}, nil
	}

	selections, err := s.db.Queries.GetProjectProviderSelections(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("project not found")
		}
		return nil, fmt.Errorf("get project provider selections: %w", err)
	}

	if selections.SelectedLlmGlobalConfigID.Valid {
		row, err := s.db.Queries.GetLLMProviderConfigByID(ctx, selections.SelectedLlmGlobalConfigID)
		if err == nil && !row.ProjectID.Valid && row.IsActive {
			cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
			if err != nil {
				return nil, fmt.Errorf("decode selected llm global config: %w", err)
			}
			return &resolvedState{
				Mode:           "global",
				Source:         "global",
				GlobalConfigID: dbconv.PgUUIDToString(selections.SelectedLlmGlobalConfigID),
				Config:         cfg,
			}, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get selected llm config: %w", err)
		}
		// Selected global config is stale (deactivated, deleted, or project-owned);
		// fall through to the default global config.
		slog.WarnContext(ctx, "llm: stale global config selection, falling back to default", slog.String("project_id", dbconv.PgUUIDToString(projectUUID)))
	}

	row, err := s.db.Queries.GetDefaultGlobalLLMProviderConfig(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("no llm configuration found for this project")
		}
		return nil, fmt.Errorf("get default llm config: %w", err)
	}
	cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode default llm config: %w", err)
	}
	return &resolvedState{
		Mode:   "default",
		Source: "default",
		Config: cfg,
	}, nil
}

func (s *Service) selectGlobalConfig(ctx context.Context, projectUUID pgtype.UUID, globalConfigID string) (*domain.ProjectProviderSetting, error) {
	globalUUID, err := dbconv.StringToPgUUID(globalConfigID)
	if err != nil {
		return nil, domain.BadRequest("invalid global_config_id")
	}

	var setting *domain.ProjectProviderSetting
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		if err := providersetting.LockProjectRow(ctx, q, projectUUID); err != nil {
			return err
		}
		row, err := q.GetLLMProviderConfigByID(ctx, globalUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("llm global config not found")
			}
			return fmt.Errorf("get llm global config: %w", err)
		}
		if row.ProjectID.Valid {
			return domain.BadRequest("global_config_id must reference a platform-managed config")
		}
		if !row.IsActive {
			return domain.BadRequest("selected llm global config is not active")
		}
		if !row.IsAvailableToProjects {
			return domain.BadRequest("selected llm global config is not available to projects")
		}
		if err := q.DeactivateProjectLLMProviderConfigs(ctx, projectUUID); err != nil {
			return fmt.Errorf("deactivate project llm configs: %w", err)
		}
		if err := q.SetSelectedLLMGlobalConfig(ctx, db.SetSelectedLLMGlobalConfigParams{
			ID:                        projectUUID,
			SelectedLlmGlobalConfigID: globalUUID,
		}); err != nil {
			return fmt.Errorf("set selected llm global config: %w", err)
		}
		cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode selected llm global config: %w", err)
		}
		setting = &domain.ProjectProviderSetting{
			Mode:           "global",
			GlobalConfigID: dbconv.PgUUIDToString(globalUUID),
			Config:         cfg,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return setting, nil
}

func (s *Service) upsertCustomConfig(ctx context.Context, projectUUID pgtype.UUID, req UpdateRequest) (*domain.LLMProviderConfig, error) {
	if err := validateCustomUpdateRequest(&req); err != nil {
		return nil, err
	}

	var cfg domain.LLMProviderConfig
	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		if err := providersetting.LockProjectRow(ctx, q, projectUUID); err != nil {
			return err
		}
		current, found, err := s.getActiveCustomConfigTx(ctx, q, projectUUID)
		if err != nil {
			return err
		}

		settingsJSON, err := providersetting.MarshalJSONObject(req.Settings)
		if err != nil {
			return domain.BadRequest("settings must be a valid JSON object")
		}
		if found && len(current.CredentialsEncrypted) > 0 && !req.Credentials.Set &&
			(req.Provider != providers.NormalizeProviderID(current.Provider) || req.EndpointURL != strings.TrimSpace(current.EndpointUrl)) {
			return domain.BadRequest("credentials must be re-entered when provider or endpoint_url changes")
		}

		credentialsEncrypted, err := providersetting.ResolveCredentials(req.Credentials, current.CredentialsEncrypted, found, s.secretKey)
		if err != nil {
			return err
		}

		if err := q.ClearSelectedLLMGlobalConfig(ctx, projectUUID); err != nil {
			return fmt.Errorf("clear selected llm global config: %w", err)
		}
		if err := q.DeactivateProjectLLMProviderConfigs(ctx, projectUUID); err != nil {
			return fmt.Errorf("deactivate project llm configs: %w", err)
		}

		model := pgtype.Text{}
		if req.Model != "" {
			model = pgtype.Text{String: req.Model, Valid: true}
		}
		row, err := q.CreateLLMProviderConfig(ctx, db.CreateLLMProviderConfigParams{
			Name:                 req.Name,
			Provider:             req.Provider,
			EndpointUrl:          req.EndpointURL,
			Model:                model,
			SettingsJson:         settingsJSON,
			CredentialsEncrypted: credentialsEncrypted,
			ProjectID:            projectUUID,
		})
		if err != nil {
			if postgres.IsUniqueViolation(err) {
				return domain.Conflict("an active llm configuration already exists; retry the request")
			}
			return fmt.Errorf("create llm provider config: %w", err)
		}

		cfg, err = dbconv.DBLLMProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode created llm config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListGlobalConfigs returns all global (project_id IS NULL) LLM configs.
func (s *Service) ListGlobalConfigs(ctx context.Context) ([]domain.LLMProviderConfig, error) {
	rows, err := s.db.Queries.ListGlobalLLMProviderConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list global llm configs: %w", err)
	}
	items := make([]domain.LLMProviderConfig, 0, len(rows))
	for _, row := range rows {
		item, err := dbconv.DBLLMProviderConfigToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("decode global llm config: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// GetGlobalDefaultConfig returns the active default global LLM config.
func (s *Service) GetGlobalDefaultConfig(ctx context.Context) (*domain.LLMProviderConfig, error) {
	row, err := s.db.Queries.GetDefaultGlobalLLMProviderConfig(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get default global llm config: %w", err)
	}
	cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode default global llm config: %w", err)
	}
	return &cfg, nil
}

// UpdateGlobalConfig creates or updates the global LLM default configuration.
// Uses immutable-append in a transaction: deactivate all global configs, then insert new.
func (s *Service) UpdateGlobalConfig(ctx context.Context, req GlobalUpdateRequest) (*domain.LLMProviderConfig, error) {
	updateReq := &UpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Settings:    req.Settings,
	}
	if err := validateCustomUpdateRequest(updateReq); err != nil {
		return nil, err
	}

	settingsJSON, err := providersetting.MarshalJSONObject(req.Settings)
	if err != nil {
		return nil, domain.BadRequest("settings must be a valid JSON object")
	}

	// Determine current global config for credential carry-forward.
	current, err := s.db.Queries.GetDefaultGlobalLLMProviderConfig(ctx)
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get current global llm config: %w", err)
	}

	if hasCurrent && len(current.CredentialsEncrypted) > 0 && !req.Credentials.Set &&
		(updateReq.Provider != current.Provider || updateReq.EndpointURL != current.EndpointUrl) {
		return nil, domain.BadRequest("credentials must be re-entered when provider or endpoint_url changes")
	}

	credentialsEncrypted, err := providersetting.ResolveCredentials(req.Credentials, current.CredentialsEncrypted, hasCurrent, s.secretKey)
	if err != nil {
		return nil, err
	}

	isAvailable := true
	if req.IsAvailableToProjects != nil {
		isAvailable = *req.IsAvailableToProjects
	}

	model := pgtype.Text{}
	if updateReq.Model != "" {
		model = pgtype.Text{String: updateReq.Model, Valid: true}
	}

	var cfg domain.LLMProviderConfig
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		if err := q.DeactivateDefaultGlobalLLMProviderConfig(ctx); err != nil {
			return fmt.Errorf("deactivate default global llm config: %w", err)
		}

		row, err := q.CreateGlobalLLMProviderConfig(ctx, db.CreateGlobalLLMProviderConfigParams{
			Name:                  updateReq.Name,
			Provider:              updateReq.Provider,
			EndpointUrl:           updateReq.EndpointURL,
			Model:                 model,
			SettingsJson:          settingsJSON,
			CredentialsEncrypted:  credentialsEncrypted,
			IsAvailableToProjects: isAvailable,
		})
		if err != nil {
			return fmt.Errorf("create global llm config: %w", err)
		}

		cfg, err = dbconv.DBLLMProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode created global llm config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// TestGlobalConnectivity tests the active global default config, or an ad-hoc config if provided.
func (s *Service) TestGlobalConnectivity(ctx context.Context, cfg *domain.LLMProviderConfig) (*providers.ConnectivityResult, error) {
	if cfg == nil {
		defaultCfg, err := s.GetGlobalDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		if defaultCfg == nil {
			return nil, domain.NotFound("no global llm configuration found")
		}
		cfg = defaultCfg
	}
	return s.TestConnectivity(ctx, cfg)
}

func (s *Service) getActiveCustomConfig(ctx context.Context, projectUUID pgtype.UUID) (db.LlmProviderConfig, bool, error) {
	return s.getActiveCustomConfigTx(ctx, s.db.Queries, projectUUID)
}

func (s *Service) getActiveCustomConfigTx(ctx context.Context, q *db.Queries, projectUUID pgtype.UUID) (db.LlmProviderConfig, bool, error) {
	row, err := q.GetActiveProjectLLMProviderConfig(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.LlmProviderConfig{}, false, nil
		}
		return db.LlmProviderConfig{}, false, fmt.Errorf("get active project llm config: %w", err)
	}
	return row, true, nil
}

// CreateAdditionalGlobalConfig creates a new non-default global LLM config.
func (s *Service) CreateAdditionalGlobalConfig(ctx context.Context, req GlobalUpdateRequest) (*domain.LLMProviderConfig, error) {
	updateReq := &UpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Settings:    req.Settings,
	}
	if err := validateCustomUpdateRequest(updateReq); err != nil {
		return nil, err
	}

	settingsJSON, err := providersetting.MarshalJSONObject(req.Settings)
	if err != nil {
		return nil, domain.BadRequest("settings must be a valid JSON object")
	}

	credentialsEncrypted, err := providersetting.ResolveCredentials(req.Credentials, nil, false, s.secretKey)
	if err != nil {
		return nil, err
	}

	isAvailable := true
	if req.IsAvailableToProjects != nil {
		isAvailable = *req.IsAvailableToProjects
	}

	model := pgtype.Text{}
	if updateReq.Model != "" {
		model = pgtype.Text{String: updateReq.Model, Valid: true}
	}

	row, err := s.db.Queries.CreateGlobalLLMProviderConfigNonDefault(ctx, db.CreateGlobalLLMProviderConfigNonDefaultParams{
		Name:                  updateReq.Name,
		Provider:              updateReq.Provider,
		EndpointUrl:           updateReq.EndpointURL,
		Model:                 model,
		SettingsJson:          settingsJSON,
		CredentialsEncrypted:  credentialsEncrypted,
		IsAvailableToProjects: isAvailable,
	})
	if err != nil {
		return nil, fmt.Errorf("create non-default global llm config: %w", err)
	}

	cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode created global llm config: %w", err)
	}
	return &cfg, nil
}

// UpdateGlobalConfigByID partially updates a specific global LLM config.
func (s *Service) UpdateGlobalConfigByID(ctx context.Context, configID string, req GlobalPatchRequest) (*domain.LLMProviderConfig, error) {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return nil, domain.BadRequest("invalid config ID")
	}

	current, err := s.db.Queries.GetGlobalLLMProviderConfigByID(ctx, configUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("provider config not found")
		}
		return nil, fmt.Errorf("get global llm config: %w", err)
	}
	if !current.IsActive {
		return nil, domain.NotFound("provider config not found")
	}

	// Determine effective provider and endpoint for credential carry-forward check.
	effectiveProvider := current.Provider
	if req.Provider != nil {
		effectiveProvider = providers.NormalizeProviderID(*req.Provider)
	}
	effectiveEndpoint := current.EndpointUrl
	if req.EndpointURL != nil {
		effectiveEndpoint = strings.TrimSpace(*req.EndpointURL)
	}

	if len(current.CredentialsEncrypted) > 0 && !req.Credentials.Set &&
		(effectiveProvider != current.Provider || effectiveEndpoint != current.EndpointUrl) {
		return nil, domain.BadRequest("credentials must be re-entered when provider or endpoint_url changes")
	}

	credentialsEncrypted, err := providersetting.ResolveCredentials(req.Credentials, current.CredentialsEncrypted, true, s.secretKey)
	if err != nil {
		return nil, err
	}

	// Build COALESCE params: only set Valid=true for fields that are provided.
	params := db.UpdateGlobalLLMProviderConfigParams{
		ID:               configUUID,
		ClearCredentials: req.Credentials.Set && req.Credentials.Clear,
	}
	if credentialsEncrypted != nil && !params.ClearCredentials {
		params.CredentialsEncrypted = credentialsEncrypted
	}
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			return nil, domain.BadRequest("name must not be empty")
		}
		params.Name = pgtype.Text{String: v, Valid: true}
	}
	if req.Provider != nil {
		v := providers.NormalizeProviderID(*req.Provider)
		if v == "" {
			return nil, domain.BadRequest("provider must not be empty")
		}
		if err := providers.ValidateProvider(providers.CapabilityLLM, v); err != nil {
			return nil, err
		}
		params.Provider = pgtype.Text{String: v, Valid: true}
	}
	if req.EndpointURL != nil {
		v := strings.TrimSpace(*req.EndpointURL)
		if v == "" {
			return nil, domain.BadRequest("endpoint_url must not be empty")
		}
		params.EndpointUrl = pgtype.Text{String: v, Valid: true}
	}
	if req.Model != nil {
		v := strings.TrimSpace(*req.Model)
		params.Model = pgtype.Text{String: v, Valid: true}
	}
	if req.Settings != nil {
		settingsJSON, err := providersetting.MarshalJSONObject(req.Settings)
		if err != nil {
			return nil, domain.BadRequest("settings must be a valid JSON object")
		}
		params.SettingsJson = settingsJSON
	}
	if req.IsAvailableToProjects != nil {
		params.IsAvailableToProjects = pgtype.Bool{Bool: *req.IsAvailableToProjects, Valid: true}
	}

	row, err := s.db.Queries.UpdateGlobalLLMProviderConfig(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("provider config not found")
		}
		return nil, fmt.Errorf("update global llm config: %w", err)
	}

	cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode updated global llm config: %w", err)
	}
	return &cfg, nil
}

// DeleteGlobalConfig deactivates a specific global LLM config.
// Cannot delete the default config or one referenced by projects.
func (s *Service) DeleteGlobalConfig(ctx context.Context, configID string) error {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return domain.BadRequest("invalid config ID")
	}

	row, err := s.db.Queries.GetGlobalLLMProviderConfigByID(ctx, configUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("provider config not found")
		}
		return fmt.Errorf("get global llm config: %w", err)
	}
	if !row.IsActive {
		return domain.NotFound("provider config not found")
	}
	if row.IsDefault {
		return domain.BadRequest("cannot delete the default provider; promote another provider first")
	}

	count, err := s.db.Queries.CountProjectsUsingLLMConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("count projects using llm config: %w", err)
	}
	if count > 0 {
		return domain.Conflict(fmt.Sprintf("cannot delete provider: %d project(s) are using it", count))
	}

	affected, err := s.db.Queries.DeactivateGlobalLLMProviderConfigByID(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("deactivate global llm config: %w", err)
	}
	if affected == 0 {
		// Re-fetch to distinguish "not found / already inactive" from "concurrently promoted".
		cur, err := s.db.Queries.GetGlobalLLMProviderConfigByID(ctx, configUUID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("re-fetch llm config after no-op deactivate: %w", err)
		}
		if err == nil && cur.IsActive && cur.IsDefault {
			return domain.BadRequest("cannot delete the default provider; promote another provider first")
		}
		return domain.NotFound("provider config not found")
	}
	return nil
}

// PromoteToDefault promotes a specific global LLM config to default.
// The previously default config becomes a regular shared provider.
func (s *Service) PromoteToDefault(ctx context.Context, configID string) (*domain.LLMProviderConfig, error) {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return nil, domain.BadRequest("invalid config ID")
	}

	var cfg domain.LLMProviderConfig
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.GetGlobalLLMProviderConfigByID(ctx, configUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("provider config not found")
			}
			return fmt.Errorf("get global llm config: %w", err)
		}
		if !row.IsActive {
			return domain.BadRequest("cannot promote inactive provider")
		}

		// Idempotent: already default → return as-is.
		if row.IsDefault {
			cfg, err = dbconv.DBLLMProviderConfigToDomain(row)
			if err != nil {
				return fmt.Errorf("decode global llm config: %w", err)
			}
			return nil
		}

		if err := q.DemoteGlobalLLMProviderConfigDefault(ctx); err != nil {
			return fmt.Errorf("demote current default: %w", err)
		}
		rowsAffected, err := q.PromoteGlobalLLMProviderConfigToDefault(ctx, configUUID)
		if err != nil {
			return fmt.Errorf("promote to default: %w", err)
		}
		if rowsAffected == 0 {
			return domain.BadRequest("cannot promote: config was deactivated or not found")
		}

		// Re-load to get updated timestamps.
		updated, err := q.GetGlobalLLMProviderConfigByID(ctx, configUUID)
		if err != nil {
			return fmt.Errorf("reload promoted config: %w", err)
		}
		cfg, err = dbconv.DBLLMProviderConfigToDomain(updated)
		if err != nil {
			return fmt.Errorf("decode promoted global llm config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// TestGlobalConnectivityByID tests connectivity for a specific global LLM config.
func (s *Service) TestGlobalConnectivityByID(ctx context.Context, configID string) (*providers.ConnectivityResult, error) {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return nil, domain.BadRequest("invalid config ID")
	}

	row, err := s.db.Queries.GetGlobalLLMProviderConfigByID(ctx, configUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("provider config not found")
		}
		return nil, fmt.Errorf("get global llm config: %w", err)
	}
	if !row.IsActive {
		return nil, domain.NotFound("provider config not found")
	}

	cfg, err := dbconv.DBLLMProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode global llm config: %w", err)
	}
	return s.TestConnectivity(ctx, &cfg)
}

func validateCustomUpdateRequest(req *UpdateRequest) error {
	if err := providersetting.ValidateBaseFields(&req.Name, &req.Provider, &req.EndpointURL, &req.Settings, providers.CapabilityLLM); err != nil {
		return err
	}
	req.Model = strings.TrimSpace(req.Model)
	return nil
}
