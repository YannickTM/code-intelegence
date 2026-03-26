package apikey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// maxProjectKeys is the maximum number of active API keys per project.
const maxProjectKeys = 50

// maxPersonalKeys is the maximum number of active personal API keys per user.
const maxPersonalKeys = 50

// CreateKeyRequest is the input for creating an API key (both types).
type CreateKeyRequest struct {
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// CreateKeyResult is returned on successful key creation.
// PlaintextKey is shown once and never stored.
type CreateKeyResult struct {
	domain.APIKeyInfo
	PlaintextKey string `json:"plaintext_key"`
}

// Service provides API key business logic.
type Service struct {
	db *postgres.DB
}

// NewService creates a new API key service.
// Returns nil when database is nil so the handler's ensureSvc guard can catch it.
func NewService(database *postgres.DB) *Service {
	if database == nil {
		return nil
	}
	return &Service{db: database}
}

// ---------- Project key operations ----------

// CreateProjectKey generates and stores a new project-scoped API key.
func (s *Service) CreateProjectKey(ctx context.Context, req CreateKeyRequest, creatorID, projectID string) (*CreateKeyResult, error) {
	role := req.Role
	if role == "" {
		role = domain.KeyRoleRead
	}
	if !domain.KeyRoleValid(role) {
		return nil, domain.BadRequest(fmt.Sprintf("invalid role: %q", role))
	}

	creatorUUID, err := dbconv.StringToPgUUID(creatorID)
	if err != nil {
		return nil, fmt.Errorf("parse creator id: %w", err)
	}
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("parse project id: %w", err)
	}

	plaintext, prefix, hash, err := GenerateAPIKey(domain.KeyTypeProject)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	var row db.ApiKey
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		// Lock the project row to serialize concurrent key creation.
		if _, lockErr := q.LockProjectRow(ctx, projectUUID); lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return domain.NotFound("project not found")
			}
			return fmt.Errorf("lock project: %w", lockErr)
		}

		count, countErr := q.CountProjectKeys(ctx, projectUUID)
		if countErr != nil {
			return fmt.Errorf("count project keys: %w", countErr)
		}
		if count >= int64(maxProjectKeys) {
			return domain.BadRequest(fmt.Sprintf("project key limit reached (max %d)", maxProjectKeys))
		}

		var insertErr error
		row, insertErr = q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
			KeyType:   domain.KeyTypeProject,
			KeyPrefix: prefix,
			KeyHash:   hash,
			Name:      req.Name,
			Role:      role,
			ProjectID: projectUUID,
			CreatedBy: creatorUUID,
			ExpiresAt: dbconv.TimeToPgTimestamptz(req.ExpiresAt),
		})
		return insertErr
	})
	if err != nil {
		return nil, err
	}

	info := dbconv.DBAPIKeyToDomain(row)
	return &CreateKeyResult{APIKeyInfo: info, PlaintextKey: plaintext}, nil
}

// ListProjectKeys returns all active API keys for a project.
func (s *Service) ListProjectKeys(ctx context.Context, projectID string) ([]domain.APIKeyInfo, error) {
	projectUUID, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("parse project id: %w", err)
	}

	rows, err := s.db.Queries.ListProjectKeys(ctx, projectUUID)
	if err != nil {
		return nil, fmt.Errorf("list project keys: %w", err)
	}

	keys := make([]domain.APIKeyInfo, len(rows))
	for i, row := range rows {
		keys[i] = dbconv.DBAPIKeyToDomain(row)
	}
	return keys, nil
}

