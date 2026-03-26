package embedding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// UpdateRequest is the input for creating or updating a project embedding setting.
type UpdateRequest struct {
	Mode           string
	GlobalConfigID string
	Name           string
	Provider       string
	EndpointURL    string
	Model          string
	Dimensions     int
	MaxTokens      int
	Settings       map[string]any
	Credentials    CredentialsUpdate
}

// GlobalUpdateRequest is the input for creating or updating the global embedding default.
type GlobalUpdateRequest struct {
	Name                  string
	Provider              string
	EndpointURL           string
	Model                 string
	Dimensions            int
	MaxTokens             int
	Settings              map[string]any
	Credentials           CredentialsUpdate
	IsAvailableToProjects *bool // defaults to true if nil
}

// GlobalPatchRequest is the input for partial-updating a specific global embedding config.
// All fields are pointers; nil means "keep current value".
type GlobalPatchRequest struct {
	Name                  *string
	Provider              *string
	EndpointURL           *string
	Model                 *string
	Dimensions            *int
	MaxTokens             *int
	Settings              map[string]any // nil means keep existing
	Credentials           CredentialsUpdate
	IsAvailableToProjects *bool
}

type resolvedState struct {
	Mode           string
	Source         string
	GlobalConfigID string
	Config         domain.EmbeddingProviderConfig
}

// Service provides embedding provider configuration business logic.
type Service struct {
	db         *postgres.DB
	httpClient *http.Client
	secretKey  []byte
}

// NewService creates a new embedding service.
// It returns nil, nil when the database is not configured.
func NewService(database *postgres.DB, httpClient *http.Client, encryptionSecret string) (*Service, error) {
	if database == nil {
		return nil, nil
	}
	if database.Queries == nil {
		return nil, fmt.Errorf("embedding: database queries not initialized")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if strings.TrimSpace(encryptionSecret) == "" {
		return nil, fmt.Errorf("embedding: encryption secret is required")
	}
	return &Service{
		db:         database,
		httpClient: httpClient,
		secretKey:  secrets.DeriveKey(encryptionSecret),
	}, nil
}

// BootstrapDefaultConfig seeds the default global embedding config if none exists yet.
func BootstrapDefaultConfig(ctx context.Context, database *postgres.DB, defaults config.EmbeddingDefaults) error {
	if database == nil || database.Queries == nil {
		return nil
	}
	req := UpdateRequest{
		Name:        config.DefaultPlatformProviderConfigName,
		Provider:    defaults.Provider,
		EndpointURL: defaults.EndpointURL,
		Model:       defaults.Model,
		Dimensions:  defaults.Dimensions,
		MaxTokens:   defaults.MaxTokens,
		Settings:    map[string]any{},
	}
	if err := validateCustomUpdateRequest(&req); err != nil {
		return fmt.Errorf("bootstrap embedding default: %w", err)
	}
	err := database.Queries.CreateEmbeddingDefaultProviderConfigIfMissing(ctx, db.CreateEmbeddingDefaultProviderConfigIfMissingParams{
		Name:         req.Name,
		Provider:     req.Provider,
		EndpointUrl:  req.EndpointURL,
		Model:        req.Model,
		Dimensions:   int32(req.Dimensions),
		MaxTokens:    int32(req.MaxTokens),
		SettingsJson: []byte(`{}`),
	})
	if err != nil && postgres.IsUniqueViolation(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create embedding default provider config: %w", err)
	}
	return nil
}

// GetProjectSetting returns the project's current embedding selection mode and effective config.
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

// ListAvailableGlobalConfigs returns active global configs that projects may select.
func (s *Service) ListAvailableGlobalConfigs(ctx context.Context) ([]domain.EmbeddingProviderConfig, error) {
	rows, err := s.db.Queries.ListAvailableGlobalEmbeddingProviderConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available embedding configs: %w", err)
	}
	items := make([]domain.EmbeddingProviderConfig, 0, len(rows))
	for _, row := range rows {
		item, err := dbconv.DBAvailableEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("decode available embedding config: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// UpdateProjectSetting changes the project's embedding selection.
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
		if err := q.ClearSelectedEmbeddingGlobalConfig(ctx, projectUUID); err != nil {
			return fmt.Errorf("clear selected embedding config: %w", err)
		}
		if err := q.DeactivateProjectEmbeddingProviderConfigs(ctx, projectUUID); err != nil {
			return fmt.Errorf("deactivate project embedding configs: %w", err)
		}
		return nil
	})
}

