// Package vectorstore provides a Qdrant REST client for vector storage.
package vectorstore

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// CollectionName builds the Qdrant collection name for a project and
// embedding version label.
// Format: project_{hex}__emb_{label}
func CollectionName(projectID pgtype.UUID, versionLabel string) string {
	return fmt.Sprintf("project_%s__emb_%s", formatUUID(projectID), versionLabel)
}

// formatUUID returns the hex representation of a pgtype.UUID without hyphens.
func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return "00000000000000000000000000000000"
	}
	b := u.Bytes
	return fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