// DeleteProjectKey soft-deletes a project key after verifying it belongs to the project.
func (s *Service) DeleteProjectKey(ctx context.Context, keyID, projectID string) error {
	keyUUID, err := dbconv.StringToPgUUID(keyID)
	if err != nil {
		return domain.NotFound("api key not found")
	}

	return s.db.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.GetAPIKeyByID(ctx, keyUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("api key not found")
			}
			return fmt.Errorf("get api key: %w", err)
		}

		// Verify it's a project key belonging to this project.
		if row.KeyType != domain.KeyTypeProject {
			return domain.NotFound("api key not found")
		}
		if dbconv.PgUUIDToString(row.ProjectID) != projectID {
			return domain.NotFound("api key not found")
		}
		if !row.IsActive {
			return domain.NotFound("api key not found")
		}

		affected, err := q.SoftDeleteAPIKey(ctx, keyUUID)
		if err != nil {
			return fmt.Errorf("soft delete api key: %w", err)
		}
		if affected == 0 {
			return domain.NotFound("api key not found")
		}
		return nil
	})
}

// ---------- Personal key operations ----------

// CreatePersonalKey generates and stores a new personal API key.
func (s *Service) CreatePersonalKey(ctx context.Context, req CreateKeyRequest, userID string) (*CreateKeyResult, error) {
	role := req.Role
	if role == "" {
		role = domain.KeyRoleRead
	}
	if !domain.KeyRoleValid(role) {
		return nil, domain.BadRequest(fmt.Sprintf("invalid role: %q", role))
	}

	userUUID, err := dbconv.StringToPgUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	plaintext, prefix, hash, err := GenerateAPIKey(domain.KeyTypePersonal)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	var row db.ApiKey
	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		// Lock the user row to serialize concurrent key creation.
		if _, lockErr := q.LockUserRow(ctx, userUUID); lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return domain.NotFound("user not found")
			}
			return fmt.Errorf("lock user: %w", lockErr)
		}

		count, countErr := q.CountPersonalKeys(ctx, userUUID)
		if countErr != nil {
			return fmt.Errorf("count personal keys: %w", countErr)
		}
		if count >= int64(maxPersonalKeys) {
			return domain.BadRequest(fmt.Sprintf("personal key limit reached (max %d)", maxPersonalKeys))
		}

		var insertErr error
		row, insertErr = q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
			KeyType:   domain.KeyTypePersonal,
			KeyPrefix: prefix,
			KeyHash:   hash,
			Name:      req.Name,
			Role:      role,
			ProjectID: pgtype.UUID{}, // NULL for personal keys
			CreatedBy: userUUID,
			ExpiresAt: dbconv.TimeToPgTimestamptz(req.ExpiresAt),
		})
		return insertErr
	})
	if err != nil {
		return nil, err
	}

	info := dbconv.DBAPIKeyToDomain(row)
	return &CreateKeyResult{APIKeyInfo: info, PlaintextKey: plaintext}, nil
}

// ListPersonalKeys returns all active personal keys for a user.
func (s *Service) ListPersonalKeys(ctx context.Context, userID string) ([]domain.APIKeyInfo, error) {
	userUUID, err := dbconv.StringToPgUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	rows, err := s.db.Queries.ListPersonalKeys(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("list personal keys: %w", err)
	}

	keys := make([]domain.APIKeyInfo, len(rows))
	for i, row := range rows {
		keys[i] = dbconv.DBAPIKeyToDomain(row)
	}
	return keys, nil
}

// DeletePersonalKey soft-deletes a personal key after verifying ownership.
func (s *Service) DeletePersonalKey(ctx context.Context, keyID, userID string) error {
	keyUUID, err := dbconv.StringToPgUUID(keyID)
	if err != nil {
		return domain.NotFound("api key not found")
	}

	return s.db.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.GetAPIKeyByID(ctx, keyUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("api key not found")
			}
			return fmt.Errorf("get api key: %w", err)
		}

		// Verify it's a personal key owned by this user.
		if row.KeyType != domain.KeyTypePersonal {
			return domain.NotFound("api key not found")
		}
		if dbconv.PgUUIDToString(row.CreatedBy) != userID {
			return domain.NotFound("api key not found")
		}
		if !row.IsActive {
			return domain.NotFound("api key not found")
		}

		affected, err := q.SoftDeleteAPIKey(ctx, keyUUID)
		if err != nil {
			return fmt.Errorf("soft delete api key: %w", err)
		}
		if affected == 0 {
			return domain.NotFound("api key not found")
		}
		return nil
	})
}