// GetResolvedConfig returns the effective embedding config for the project.
func (s *Service) GetResolvedConfig(ctx context.Context, projectID string) (*domain.ResolvedEmbeddingProviderSetting, error) {
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return nil, domain.BadRequest("invalid project ID")
	}
	state, err := s.resolveState(ctx, projectUUID)
	if err != nil {
		return nil, err
	}
	return &domain.ResolvedEmbeddingProviderSetting{
		Source: state.Source,
		Config: state.Config,
	}, nil
}

// TestConnectivity tests connectivity to an embedding provider.
func (s *Service) TestConnectivity(ctx context.Context, cfg *domain.EmbeddingProviderConfig) (*providers.ConnectivityResult, error) {
	if cfg == nil {
		return nil, domain.BadRequest("missing embedding configuration to test")
	}
	result, err := providers.TestEmbeddingConnectivity(ctx, s.httpClient, cfg)
	if err != nil {
		return nil, domain.BadRequest(err.Error())
	}
	return &result, nil
}

func (s *Service) resolveState(ctx context.Context, projectUUID pgtype.UUID) (*resolvedState, error) {
	if custom, found, err := s.getActiveCustomConfig(ctx, projectUUID); err != nil {
		return nil, err
	} else if found {
		cfg, err := dbconv.DBEmbeddingProviderConfigToDomain(custom)
		if err != nil {
			return nil, fmt.Errorf("decode active project embedding config: %w", err)
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

	if selections.SelectedEmbeddingGlobalConfigID.Valid {
		row, err := s.db.Queries.GetEmbeddingProviderConfigByID(ctx, selections.SelectedEmbeddingGlobalConfigID)
		if err == nil && !row.ProjectID.Valid && row.IsActive {
			cfg, err := dbconv.DBEmbeddingProviderConfigToDomain(row)
			if err != nil {
				return nil, fmt.Errorf("decode selected embedding global config: %w", err)
			}
			return &resolvedState{
				Mode:           "global",
				Source:         "global",
				GlobalConfigID: dbconv.PgUUIDToString(selections.SelectedEmbeddingGlobalConfigID),
				Config:         cfg,
			}, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get selected embedding config: %w", err)
		}
		// Selected global config is stale (deactivated, deleted, or project-owned);
		// fall through to the default global config.
		slog.WarnContext(ctx, "embedding: stale global config selection, falling back to default", slog.String("project_id", dbconv.PgUUIDToString(projectUUID)))
	}

	row, err := s.db.Queries.GetDefaultGlobalEmbeddingProviderConfig(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("no embedding configuration found for this project")
		}
		return nil, fmt.Errorf("get default embedding config: %w", err)
	}
	cfg, err := dbconv.DBEmbeddingProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode default embedding config: %w", err)
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
		row, err := q.GetEmbeddingProviderConfigByID(ctx, globalUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("embedding global config not found")
			}
			return fmt.Errorf("get embedding global config: %w", err)
		}
		if row.ProjectID.Valid {
			return domain.BadRequest("global_config_id must reference a platform-managed config")
		}
		if !row.IsActive {
			return domain.BadRequest("selected embedding global config is not active")
		}
		if !row.IsAvailableToProjects {
			return domain.BadRequest("selected embedding global config is not available to projects")
		}
		if err := q.DeactivateProjectEmbeddingProviderConfigs(ctx, projectUUID); err != nil {
			return fmt.Errorf("deactivate project embedding configs: %w", err)
		}
		if err := q.SetSelectedEmbeddingGlobalConfig(ctx, db.SetSelectedEmbeddingGlobalConfigParams{
			ID:                              projectUUID,
			SelectedEmbeddingGlobalConfigID: globalUUID,
		}); err != nil {
			return fmt.Errorf("set selected embedding global config: %w", err)
		}
		cfg, err := dbconv.DBEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode selected embedding global config: %w", err)
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

func (s *Service) upsertCustomConfig(ctx context.Context, projectUUID pgtype.UUID, req UpdateRequest) (*domain.EmbeddingProviderConfig, error) {
	if err := validateCustomUpdateRequest(&req); err != nil {
		return nil, err
	}

	var cfg domain.EmbeddingProviderConfig
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

		if err := q.ClearSelectedEmbeddingGlobalConfig(ctx, projectUUID); err != nil {
			return fmt.Errorf("clear selected embedding global config: %w", err)
		}
		if err := q.DeactivateProjectEmbeddingProviderConfigs(ctx, projectUUID); err != nil {
			return fmt.Errorf("deactivate project embedding configs: %w", err)
		}

		row, err := q.CreateEmbeddingProviderConfig(ctx, db.CreateEmbeddingProviderConfigParams{
			Name:                 req.Name,
			Provider:             req.Provider,
			EndpointUrl:          req.EndpointURL,
			Model:                req.Model,
			Dimensions:           int32(req.Dimensions),
			MaxTokens:            int32(req.MaxTokens),
			SettingsJson:         settingsJSON,
			CredentialsEncrypted: credentialsEncrypted,
			ProjectID:            projectUUID,
		})
		if err != nil {
			if postgres.IsUniqueViolation(err) {
				return domain.Conflict("an active embedding configuration already exists; retry the request")
			}
			return fmt.Errorf("create embedding provider config: %w", err)
		}

		label, err := versionLabel(req.Provider, req.Model)
		if err != nil {
			return err
		}
		if _, err := q.CreateEmbeddingVersion(ctx, db.CreateEmbeddingVersionParams{
			EmbeddingProviderConfigID: row.ID,
			Provider:                  req.Provider,
			Model:                     req.Model,
			Dimensions:                int32(req.Dimensions),
			VersionLabel:              label,
		}); err != nil {
			return fmt.Errorf("create embedding version: %w", err)
		}

		cfg, err = dbconv.DBEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode created embedding config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListGlobalConfigs returns all global (project_id IS NULL) embedding configs.
func (s *Service) ListGlobalConfigs(ctx context.Context) ([]domain.EmbeddingProviderConfig, error) {
	rows, err := s.db.Queries.ListGlobalEmbeddingProviderConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list global embedding configs: %w", err)
	}
	items := make([]domain.EmbeddingProviderConfig, 0, len(rows))
	for _, row := range rows {
		item, err := dbconv.DBEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("decode global embedding config: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// GetGlobalDefaultConfig returns the active default global embedding config.
func (s *Service) GetGlobalDefaultConfig(ctx context.Context) (*domain.EmbeddingProviderConfig, error) {
	row, err := s.db.Queries.GetDefaultGlobalEmbeddingProviderConfig(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get default global embedding config: %w", err)
	}
	cfg, err := dbconv.DBEmbeddingProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode default global embedding config: %w", err)
	}
	return &cfg, nil
}

// UpdateGlobalConfig creates or updates the global embedding default configuration.
// Uses immutable-append in a transaction: deactivate all global configs, then insert new.
func (s *Service) UpdateGlobalConfig(ctx context.Context, req GlobalUpdateRequest) (*domain.EmbeddingProviderConfig, error) {
	updateReq := &UpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Dimensions:  req.Dimensions,
		MaxTokens:   req.MaxTokens,
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
	current, err := s.db.Queries.GetDefaultGlobalEmbeddingProviderConfig(ctx)
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get current global embedding config: %w", err)
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

	var cfg domain.EmbeddingProviderConfig
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		if err := q.DeactivateDefaultGlobalEmbeddingProviderConfig(ctx); err != nil {
			return fmt.Errorf("deactivate default global embedding config: %w", err)
		}

		row, err := q.CreateGlobalEmbeddingProviderConfig(ctx, db.CreateGlobalEmbeddingProviderConfigParams{
			Name:                  updateReq.Name,
			Provider:              updateReq.Provider,
			EndpointUrl:           updateReq.EndpointURL,
			Model:                 updateReq.Model,
			Dimensions:            int32(updateReq.Dimensions),
			MaxTokens:             int32(updateReq.MaxTokens),
			SettingsJson:          settingsJSON,
			CredentialsEncrypted:  credentialsEncrypted,
			IsAvailableToProjects: isAvailable,
		})
		if err != nil {
			return fmt.Errorf("create global embedding config: %w", err)
		}

		label, err := versionLabel(updateReq.Provider, updateReq.Model)
		if err != nil {
			return err
		}
		if _, err := q.CreateEmbeddingVersion(ctx, db.CreateEmbeddingVersionParams{
			EmbeddingProviderConfigID: row.ID,
			Provider:                  updateReq.Provider,
			Model:                     updateReq.Model,
			Dimensions:                int32(updateReq.Dimensions),
			VersionLabel:              label,
		}); err != nil {
			return fmt.Errorf("create embedding version: %w", err)
		}

		cfg, err = dbconv.DBEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode created global embedding config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// TestGlobalConnectivity tests the active global default config, or an ad-hoc config if provided.
func (s *Service) TestGlobalConnectivity(ctx context.Context, cfg *domain.EmbeddingProviderConfig) (*providers.ConnectivityResult, error) {
	if cfg == nil {
		defaultCfg, err := s.GetGlobalDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		if defaultCfg == nil {
			return nil, domain.NotFound("no global embedding configuration found")
		}
		cfg = defaultCfg
	}
	return s.TestConnectivity(ctx, cfg)
}

func (s *Service) getActiveCustomConfig(ctx context.Context, projectUUID pgtype.UUID) (db.EmbeddingProviderConfig, bool, error) {
	return s.getActiveCustomConfigTx(ctx, s.db.Queries, projectUUID)
}

func (s *Service) getActiveCustomConfigTx(ctx context.Context, q *db.Queries, projectUUID pgtype.UUID) (db.EmbeddingProviderConfig, bool, error) {
	row, err := q.GetActiveProjectEmbeddingProviderConfig(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.EmbeddingProviderConfig{}, false, nil
		}
		return db.EmbeddingProviderConfig{}, false, fmt.Errorf("get active project embedding config: %w", err)
	}
	return row, true, nil
}

// CreateAdditionalGlobalConfig creates a new non-default global embedding config.
func (s *Service) CreateAdditionalGlobalConfig(ctx context.Context, req GlobalUpdateRequest) (*domain.EmbeddingProviderConfig, error) {
	updateReq := &UpdateRequest{
		Name:        req.Name,
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		Model:       req.Model,
		Dimensions:  req.Dimensions,
		MaxTokens:   req.MaxTokens,
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

	var cfg domain.EmbeddingProviderConfig
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.CreateGlobalEmbeddingProviderConfigNonDefault(ctx, db.CreateGlobalEmbeddingProviderConfigNonDefaultParams{
			Name:                  updateReq.Name,
			Provider:              updateReq.Provider,
			EndpointUrl:           updateReq.EndpointURL,
			Model:                 updateReq.Model,
			Dimensions:            int32(updateReq.Dimensions),
			MaxTokens:             int32(updateReq.MaxTokens),
			SettingsJson:          settingsJSON,
			CredentialsEncrypted:  credentialsEncrypted,
			IsAvailableToProjects: isAvailable,
		})
		if err != nil {
			return fmt.Errorf("create non-default global embedding config: %w", err)
		}

		label, err := versionLabel(updateReq.Provider, updateReq.Model)
		if err != nil {
			return err
		}
		if _, err := q.CreateEmbeddingVersion(ctx, db.CreateEmbeddingVersionParams{
			EmbeddingProviderConfigID: row.ID,
			Provider:                  updateReq.Provider,
			Model:                     updateReq.Model,
			Dimensions:                int32(updateReq.Dimensions),
			VersionLabel:              label,
		}); err != nil {
			return fmt.Errorf("create embedding version: %w", err)
		}

		cfg, err = dbconv.DBEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode created global embedding config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateGlobalConfigByID partially updates a specific global embedding config.
func (s *Service) UpdateGlobalConfigByID(ctx context.Context, configID string, req GlobalPatchRequest) (*domain.EmbeddingProviderConfig, error) {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return nil, domain.BadRequest("invalid config ID")
	}

	current, err := s.db.Queries.GetGlobalEmbeddingProviderConfigByID(ctx, configUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("provider config not found")
		}
		return nil, fmt.Errorf("get global embedding config: %w", err)
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
	params := db.UpdateGlobalEmbeddingProviderConfigParams{
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
		if err := providers.ValidateProvider(providers.CapabilityEmbedding, v); err != nil {
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
		if v == "" {
			return nil, domain.BadRequest("model must not be empty")
		}
		params.Model = pgtype.Text{String: v, Valid: true}
	}
	if req.Dimensions != nil {
		if *req.Dimensions < 1 || *req.Dimensions > 65536 {
			return nil, domain.BadRequest("dimensions must be between 1 and 65536")
		}
		params.Dimensions = pgtype.Int4{Int32: int32(*req.Dimensions), Valid: true}
	}
	if req.MaxTokens != nil {
		if *req.MaxTokens < 1 || *req.MaxTokens > 131072 {
			return nil, domain.BadRequest("max_tokens must be between 1 and 131072")
		}
		params.MaxTokens = pgtype.Int4{Int32: int32(*req.MaxTokens), Valid: true}
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

	var cfg domain.EmbeddingProviderConfig
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.UpdateGlobalEmbeddingProviderConfig(ctx, params)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("provider config not found")
			}
			return fmt.Errorf("update global embedding config: %w", err)
		}

		// Create embedding version if provider, model, or dimensions changed.
		newProvider := row.Provider
		newModel := row.Model
		newDims := row.Dimensions
		if newProvider != current.Provider || newModel != current.Model || newDims != current.Dimensions {
			label, err := versionLabel(newProvider, newModel)
			if err != nil {
				return err
			}
			if _, err := q.CreateEmbeddingVersion(ctx, db.CreateEmbeddingVersionParams{
				EmbeddingProviderConfigID: row.ID,
				Provider:                  newProvider,
				Model:                     newModel,
				Dimensions:                newDims,
				VersionLabel:              label,
			}); err != nil {
				return fmt.Errorf("create embedding version: %w", err)
			}
		}

		cfg, err = dbconv.DBEmbeddingProviderConfigToDomain(row)
		if err != nil {
			return fmt.Errorf("decode updated global embedding config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DeleteGlobalConfig deactivates a specific global embedding config.
// Cannot delete the default config or one referenced by projects.
func (s *Service) DeleteGlobalConfig(ctx context.Context, configID string) error {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return domain.BadRequest("invalid config ID")
	}

	row, err := s.db.Queries.GetGlobalEmbeddingProviderConfigByID(ctx, configUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("provider config not found")
		}
		return fmt.Errorf("get global embedding config: %w", err)
	}
	if !row.IsActive {
		return domain.NotFound("provider config not found")
	}
	if row.IsDefault {
		return domain.BadRequest("cannot delete the default provider; promote another provider first")
	}

	count, err := s.db.Queries.CountProjectsUsingEmbeddingConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("count projects using embedding config: %w", err)
	}
	if count > 0 {
		return domain.Conflict(fmt.Sprintf("cannot delete provider: %d project(s) are using it", count))
	}

	affected, err := s.db.Queries.DeactivateGlobalEmbeddingProviderConfigByID(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("deactivate global embedding config: %w", err)
	}
	if affected == 0 {
		// Re-fetch to distinguish "not found / already inactive" from "concurrently promoted".
		cur, err := s.db.Queries.GetGlobalEmbeddingProviderConfigByID(ctx, configUUID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("re-fetch embedding config after no-op deactivate: %w", err)
		}
		if err == nil && cur.IsActive && cur.IsDefault {
			return domain.BadRequest("cannot delete the default provider; promote another provider first")
		}
		return domain.NotFound("provider config not found")
	}
	return nil
}

// PromoteToDefault promotes a specific global embedding config to default.
// The previously default config becomes a regular shared provider.
func (s *Service) PromoteToDefault(ctx context.Context, configID string) (*domain.EmbeddingProviderConfig, error) {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return nil, domain.BadRequest("invalid config ID")
	}

	var cfg domain.EmbeddingProviderConfig
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.GetGlobalEmbeddingProviderConfigByID(ctx, configUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("provider config not found")
			}
			return fmt.Errorf("get global embedding config: %w", err)
		}
		if !row.IsActive {
			return domain.BadRequest("cannot promote inactive provider")
		}

		// Idempotent: already default → return as-is.
		if row.IsDefault {
			cfg, err = dbconv.DBEmbeddingProviderConfigToDomain(row)
			if err != nil {
				return fmt.Errorf("decode global embedding config: %w", err)
			}
			return nil
		}

		if err := q.DemoteGlobalEmbeddingProviderConfigDefault(ctx); err != nil {
			return fmt.Errorf("demote current default: %w", err)
		}
		rowsAffected, err := q.PromoteGlobalEmbeddingProviderConfigToDefault(ctx, configUUID)
		if err != nil {
			return fmt.Errorf("promote to default: %w", err)
		}
		if rowsAffected == 0 {
			return domain.BadRequest("cannot promote: config was deactivated or not found")
		}

		// Re-load to get updated timestamps.
		updated, err := q.GetGlobalEmbeddingProviderConfigByID(ctx, configUUID)
		if err != nil {
			return fmt.Errorf("reload promoted config: %w", err)
		}
		cfg, err = dbconv.DBEmbeddingProviderConfigToDomain(updated)
		if err != nil {
			return fmt.Errorf("decode promoted global embedding config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// TestGlobalConnectivityByID tests connectivity for a specific global embedding config.
func (s *Service) TestGlobalConnectivityByID(ctx context.Context, configID string) (*providers.ConnectivityResult, error) {
	configUUID, err := dbconv.StringToPgUUID(configID)
	if err != nil {
		return nil, domain.BadRequest("invalid config ID")
	}

	row, err := s.db.Queries.GetGlobalEmbeddingProviderConfigByID(ctx, configUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("provider config not found")
		}
		return nil, fmt.Errorf("get global embedding config: %w", err)
	}
	if !row.IsActive {
		return nil, domain.NotFound("provider config not found")
	}

	cfg, err := dbconv.DBEmbeddingProviderConfigToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("decode global embedding config: %w", err)
	}
	return s.TestConnectivity(ctx, &cfg)
}

func validateCustomUpdateRequest(req *UpdateRequest) error {
	if err := providersetting.ValidateBaseFields(&req.Name, &req.Provider, &req.EndpointURL, &req.Settings, providers.CapabilityEmbedding); err != nil {
		return err
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		return domain.BadRequest("model is required")
	}
	if req.Dimensions < 1 || req.Dimensions > 65536 {
		return domain.BadRequest("dimensions must be between 1 and 65536")
	}
	if req.MaxTokens < 1 || req.MaxTokens > 131072 {
		return domain.BadRequest("max_tokens must be between 1 and 131072")
	}
	return nil
}

func versionLabel(provider, model string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate version label: %w", err)
	}
	return fmt.Sprintf("%s-%s-%s", provider, model, hex.EncodeToString(b[:])), nil
}
