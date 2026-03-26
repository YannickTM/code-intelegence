// Package commits persists git commit history into the database
// so the commit browser API has data to serve.
package commits

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/gitclient"
	"myjungle/backend-worker/internal/logger"
	db "myjungle/datastore/postgres/sqlc"
)

// commitLogProvider abstracts the git log extraction methods used by the indexer.
type commitLogProvider interface {
	LogCommits(ctx context.Context, repoDir, sinceCommit string, maxCount int) ([]gitclient.CommitLog, error)
	DiffStatLog(ctx context.Context, repoDir, sinceCommit string, maxCount int) (map[string][]gitclient.FileDiffEntry, error)
}

// Indexer orchestrates persisting commit history from git into the database.
type Indexer struct {
	queries db.Querier
	git     commitLogProvider
}

// Result summarises what the indexer persisted.
type Result struct {
	CommitsIndexed int
	DiffsIndexed   int
	HeadCommitDBID pgtype.UUID
}

// New creates a commit indexer.
func New(q db.Querier, g commitLogProvider) *Indexer {
	return &Indexer{queries: q, git: g}
}

// IndexAll indexes full commit history (up to maxCommits) for a project.
func (ix *Indexer) IndexAll(ctx context.Context, projectID pgtype.UUID, repoDir string, maxCommits int) (*Result, error) {
	commits, err := ix.git.LogCommits(ctx, repoDir, "", maxCommits)
	if err != nil {
		return nil, fmt.Errorf("commits: log commits: %w", err)
	}
	diffMap, err := ix.git.DiffStatLog(ctx, repoDir, "", maxCommits)
	if err != nil {
		return nil, fmt.Errorf("commits: diff stat log: %w", err)
	}
	return ix.indexCommits(ctx, projectID, commits, diffMap)
}

// IndexSince indexes only commits newer than sinceCommit.
func (ix *Indexer) IndexSince(ctx context.Context, projectID pgtype.UUID, repoDir, sinceCommit string) (*Result, error) {
	commits, err := ix.git.LogCommits(ctx, repoDir, sinceCommit, 0)
	if err != nil {
		return nil, fmt.Errorf("commits: log commits (since %s): %w", sinceCommit, err)
	}
	diffMap, err := ix.git.DiffStatLog(ctx, repoDir, sinceCommit, 0)
	if err != nil {
		return nil, fmt.Errorf("commits: diff stat log (since %s): %w", sinceCommit, err)
	}
	return ix.indexCommits(ctx, projectID, commits, diffMap)
}

