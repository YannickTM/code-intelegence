// Package dbconv provides conversion helpers between sqlc database types
// and domain models. It lives in a neutral package to avoid cross-layer
// coupling between handler and middleware.
package dbconv

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"myjungle/backend-api/internal/domain"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// DBUserToDomain converts a sqlc User to a domain User.
func DBUserToDomain(u db.User) domain.User {
	d := domain.User{
		Username: u.Username,
		Email:    u.Email,
		IsActive: u.IsActive,
	}
	if u.ID.Valid {
		d.ID = PgUUIDToString(u.ID)
	}
	if u.DisplayName.Valid {
		d.DisplayName = u.DisplayName.String
	}
	if u.AvatarUrl.Valid {
		d.AvatarURL = u.AvatarUrl.String
	}
	if u.CreatedAt.Valid {
		d.CreatedAt = u.CreatedAt.Time
	}
	if u.UpdatedAt.Valid {
		d.UpdatedAt = u.UpdatedAt.Time
	}
	return d
}

// DBMemberToDomain converts a sqlc ProjectMember to a domain ProjectMember.
func DBMemberToDomain(m db.ProjectMember) domain.ProjectMember {
	d := domain.ProjectMember{
		Role: m.Role,
	}
	if m.ID.Valid {
		d.ID = PgUUIDToString(m.ID)
	}
	if m.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(m.ProjectID)
	}
	if m.UserID.Valid {
		d.UserID = PgUUIDToString(m.UserID)
	}
	if m.CreatedAt.Valid {
		d.CreatedAt = m.CreatedAt.Time
	}
	if m.UpdatedAt.Valid {
		d.UpdatedAt = m.UpdatedAt.Time
	}
	return d
}

// DBMemberWithUserToDomain converts a ListProjectMembersRow (joined member+user)
// to a domain ProjectMemberWithUser.
func DBMemberWithUserToDomain(row db.ListProjectMembersRow) domain.ProjectMemberWithUser {
	d := domain.ProjectMemberWithUser{
		Username: row.Username,
		Role:     row.Role,
	}
	if row.ID.Valid {
		d.ID = PgUUIDToString(row.ID)
	}
	if row.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(row.ProjectID)
	}
	if row.UserID.Valid {
		d.UserID = PgUUIDToString(row.UserID)
	}
	if row.DisplayName.Valid {
		d.DisplayName = row.DisplayName.String
	}
	if row.AvatarUrl.Valid {
		d.AvatarURL = row.AvatarUrl.String
	}
	if row.CreatedAt.Valid {
		d.CreatedAt = row.CreatedAt.Time
	}
	return d
}

// SessionRowToUser converts a GetSessionByTokenHashRow (joined session+user)
// to a domain User.
func SessionRowToUser(row db.GetSessionByTokenHashRow) domain.User {
	d := domain.User{
		Username: row.Username,
		Email:    row.Email,
		IsActive: row.IsActive,
	}
	if row.UserID.Valid {
		d.ID = PgUUIDToString(row.UserID)
	}
	if row.DisplayName.Valid {
		d.DisplayName = row.DisplayName.String
	}
	if row.AvatarUrl.Valid {
		d.AvatarURL = row.AvatarUrl.String
	}
	if row.UserCreatedAt.Valid {
		d.CreatedAt = row.UserCreatedAt.Time
	}
	if row.UserUpdatedAt.Valid {
		d.UpdatedAt = row.UserUpdatedAt.Time
	}
	return d
}

// DBSSHKeyToDomain converts a sqlc SshKey to a domain SSHKey.
// The encrypted private key and CreatedBy fields are intentionally omitted.
func DBSSHKeyToDomain(k db.SshKey) domain.SSHKey {
	d := domain.SSHKey{
		Name:        k.Name,
		PublicKey:   k.PublicKey,
		Fingerprint: k.Fingerprint,
		KeyType:     k.KeyType,
		IsActive:    k.IsActive,
	}
	if k.ID.Valid {
		d.ID = PgUUIDToString(k.ID)
	}
	if k.CreatedAt.Valid {
		d.CreatedAt = k.CreatedAt.Time
	}
	if k.RotatedAt.Valid {
		t := k.RotatedAt.Time
		d.RotatedAt = &t
	}
	return d
}

