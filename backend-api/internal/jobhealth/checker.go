// Package jobhealth provides lazy, on-demand health checks for running
// indexing jobs. When an API read path encounters a "running" job, the
// Checker verifies the worker heartbeat in Redis and transitions the job
// to "failed" inline if the worker is dead.
package jobhealth

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/redisclient"
	"myjungle/backend-api/internal/sse"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"
)

// workerStatus mirrors the heartbeat payload stored at worker:status:{id}.
type workerStatus struct {
	CurrentJobID string            `json:"current_job_id"`
	ActiveJobs   map[string]string `json:"active_jobs"`
}

// isActiveJob returns true if jobID is tracked in the heartbeat, checking
// the active_jobs map first (concurrent-aware) and falling back to
// current_job_id for backward compatibility with older workers.
func (ws *workerStatus) isActiveJob(jobID string) bool {
	if len(ws.ActiveJobs) > 0 {
		_, ok := ws.ActiveJobs[jobID]
		return ok
	}
	return ws.CurrentJobID == jobID
}

// reaperErrorDetails is the JSONB error payload written when a job is reaped.
var reaperErrorDetails = []byte(`[{"category":"worker_crash","message":"Worker process became unreachable during indexing. The job was automatically marked as failed.","step":"reaper"}]`)

// Checker verifies worker liveness on running jobs and reaps dead ones.
// Nil-safe: all methods are no-ops on a nil *Checker.
type Checker struct {
	redis          *redisclient.Reader
	queries        db.Querier
	evtPublisher   *sse.EventPublisher
	staleThreshold time.Duration
}

// NewChecker creates a Checker backed by the given Redis reader, database
// queries, SSE event publisher, and stale threshold duration.
func NewChecker(
	redis *redisclient.Reader,
	pdb *postgres.DB,
	evtPublisher *sse.EventPublisher,
	staleThreshold time.Duration,
) *Checker {
	return &Checker{
		redis:          redis,
		queries:        pdb.Queries,
		evtPublisher:   evtPublisher,
		staleThreshold: staleThreshold,
	}
}

// CheckAndReapIfDead examines a running job's worker heartbeat.
// If the worker is dead, it transitions the job to failed and returns "failed".
// On any error or if the worker is alive, it returns the original status.
//
// Fast paths (no Redis call):
//   - currentStatus != "running"
//   - job started less than staleThreshold ago
//   - workerID is empty AND job is not stale (legacy job without worker tracking)
//
// This is best-effort: errors are logged but never propagated to the caller.
func (c *Checker) CheckAndReapIfDead(
	ctx context.Context,
	jobID string,
	workerID string,
	currentStatus string,
	startedAt time.Time,
	projectID string,
	snapshotID string,
) string {
	if c == nil {
		return currentStatus
	}
	if currentStatus != "running" {
		return currentStatus
	}
	if !startedAt.IsZero() && time.Since(startedAt) < c.staleThreshold {
		return currentStatus
	}
	if workerID == "" {
		// Legacy job without worker tracking that has exceeded the stale
		// threshold. No heartbeat to check — treat as dead.
		return c.reap(ctx, jobID, projectID, snapshotID)
	}

	// Slow path: check worker heartbeat in Redis.
	raw, err := c.redis.GetJSON(ctx, "worker:status:"+workerID)
	if err != nil {
		slog.WarnContext(ctx, "jobhealth: redis heartbeat check failed, skipping reap",
			slog.String("job_id", jobID),
			slog.String("worker_id", workerID),
			slog.Any("error", err))
		return currentStatus
	}

	// Key exists — check if worker is still working on this job.
	if raw != "" {
		var ws workerStatus
		if err := json.Unmarshal([]byte(raw), &ws); err != nil {
			// Corrupted payload — assume alive to avoid false positives.
			slog.WarnContext(ctx, "jobhealth: malformed heartbeat payload, skipping reap",
				slog.String("job_id", jobID),
				slog.String("worker_id", workerID),
				slog.Any("error", err))
			return currentStatus
		}
		if ws.isActiveJob(jobID) {
			return "running" // worker alive and working on this job
		}
		// Worker moved on to another job — this job is orphaned.
	}

	// Worker is dead or moved on. Reap the job.
	return c.reap(ctx, jobID, projectID, snapshotID)
}

// reap transitions the job to failed, cleans up orphaned snapshots,
// and publishes an SSE event.
func (c *Checker) reap(ctx context.Context, jobID, projectID, snapshotID string) string {
	pgJobID, err := dbconv.StringToPgUUID(jobID)
	if err != nil {
		slog.WarnContext(ctx, "jobhealth: invalid job UUID, skipping reap",
			slog.String("job_id", jobID), slog.Any("error", err))
		return "running"
	}

	rows, err := c.queries.FailStaleJob(ctx, db.FailStaleJobParams{
		ID:           pgJobID,
		ErrorDetails: reaperErrorDetails,
	})
	if err != nil {
		slog.WarnContext(ctx, "jobhealth: FailStaleJob query failed, skipping reap",
			slog.String("job_id", jobID), slog.Any("error", err))
		return "running"
	}

	if rows == 0 {
		// Already reaped by a concurrent check — skip cleanup and SSE.
		return "failed"
	}

	slog.InfoContext(ctx, "jobhealth: reaped dead-worker job",
		slog.String("job_id", jobID),
		slog.String("project_id", projectID))

	// Clean up orphaned building snapshot.
	if snapshotID != "" {
		pgSnapID, err := dbconv.StringToPgUUID(snapshotID)
		if err == nil {
			if delErr := c.queries.DeleteOrphanedBuildingSnapshot(ctx, pgSnapID); delErr != nil {
				slog.WarnContext(ctx, "jobhealth: snapshot cleanup failed",
					slog.String("snapshot_id", snapshotID), slog.Any("error", delErr))
			}
		}
	}

	// Publish job:failed SSE event.
	c.evtPublisher.Publish(ctx, sse.SSEEvent{
		Event:     "job:failed",
		ProjectID: projectID,
		JobID:     jobID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"status": "failed",
			"reason": "worker_crash",
		},
	})

	return "failed"
}
