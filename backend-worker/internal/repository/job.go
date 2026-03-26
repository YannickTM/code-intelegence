// Package repository provides data access for workflow handlers.
package repository

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/sshkey"
	db "myjungle/datastore/postgres/sqlc"
)

// ErrJobNotQueued is returned when ClaimJob finds the job is no longer in
// "queued" status — it was already claimed, completed, or failed. This lets
// callers distinguish "stale retry" from transient DB errors.
var ErrJobNotQueued = errors.New("job not in queued status")

// ErrProjectLocked is returned when the project-level advisory lock
// cannot be acquired because another worker holds it.
var ErrProjectLocked = errors.New("repository: project is locked by another worker")

// PinnedConn holds a db.Querier pinned to a dedicated database connection
// and functions to return or destroy the connection.
type PinnedConn struct {
	Querier db.Querier
	// Release returns the connection to the pool for reuse.
	Release func()
	// Destroy removes the connection from the pool and closes it.
	// Use when session state (e.g. advisory locks) could not be cleaned up.
	Destroy func()
}

// PinnedQueryFunc acquires a dedicated database connection and returns a
// PinnedConn. The caller MUST call either Release or Destroy when done.
// This is required for session-scoped operations like advisory locks,
// which must acquire and release on the same connection.
type PinnedQueryFunc func(ctx context.Context) (*PinnedConn, error)

// JobRepository loads execution context and updates job state.
type JobRepository struct {
	queries       db.Querier
	decryptionKey []byte
	acquirePinned PinnedQueryFunc
}

// New creates a JobRepository with the given Querier, SSH encryption secret,
// and a function to acquire pinned database connections for advisory locks.
// It returns an error if encryptionSecret is empty or acquirePinned is nil.
func New(queries db.Querier, encryptionSecret string, acquirePinned PinnedQueryFunc) (*JobRepository, error) {
	if encryptionSecret == "" {
		return nil, errors.New("repository: SSH encryption secret must not be empty")
	}
	if acquirePinned == nil {
		return nil, errors.New("repository: acquirePinned function must not be nil")
	}
	return &JobRepository{
		queries:       queries,
		decryptionKey: sshkey.DeriveKey(encryptionSecret),
		acquirePinned: acquirePinned,
	}, nil
}

// LoadExecutionContext loads all data needed to execute an indexing job.
// It validates that the project exists, an active SSH key assignment exists,
// the private key can be decrypted, and the embedding provider config exists.
//
// This method is read-only — it does NOT check or change job status.
// Use ClaimJob to atomically transition a queued job to running.
func (r *JobRepository) LoadExecutionContext(ctx context.Context, jobID pgtype.UUID) (*execution.Context, error) {
	job, err := r.queries.GetIndexingJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("repository: load job %s: %w", fmtUUID(jobID), err)
	}

	sshRow, err := r.queries.GetProjectSSHKeyForExecution(ctx, job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("repository: load project SSH key for project %s: %w", fmtUUID(job.ProjectID), err)
	}

	privateKey, err := sshkey.Decrypt(sshRow.PrivateKeyEncrypted, r.decryptionKey)
	if err != nil {
		return nil, fmt.Errorf("repository: decrypt SSH key %s: %w", fmtUUID(sshRow.ID), err)
	}

	if !job.EmbeddingProviderConfigID.Valid {
		return nil, fmt.Errorf("repository: job %s has no embedding provider config ID", fmtUUID(jobID))
	}

	embCfg, err := r.queries.GetEmbeddingProviderConfigByID(ctx, job.EmbeddingProviderConfigID)
	if err != nil {
		return nil, fmt.Errorf("repository: load embedding config %s: %w", fmtUUID(job.EmbeddingProviderConfigID), err)
	}

	// Use snapshot branch if the job references a snapshot, otherwise fall
	// back to the project's default branch from the SSH key query.
	branch := sshRow.DefaultBranch
	if job.IndexSnapshotID.Valid {
		snap, err := r.queries.GetIndexSnapshot(ctx, job.IndexSnapshotID)
		if err != nil {
			return nil, fmt.Errorf("repository: load index snapshot %s: %w", fmtUUID(job.IndexSnapshotID), err)
		}
		if snap.Branch != "" {
			branch = snap.Branch
		}
	}

	return &execution.Context{
		JobID:         job.ID,
		JobType:       job.JobType,
		ProjectID:     job.ProjectID,
		RepoURL:       sshRow.RepoUrl,
		Branch:        branch,
		SSHKeyID:      sshRow.ID,
		SSHPrivateKey: privateKey,
		Embedding: execution.EmbeddingConfig{
			ID:          embCfg.ID,
			Provider:    embCfg.Provider,
			EndpointURL: embCfg.EndpointUrl,
			Model:       embCfg.Model,
			Dimensions:  embCfg.Dimensions,
			MaxTokens:   embCfg.MaxTokens,
		},
	}, nil
}

// ClaimJob atomically transitions a job from "queued" to "running" and
// stamps it with the claiming worker's ID. Returns an error if the job
// was not in "queued" status (e.g. already claimed by another worker).
func (r *JobRepository) ClaimJob(ctx context.Context, jobID pgtype.UUID, workerID string) error {
	rows, err := r.queries.ClaimQueuedIndexingJob(ctx, db.ClaimQueuedIndexingJobParams{
		ID:       jobID,
		WorkerID: pgtype.Text{String: workerID, Valid: workerID != ""},
	})
	if err != nil {
		return fmt.Errorf("repository: claim job %s: %w", fmtUUID(jobID), err)
	}
	if rows == 0 {
		return fmt.Errorf("repository: claim job %s: %w", fmtUUID(jobID), ErrJobNotQueued)
	}
	return nil
}