// DBProjectToDomain converts a sqlc Project to a domain Project.
func DBProjectToDomain(p db.Project) domain.Project {
	d := domain.Project{
		Name:          p.Name,
		RepoURL:       p.RepoUrl,
		DefaultBranch: p.DefaultBranch,
		Status:        p.Status,
	}
	if p.ID.Valid {
		d.ID = PgUUIDToString(p.ID)
	}
	if p.CreatedBy.Valid {
		d.CreatedBy = PgUUIDToString(p.CreatedBy)
	}
	if p.CreatedAt.Valid {
		d.CreatedAt = p.CreatedAt.Time
	}
	if p.UpdatedAt.Valid {
		d.UpdatedAt = p.UpdatedAt.Time
	}
	return d
}

// DBSSHKeySummaryToDomain converts a GetProjectSSHKeyWithDetailsRow to a domain SSHKeySummary.
func DBSSHKeySummaryToDomain(k db.GetProjectSSHKeyWithDetailsRow) domain.SSHKeySummary {
	d := domain.SSHKeySummary{
		Name:        k.Name,
		Fingerprint: k.Fingerprint,
		PublicKey:   k.PublicKey,
		KeyType:     k.KeyType,
	}
	if k.ID.Valid {
		d.ID = PgUUIDToString(k.ID)
	}
	if k.CreatedAt.Valid {
		d.CreatedAt = k.CreatedAt.Time
	}
	return d
}

// DBAPIKeyToDomain converts a sqlc ApiKey to a domain APIKeyInfo.
func DBAPIKeyToDomain(k db.ApiKey) domain.APIKeyInfo {
	d := domain.APIKeyInfo{
		KeyType:   k.KeyType,
		KeyPrefix: k.KeyPrefix,
		Name:      k.Name,
		Role:      k.Role,
		IsActive:  k.IsActive,
	}
	if k.ID.Valid {
		d.ID = PgUUIDToString(k.ID)
	}
	if k.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(k.ProjectID)
	}
	if k.CreatedBy.Valid {
		d.CreatedBy = PgUUIDToString(k.CreatedBy)
	}
	if k.CreatedAt.Valid {
		d.CreatedAt = k.CreatedAt.Time
	}
	if k.ExpiresAt.Valid {
		t := k.ExpiresAt.Time
		d.ExpiresAt = &t
	}
	if k.LastUsedAt.Valid {
		t := k.LastUsedAt.Time
		d.LastUsedAt = &t
	}
	return d
}

// DBEmbeddingProviderConfigToDomain converts a sqlc EmbeddingProviderConfig to a domain model.
func DBEmbeddingProviderConfigToDomain(c db.EmbeddingProviderConfig) (domain.EmbeddingProviderConfig, error) {
	d := domain.EmbeddingProviderConfig{
		Name:                  c.Name,
		Provider:              c.Provider,
		EndpointURL:           c.EndpointUrl,
		Model:                 c.Model,
		Dimensions:            int(c.Dimensions),
		MaxTokens:             int(c.MaxTokens),
		HasCredentials:        len(c.CredentialsEncrypted) > 0,
		IsActive:              c.IsActive,
		IsDefault:             c.IsDefault,
		IsAvailableToProjects: c.IsAvailableToProjects,
	}
	settings, err := jsonBytesToMap(c.SettingsJson, "embedding_provider_config.settings_json")
	if err != nil {
		return domain.EmbeddingProviderConfig{}, err
	}
	d.Settings = settings
	if c.ID.Valid {
		d.ID = PgUUIDToString(c.ID)
	}
	if c.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(c.ProjectID)
	}
	if c.CreatedAt.Valid {
		d.CreatedAt = c.CreatedAt.Time
	}
	if c.UpdatedAt.Valid {
		d.UpdatedAt = c.UpdatedAt.Time
	}
	return d, nil
}

