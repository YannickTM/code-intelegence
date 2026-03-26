// Package embedding provides embedding version resolution and an Ollama
// embedding client for the worker storage pipeline.
package embedding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/execution"
	db "myjungle/datastore/postgres/sqlc"
)

// VersionLabel builds a deterministic version label from embedding config parts.
// Format: "{provider}-{model}-{dimensions}".
// Example: "ollama-jina/jina-embeddings-v2-base-en-768".
//
// Slashes in model names (e.g. "jina/jina-embeddings-v2-base-en") are replaced
// with hyphens so the label is safe for use in Qdrant collection names and URLs.
func VersionLabel(provider, model string, dimensions int32) string {
	safeModel := strings.ReplaceAll(model, "/", "-")
	return fmt.Sprintf("%s-%s-%d", provider, safeModel, dimensions)
}

// ResolveVersion looks up an embedding version by its deterministic label.
// If no row exists, it creates one. A unique-violation race (concurrent
// creation by another worker) is handled by retrying the lookup.
func ResolveVersion(ctx context.Context, q db.Querier, embCfg execution.EmbeddingConfig) (db.EmbeddingVersion, error) {
	label := VersionLabel(embCfg.Provider, embCfg.Model, embCfg.Dimensions)

	ver, err := q.GetEmbeddingVersionByLabel(ctx, label)
	if err == nil {
		return ver, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.EmbeddingVersion{}, fmt.Errorf("embedding: lookup version %q: %w", label, err)
	}

	// Not found — create.
	ver, err = q.CreateEmbeddingVersion(ctx, db.CreateEmbeddingVersionParams{
		EmbeddingProviderConfigID: embCfg.ID,
		Provider:                  embCfg.Provider,
		Model:                     embCfg.Model,
		Dimensions:                embCfg.Dimensions,
		VersionLabel:              label,
	})
	if err == nil {
		return ver, nil
	}

	// If we hit a unique violation (race with another worker), retry lookup.
	if isUniqueViolation(err) {
		ver, err = q.GetEmbeddingVersionByLabel(ctx, label)
		if err != nil {
			return db.EmbeddingVersion{}, fmt.Errorf("embedding: retry lookup version %q: %w", label, err)
		}
		return ver, nil
	}

	return db.EmbeddingVersion{}, fmt.Errorf("embedding: create version %q: %w", label, err)
}

// isUniqueViolation checks whether err is a PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	// pgx wraps PostgreSQL errors as *pgconn.PgError.
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}

// UUIDFromString parses a hex UUID string into a pgtype.UUID.
// Returns an invalid UUID if the string is empty.
// Returns an error if the string is non-empty but malformed.
func UUIDFromString(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if s == "" {
		return u, nil
	}
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("embedding: invalid UUID %q: %w", s, err)
	}
	return u, nil
}