// CreateSnapshot creates a new index snapshot.
func (r *JobRepository) CreateSnapshot(ctx context.Context, params db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
	snap, err := r.queries.CreateIndexSnapshot(ctx, params)
	if err != nil {
		return db.IndexSnapshot{}, fmt.Errorf("repository: create snapshot: %w", err)
	}
	return snap, nil
}

// SetJobRunning transitions a job to "running" status.
func (r *JobRepository) SetJobRunning(ctx context.Context, jobID pgtype.UUID) error {
	if err := r.queries.SetIndexingJobRunning(ctx, jobID); err != nil {
		return fmt.Errorf("repository: set job running %s: %w", fmtUUID(jobID), err)
	}
	return nil
}

// SetJobProgress updates the progress counters on a job.
func (r *JobRepository) SetJobProgress(ctx context.Context, jobID pgtype.UUID, files, chunks, vectors int32) error {
	err := r.queries.SetIndexingJobProgress(ctx, db.SetIndexingJobProgressParams{
		ID:             jobID,
		FilesProcessed: files,
		ChunksUpserted: chunks,
		VectorsDeleted: vectors,
	})
	if err != nil {
		return fmt.Errorf("repository: set job progress %s: %w", fmtUUID(jobID), err)
	}
	return nil
}

// SetJobCompleted transitions a job from "running" to "completed".
// Returns true if the transition took effect, false if the job was no
// longer in "running" status (stale or already-transitioned).
func (r *JobRepository) SetJobCompleted(ctx context.Context, jobID pgtype.UUID) (bool, error) {
	rows, err := r.queries.SetIndexingJobCompletedFromRunning(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("repository: set job completed %s: %w", fmtUUID(jobID), err)
	}
	return rows > 0, nil
}

// SetJobFailed transitions a job from "running" to "failed" with error details.
// Returns true if the transition took effect, false if the job was no
// longer in "running" status.
func (r *JobRepository) SetJobFailed(ctx context.Context, jobID pgtype.UUID, errorDetails []byte) (bool, error) {
	rows, err := r.queries.SetIndexingJobFailedFromRunning(ctx, db.SetIndexingJobFailedFromRunningParams{
		ID:           jobID,
		ErrorDetails: errorDetails,
	})
	if err != nil {
		return false, fmt.Errorf("repository: set job failed %s: %w", fmtUUID(jobID), err)
	}
	return rows > 0, nil
}

// TryProjectLock attempts to acquire a session-level PostgreSQL advisory
// lock keyed by project ID. It acquires a dedicated database connection
// from the pool to ensure the lock and unlock happen on the same session.
// On success it returns an unlock function that MUST be called to release
// the lock and return the connection to the pool. If the lock is already
// held by another session, it returns ErrProjectLocked.
func (r *JobRepository) TryProjectLock(ctx context.Context, projectID pgtype.UUID) (unlock func(context.Context), err error) {
	pc, err := r.acquirePinned(ctx)
	if err != nil {
		return nil, fmt.Errorf("repository: acquire connection for project lock: %w", err)
	}

	lockKeys := advisoryLockKeys(projectID)
	acquired, err := pc.Querier.TryAdvisoryLockForProject(ctx, lockKeys)
	if err != nil {
		pc.Release()
		return nil, fmt.Errorf("repository: try project lock %s: %w", fmtUUID(projectID), err)
	}
	if !acquired {
		pc.Release()
		return nil, ErrProjectLocked
	}
	return func(_ context.Context) {
		// Use a detached context so the unlock is not skipped when the
		// caller's context is already canceled (e.g. task timeout or
		// shutdown).
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, unlockErr := pc.Querier.ReleaseAdvisoryLockForProject(unlockCtx, db.ReleaseAdvisoryLockForProjectParams(lockKeys))
		if unlockErr != nil {
			// Unlock failed — destroy the connection so the advisory lock
			// does not leak onto a pooled connection.
			slog.Error("repository: failed to release project lock, destroying connection",
				slog.String("project_id", fmtUUID(projectID)),
				slog.String("error", unlockErr.Error()))
			pc.Destroy()
			return
		}
		pc.Release()
	}, nil
}

// advisoryLockKeys derives two int32 keys from a UUID for the two-argument
// form of pg_try_advisory_lock(key1, key2). This avoids hash collisions
// that would occur with single-key hashtext().
func advisoryLockKeys(id pgtype.UUID) db.TryAdvisoryLockForProjectParams {
	return db.TryAdvisoryLockForProjectParams{
		Column1: int32(binary.BigEndian.Uint32(id.Bytes[0:4])),
		Column2: int32(binary.BigEndian.Uint32(id.Bytes[4:8])),
	}
}

// ActivateSnapshot deactivates the current active snapshot for the
// (project, branch) pair and then activates the specified one.
//
// NOTE: Production code should use indexing.ActivateSnapshotTx instead,
// which wraps both operations in a database transaction. This method is
// retained for test convenience with mock queriers.
func (r *JobRepository) ActivateSnapshot(ctx context.Context, projectID pgtype.UUID, branch string, snapshotID pgtype.UUID) error {
	if err := r.queries.DeactivateActiveSnapshot(ctx, db.DeactivateActiveSnapshotParams{
		ProjectID: projectID,
		Branch:    branch,
	}); err != nil {
		return fmt.Errorf("repository: deactivate snapshot for %s: %w", fmtUUID(projectID), err)
	}
	if err := r.queries.ActivateSnapshot(ctx, snapshotID); err != nil {
		return fmt.Errorf("repository: activate snapshot %s: %w", fmtUUID(snapshotID), err)
	}
	return nil
}

// fmtUUID formats a pgtype.UUID for error messages.
func fmtUUID(u pgtype.UUID) string {
	if !u.Valid {
		return "<nil>"
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