// DBAvailableEmbeddingProviderConfigToDomain converts a sqlc list row to a domain model.
func DBAvailableEmbeddingProviderConfigToDomain(c db.ListAvailableGlobalEmbeddingProviderConfigsRow) (domain.EmbeddingProviderConfig, error) {
	d := domain.EmbeddingProviderConfig{
		Name:                  c.Name,
		Provider:              c.Provider,
		EndpointURL:           c.EndpointUrl,
		Model:                 c.Model,
		Dimensions:            int(c.Dimensions),
		MaxTokens:             int(c.MaxTokens),
		HasCredentials:        c.HasCredentials,
		IsActive:              c.IsActive,
		IsDefault:             c.IsDefault,
		IsAvailableToProjects: c.IsAvailableToProjects,
	}
	settings, err := jsonBytesToMap(c.SettingsJson, "available_embedding_provider_config.settings_json")
	if err != nil {
		return domain.EmbeddingProviderConfig{}, err
	}
	d.Settings = settings
	if c.ID.Valid {
		d.ID = PgUUIDToString(c.ID)
	}
	if c.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(c.ProjectID)
	}
	if c.CreatedAt.Valid {
		d.CreatedAt = c.CreatedAt.Time
	}
	if c.UpdatedAt.Valid {
		d.UpdatedAt = c.UpdatedAt.Time
	}
	return d, nil
}

// DBLLMProviderConfigToDomain converts a sqlc LlmProviderConfig to a domain model.
func DBLLMProviderConfigToDomain(c db.LlmProviderConfig) (domain.LLMProviderConfig, error) {
	d := domain.LLMProviderConfig{
		Name:                  c.Name,
		Provider:              c.Provider,
		EndpointURL:           c.EndpointUrl,
		HasCredentials:        len(c.CredentialsEncrypted) > 0,
		IsActive:              c.IsActive,
		IsDefault:             c.IsDefault,
		IsAvailableToProjects: c.IsAvailableToProjects,
	}
	settings, err := jsonBytesToMap(c.SettingsJson, "llm_provider_config.settings_json")
	if err != nil {
		return domain.LLMProviderConfig{}, err
	}
	d.Settings = settings
	if c.Model.Valid {
		d.Model = c.Model.String
	}
	if c.ID.Valid {
		d.ID = PgUUIDToString(c.ID)
	}
	if c.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(c.ProjectID)
	}
	if c.CreatedAt.Valid {
		d.CreatedAt = c.CreatedAt.Time
	}
	if c.UpdatedAt.Valid {
		d.UpdatedAt = c.UpdatedAt.Time
	}
	return d, nil
}

// DBAvailableLLMProviderConfigToDomain converts a sqlc list row to a domain model.
func DBAvailableLLMProviderConfigToDomain(c db.ListAvailableGlobalLLMProviderConfigsRow) (domain.LLMProviderConfig, error) {
	d := domain.LLMProviderConfig{
		Name:                  c.Name,
		Provider:              c.Provider,
		EndpointURL:           c.EndpointUrl,
		HasCredentials:        c.HasCredentials,
		IsActive:              c.IsActive,
		IsDefault:             c.IsDefault,
		IsAvailableToProjects: c.IsAvailableToProjects,
	}
	settings, err := jsonBytesToMap(c.SettingsJson, "available_llm_provider_config.settings_json")
	if err != nil {
		return domain.LLMProviderConfig{}, err
	}
	d.Settings = settings
	if c.Model.Valid {
		d.Model = c.Model.String
	}
	if c.ID.Valid {
		d.ID = PgUUIDToString(c.ID)
	}
	if c.ProjectID.Valid {
		d.ProjectID = PgUUIDToString(c.ProjectID)
	}
	if c.CreatedAt.Valid {
		d.CreatedAt = c.CreatedAt.Time
	}
	if c.UpdatedAt.Valid {
		d.UpdatedAt = c.UpdatedAt.Time
	}
	return d, nil
}

func jsonBytesToMap(b []byte, ctx string) (map[string]any, error) {
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil || out == nil {
		if err != nil {
			wrappedErr := fmt.Errorf("%s: %w", ctx, err)
			slog.Warn("dbconv: invalid JSON", slog.String("context", ctx), slog.Int("bytes", len(b)), slog.Any("error", wrappedErr))
			return nil, wrappedErr
		}
		err = fmt.Errorf("%s must contain a JSON object", ctx)
		slog.Warn("dbconv: invalid JSON", slog.String("context", ctx), slog.Int("bytes", len(b)), slog.Any("error", err))
		return nil, err
	}
	return out, nil
}

// PgUUIDToString formats a pgtype.UUID as a standard UUID string.
// Returns an empty string if the UUID is not valid.
func PgUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// StringToPgUUID parses a UUID string into a pgtype.UUID.
func StringToPgUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

// TimeToPgTimestamptz converts a *time.Time to a pgtype.Timestamptz.
// Returns an invalid (NULL) Timestamptz if t is nil.
func TimeToPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
