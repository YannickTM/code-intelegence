// Package providersetting contains shared helpers and types
// used by both the embedding and LLM provider-setting services.
package providersetting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/providers"
	"myjungle/backend-api/internal/secrets"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CredentialsUpdate preserves the distinction between omitted and explicit
// credential updates. Both the embedding and LLM services use this type.
type CredentialsUpdate struct {
	Set   bool
	Clear bool
	Data  map[string]any
}

// LockProjectRow acquires a row-level advisory lock on the project to
// serialise concurrent writes for provider settings.
func LockProjectRow(ctx context.Context, q *db.Queries, projectUUID pgtype.UUID) error {
	if _, err := q.LockProjectRow(ctx, projectUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("project not found")
		}
		return fmt.Errorf("lock project row: %w", err)
	}
	return nil
}

// MarshalJSONObject returns a safe JSON encoding of a map, defaulting nil
// to `{}`.
func MarshalJSONObject(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	out, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ResolveCredentials computes the ciphertext to store for credentials based
// on the update payload and the currently stored ciphertext.
//
// Logic:
//   - update.Set == false → carry forward currentEncrypted (or nil)
//   - update.Set && update.Clear → clear credentials
//   - update.Set && data present → encrypt data
func ResolveCredentials(update CredentialsUpdate, currentEncrypted []byte, hasCurrent bool, secretKey []byte) ([]byte, error) {
	if !update.Set {
		if hasCurrent {
			return currentEncrypted, nil
		}
		return nil, nil
	}
	if update.Clear {
		return nil, nil
	}
	if len(update.Data) == 0 {
		return nil, nil
	}
	payload, err := MarshalJSONObject(update.Data)
	if err != nil {
		return nil, domain.BadRequest("credentials must be a valid JSON object")
	}
	encrypted, err := secrets.Encrypt(payload, secretKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt credentials: %w", err)
	}
	return encrypted, nil
}

// ValidateBaseFields validates the shared fields for a custom config update
// (name, provider, endpoint_url, settings). It normalises values in place.
// Callers must validate capability-specific fields (model, dimensions) separately.
func ValidateBaseFields(name *string, providerID *string, endpointURL *string, settings *map[string]any, capability providers.Capability) error {
	if name == nil {
		return domain.BadRequest("name is required")
	}
	if providerID == nil {
		return domain.BadRequest("provider is required")
	}
	if endpointURL == nil {
		return domain.BadRequest("endpoint_url is required")
	}
	if settings == nil {
		return domain.BadRequest("settings must be provided")
	}
	*name = strings.TrimSpace(*name)
	if *name == "" {
		return domain.BadRequest("name is required")
	}
	*providerID = providers.NormalizeProviderID(*providerID)
	if err := providers.ValidateProvider(capability, *providerID); err != nil {
		return domain.BadRequest(err.Error())
	}
	*endpointURL = strings.TrimSpace(*endpointURL)
	if *endpointURL == "" {
		return domain.BadRequest("endpoint_url is required")
	}
	normalizedEndpointURL, err := providers.ValidateEndpointURL(*endpointURL)
	if err != nil {
		return domain.BadRequest(err.Error())
	}
	*endpointURL = normalizedEndpointURL
	if *settings == nil {
		*settings = map[string]any{}
	}
	return nil
}
