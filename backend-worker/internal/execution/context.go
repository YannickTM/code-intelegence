// Package execution defines the runtime context for workflow handlers.
package execution

import "github.com/jackc/pgx/v5/pgtype"

// Context holds all data a workflow handler needs to execute a job.
type Context struct {
	JobID         pgtype.UUID
	JobType       string
	ProjectID     pgtype.UUID
	RepoURL       string
	Branch        string
	SSHKeyID      pgtype.UUID
	SSHPrivateKey []byte // decrypted PEM bytes

	Embedding EmbeddingConfig
}

// EmbeddingConfig holds the embedding provider settings for a job.
type EmbeddingConfig struct {
	ID          pgtype.UUID
	Provider    string
	EndpointURL string
	Model       string
	Dimensions  int32
	MaxTokens   int32
}