// indexCommits is the shared core that inserts commits, parents, and file diffs.
func (ix *Indexer) indexCommits(
	ctx context.Context,
	projectID pgtype.UUID,
	commits []gitclient.CommitLog,
	diffMap map[string][]gitclient.FileDiffEntry,
) (*Result, error) {
	if len(commits) == 0 {
		return &Result{}, nil
	}

	log := logger.FromContext(ctx)
	result := &Result{}
	hashToDBID := make(map[string]pgtype.UUID, len(commits))

	// Phase 1: Insert all commits (build hashToDBID map).
	for _, cm := range commits {
		dbCommit, err := ix.queries.InsertCommit(ctx, db.InsertCommitParams{
			ProjectID:      projectID,
			CommitHash:     cm.Hash,
			AuthorName:     cm.AuthorName,
			AuthorEmail:    cm.AuthorEmail,
			AuthorDate:     toPgTimestamptz(cm.AuthorDate),
			CommitterName:  cm.CommitterName,
			CommitterEmail: cm.CommitterEmail,
			CommitterDate:  toPgTimestamptz(cm.CommitterDate),
			Message:        cm.Message,
		})
		if err != nil {
			return nil, fmt.Errorf("commits: insert commit %s: %w", cm.Hash, err)
		}
		hashToDBID[cm.Hash] = dbCommit.ID
		result.CommitsIndexed++
	}

	// Phase 2: Insert parent relationships.
	for _, cm := range commits {
		commitDBID := hashToDBID[cm.Hash]
		for i, parentHash := range cm.ParentHashes {
			parentDBID, ok := hashToDBID[parentHash]
			if !ok {
				// Parent not in current batch — try DB lookup.
				dbParent, err := ix.queries.GetCommitByHash(ctx, db.GetCommitByHashParams{
					ProjectID:  projectID,
					CommitHash: parentHash,
				})
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						// Parent outside indexed range — skip.
						log.Debug("commits: parent not found, skipping",
							slog.String("commit", cm.Hash),
							slog.String("parent", parentHash))
						continue
					}
					return nil, fmt.Errorf("commits: lookup parent %s: %w", parentHash, err)
				}
				parentDBID = dbParent.ID
			}
			if err := ix.queries.InsertCommitParent(ctx, db.InsertCommitParentParams{
				ProjectID:      projectID,
				CommitID:       commitDBID,
				ParentCommitID: parentDBID,
				Ordinal:        int32(i),
			}); err != nil {
				return nil, fmt.Errorf("commits: insert parent %s→%s: %w", cm.Hash, parentHash, err)
			}
		}
	}

	// Phase 3: Insert file diffs.
	for _, cm := range commits {
		commitDBID := hashToDBID[cm.Hash]
		diffs := diffMap[cm.Hash]

		// Resolve parent commit DB ID for file diffs (first parent).
		var parentCommitDBID pgtype.UUID // zero value = invalid
		if len(cm.ParentHashes) > 0 {
			if pid, ok := hashToDBID[cm.ParentHashes[0]]; ok {
				parentCommitDBID = pid
			} else {
				// Try DB lookup for parent.
				dbParent, err := ix.queries.GetCommitByHash(ctx, db.GetCommitByHashParams{
					ProjectID:  projectID,
					CommitHash: cm.ParentHashes[0],
				})
				if err != nil {
					if !errors.Is(err, pgx.ErrNoRows) {
						return nil, fmt.Errorf("commits: lookup file-diff parent %s: %w", cm.ParentHashes[0], err)
					}
					// ErrNoRows: parent outside indexed range — leave as zero UUID.
					log.Debug("commits: file-diff parent not found, leaving unlinked",
						slog.String("commit", cm.Hash),
						slog.String("parent", cm.ParentHashes[0]))
				} else {
					parentCommitDBID = dbParent.ID
				}
			}
		}

		for _, d := range diffs {
			changeType := mapChangeType(d.Status)

			// old_file_path: NULL for added files, path otherwise.
			var oldPath pgtype.Text
			if d.Status != "A" {
				oldPath = pgtype.Text{String: d.Path, Valid: true}
			}

			// new_file_path: NULL for deleted files, path otherwise.
			var newPath pgtype.Text
			if d.Status != "D" {
				newPath = pgtype.Text{String: d.Path, Valid: true}
			}

			if _, err := ix.queries.InsertCommitFileDiff(ctx, db.InsertCommitFileDiffParams{
				ProjectID:      projectID,
				CommitID:       commitDBID,
				ParentCommitID: parentCommitDBID,
				OldFilePath:    oldPath,
				NewFilePath:    newPath,
				ChangeType:     changeType,
				Patch:          toPgText(d.Patch),
				Additions:      int32(d.Additions),
				Deletions:      int32(d.Deletions),
			}); err != nil {
				return nil, fmt.Errorf("commits: insert file diff %s/%s: %w", cm.Hash, d.Path, err)
			}
			result.DiffsIndexed++
		}
	}

	// HEAD commit is first in the list (newest-first from git log).
	result.HeadCommitDBID = hashToDBID[commits[0].Hash]

	return result, nil
}

// toPgTimestamptz converts a time.Time to pgtype.Timestamptz.
func toPgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// toPgText converts a Go string to pgtype.Text. Empty strings become NULL.
func toPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// mapChangeType maps git status codes to human-readable change types.
func mapChangeType(status string) string {
	switch status {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "deleted"
	default:
		return "modified"
	}
}
