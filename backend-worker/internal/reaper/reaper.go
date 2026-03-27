// Package reaper detects orphaned running jobs whose worker has crashed and
// transitions them to failed. It also cleans up orphaned building snapshots.
package reaper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"myjungle/backend-worker/internal/notify"
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

// Config holds reaper configuration.
type Config struct {
	StaleThreshold time.Duration
}

// Reaper scans for stuck running jobs and reaps them.
type Reaper struct {
	cfg       Config
	queries   db.Querier
	rdb       *redis.Client
	publisher *notify.EventPublisher
}

// New creates a Reaper with the given dependencies.
func New(cfg Config, queries db.Querier, rdb *redis.Client, publisher *notify.EventPublisher) *Reaper {
	return &Reaper{
		cfg:       cfg,
		queries:   queries,
		rdb:       rdb,
		publisher: publisher,
	}
}

// RunOnce performs one full reap cycle: identifies stuck jobs, fails them,
// cleans up orphaned snapshots, and publishes SSE events.
// Returns the number of jobs reaped.
func (r *Reaper) RunOnce(ctx context.Context) (int, error) {
	threshold := pgtype.Interval{
		Microseconds: int64(r.cfg.StaleThreshold / time.Microsecond),
		Valid:        true,
	}
	staleJobs, err := r.queries.ListStaleRunningJobs(ctx, threshold)
	if err != nil {
		return 0, fmt.Errorf("reaper: list stale jobs: %w", err)
	}

	reaped := 0
	for _, job := range staleJobs {
		workerID := ""
		if job.WorkerID.Valid {
			workerID = job.WorkerID.String
		}
		if r.isWorkerAlive(ctx, workerID, job.ID) {
			continue
		}

		rows, err := r.queries.FailStaleJob(ctx, db.FailStaleJobParams{
			ID:           job.ID,
			ErrorDetails: reaperErrorDetails,
		})
		if err != nil {
			slog.WarnContext(ctx, "reaper: FailStaleJob query failed",
				slog.String("job_id", fmtUUID(job.ID)),
				slog.Any("error", err))
			continue
		}
		if rows == 0 {
			continue // already reaped by a concurrent check
		}

		reaped++
		slog.WarnContext(ctx, "reaper: reaped stuck job",
			slog.String("job_id", fmtUUID(job.ID)),
			slog.String("project_id", fmtUUID(job.ProjectID)),
			slog.String("worker_id", workerID))

		// Snapshot cleanup is handled by the orphaned snapshot pass below,
		// which catches both reaped-job snapshots and other stragglers.

		r.publisher.PublishJobFailed(ctx,
			fmtUUID(job.ProjectID),
			fmtUUID(job.ID),
			job.JobType,
			"Worker process became unreachable during indexing.",
		)
	}

	orphanedSnaps, err := r.queries.ListOrphanedBuildingSnapshots(ctx)
	if err != nil {
		slog.WarnContext(ctx, "reaper: list orphaned snapshots failed", slog.Any("error", err))
	} else {
		for _, snap := range orphanedSnaps {
			if err := r.queries.DeleteOrphanedBuildingSnapshot(ctx, snap.ID); err != nil {
				slog.WarnContext(ctx, "reaper: orphaned snapshot cleanup failed",
					slog.String("snapshot_id", fmtUUID(snap.ID)),
					slog.Any("error", err))
			} else {
				slog.InfoContext(ctx, "reaper: deleted orphaned building snapshot",
					slog.String("snapshot_id", fmtUUID(snap.ID)),
					slog.String("project_id", fmtUUID(snap.ProjectID)))
			}
		}
	}

	return reaped, nil
}

// IsWorkerAlive checks whether the worker assigned to a job is still alive
// and working on that specific job. Exported for use by the API lazy reaper.
func (r *Reaper) IsWorkerAlive(ctx context.Context, workerID string, jobID pgtype.UUID) bool {
	return r.isWorkerAlive(ctx, workerID, jobID)
}

// isWorkerAlive checks the Redis heartbeat for the given worker.
func (r *Reaper) isWorkerAlive(ctx context.Context, workerID string, jobID pgtype.UUID) bool {
	if workerID == "" {
		return r.scanForJobOwner(ctx, jobID)
	}

	raw, err := r.rdb.Get(ctx, "worker:status:"+workerID).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false // key expired → worker dead
		}
		// Redis connectivity error — assume alive to avoid false positives.
		slog.WarnContext(ctx, "reaper: Redis error checking worker liveness",
			slog.String("worker_id", workerID),
			slog.Any("error", err))
		return true
	}

	var ws workerStatus
	if err := json.Unmarshal([]byte(raw), &ws); err != nil {
		return false
	}
	return ws.isActiveJob(fmtUUID(jobID))
}

// scanForJobOwner scans all worker:status:* keys for one reporting jobID.
// Used only for legacy jobs without worker_id.
func (r *Reaper) scanForJobOwner(ctx context.Context, jobID pgtype.UUID) bool {
	jobStr := fmtUUID(jobID)
	var cursor uint64
	const maxScanIterations = 1000
	for i := 0; i < maxScanIterations; i++ {
		if err := ctx.Err(); err != nil {
			return true // context cancelled — assume alive
		}
		keys, next, err := r.rdb.Scan(ctx, cursor, "worker:status:*", 100).Result()
		if err != nil {
			slog.WarnContext(ctx, "reaper: Redis SCAN error checking job owner",
				slog.String("job_id", jobStr),
				slog.Any("error", err))
			return true // assume alive to avoid false positives
		}
		for _, key := range keys {
			raw, err := r.rdb.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			var ws workerStatus
			if err := json.Unmarshal([]byte(raw), &ws); err != nil {
				continue
			}
			if ws.isActiveJob(jobStr) {
				return true
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return false
}

// fmtUUID formats a pgtype.UUID as a standard UUID string.
func fmtUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
