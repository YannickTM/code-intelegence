package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	db "myjungle/datastore/postgres/sqlc"
)

// testUUID returns a pgtype.UUID from a [16]byte literal.
func testUUID(b [16]byte) pgtype.UUID {
	return pgtype.UUID{Bytes: b, Valid: true}
}

var (
	jobUUID1  = testUUID([16]byte{0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	jobUUID2  = testUUID([16]byte{0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2})
	jobUUID3  = testUUID([16]byte{0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3})
	projUUID  = testUUID([16]byte{0, 0, 0, 0xA, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xA})
	snapUUID  = testUUID([16]byte{0, 0, 0, 0xB, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xB})
)

// mockQuerier embeds db.Querier and overrides only the methods the reaper uses.
type mockQuerier struct {
	db.Querier

	listStaleRunningJobsFn          func(ctx context.Context, threshold pgtype.Interval) ([]db.ListStaleRunningJobsRow, error)
	failStaleJobFn                  func(ctx context.Context, arg db.FailStaleJobParams) (int64, error)
	listOrphanedBuildingSnapshotsFn func(ctx context.Context) ([]db.ListOrphanedBuildingSnapshotsRow, error)
	deleteOrphanedBuildingSnapshotFn func(ctx context.Context, id pgtype.UUID) error
}

func (m *mockQuerier) ListStaleRunningJobs(ctx context.Context, threshold pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
	if m.listStaleRunningJobsFn != nil {
		return m.listStaleRunningJobsFn(ctx, threshold)
	}
	return nil, nil
}

func (m *mockQuerier) FailStaleJob(ctx context.Context, arg db.FailStaleJobParams) (int64, error) {
	if m.failStaleJobFn != nil {
		return m.failStaleJobFn(ctx, arg)
	}
	return 1, nil
}

func (m *mockQuerier) ListOrphanedBuildingSnapshots(ctx context.Context) ([]db.ListOrphanedBuildingSnapshotsRow, error) {
	if m.listOrphanedBuildingSnapshotsFn != nil {
		return m.listOrphanedBuildingSnapshotsFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) DeleteOrphanedBuildingSnapshot(ctx context.Context, id pgtype.UUID) error {
	if m.deleteOrphanedBuildingSnapshotFn != nil {
		return m.deleteOrphanedBuildingSnapshotFn(ctx, id)
	}
	return nil
}

// setup creates a miniredis server and a connected redis.Client.
func setup(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// setHeartbeat writes a worker heartbeat key in miniredis.
func setHeartbeat(t *testing.T, mr *miniredis.Miniredis, workerID, currentJobID string) {
	t.Helper()
	ws := workerStatus{CurrentJobID: currentJobID}
	if currentJobID != "" {
		ws.ActiveJobs = map[string]string{currentJobID: "proj-placeholder"}
	}
	data, _ := json.Marshal(ws)
	mr.Set("worker:status:"+workerID, string(data))
	mr.SetTTL("worker:status:"+workerID, 30*time.Second)
}

func staleJob(workerID string, snapValid bool) db.ListStaleRunningJobsRow {
	return staleJobWithID(jobUUID1, workerID, snapValid)
}

func staleJobWithID(id pgtype.UUID, workerID string, snapValid bool) db.ListStaleRunningJobsRow {
	row := db.ListStaleRunningJobsRow{
		ID:        id,
		ProjectID: projUUID,
		WorkerID:  pgtype.Text{String: workerID, Valid: workerID != ""},
		StartedAt: pgtype.Timestamptz{Time: time.Now().Add(-10 * time.Minute), Valid: true},
		JobType:   "full-index",
	}
	if snapValid {
		row.IndexSnapshotID = snapUUID
	}
	return row
}

func TestRunOnce_WorkerDead(t *testing.T) {
	mr, rdb := setup(t)
	_ = mr // no heartbeat set → worker dead

	var failedID pgtype.UUID
	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("worker-1", true)}, nil
		},
		failStaleJobFn: func(_ context.Context, arg db.FailStaleJobParams) (int64, error) {
			failedID = arg.ID
			return 1, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 1 {
		t.Errorf("reaped = %d, want 1", n)
	}
	if failedID != jobUUID1 {
		t.Errorf("failed job ID = %v, want %v", failedID, jobUUID1)
	}
}

func TestRunOnce_WorkerAlive(t *testing.T) {
	mr, rdb := setup(t)
	setHeartbeat(t, mr, "worker-1", fmtUUID(jobUUID1))

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("worker-1", true)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			t.Error("FailStaleJob should not be called when worker is alive")
			return 0, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 0 {
		t.Errorf("reaped = %d, want 0", n)
	}
}

func TestRunOnce_WorkerMovedOn(t *testing.T) {
	mr, rdb := setup(t)
	// Worker is alive but reporting a different job.
	setHeartbeat(t, mr, "worker-1", fmtUUID(jobUUID2))

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("worker-1", false)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			return 1, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 1 {
		t.Errorf("reaped = %d, want 1", n)
	}
}

func TestRunOnce_LegacyJobNoWorkerID(t *testing.T) {
	mr, rdb := setup(t)
	_ = mr // no workers in Redis → orphaned

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("", false)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			return 1, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 1 {
		t.Errorf("reaped = %d, want 1", n)
	}
}

func TestRunOnce_LegacyJobWorkerFound(t *testing.T) {
	mr, rdb := setup(t)
	// Some worker reports this job via SCAN.
	setHeartbeat(t, mr, "worker-x", fmtUUID(jobUUID1))

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("", false)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			t.Error("FailStaleJob should not be called when worker found via SCAN")
			return 0, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 0 {
		t.Errorf("reaped = %d, want 0", n)
	}
}

func TestRunOnce_OrphanedSnapshotCleanup(t *testing.T) {
	_, rdb := setup(t)

	var deletedIDs []pgtype.UUID
	mq := &mockQuerier{
		listOrphanedBuildingSnapshotsFn: func(_ context.Context) ([]db.ListOrphanedBuildingSnapshotsRow, error) {
			return []db.ListOrphanedBuildingSnapshotsRow{
				{ID: snapUUID, ProjectID: projUUID},
			}, nil
		},
		deleteOrphanedBuildingSnapshotFn: func(_ context.Context, id pgtype.UUID) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	_, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if len(deletedIDs) != 1 || deletedIDs[0] != snapUUID {
		t.Errorf("deleted snapshots = %v, want [%v]", deletedIDs, snapUUID)
	}
}

func TestRunOnce_Idempotent(t *testing.T) {
	mr, rdb := setup(t)
	_ = mr

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("worker-1", true)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			return 0, nil // already reaped
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 0 {
		t.Errorf("reaped = %d, want 0 (idempotent)", n)
	}
}

func TestRunOnce_FailStaleJobError(t *testing.T) {
	mr, rdb := setup(t)
	_ = mr

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("worker-1", false)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			return 0, fmt.Errorf("db error")
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce should not return error for individual job failures, got: %v", err)
	}
	if n != 0 {
		t.Errorf("reaped = %d, want 0", n)
	}
}

func TestIsWorkerAlive_Alive(t *testing.T) {
	mr, rdb := setup(t)
	setHeartbeat(t, mr, "w1", fmtUUID(jobUUID1))

	r := New(Config{}, nil, rdb, nil)
	if !r.IsWorkerAlive(context.Background(), "w1", jobUUID1) {
		t.Error("IsWorkerAlive = false, want true")
	}
}

func TestIsWorkerAlive_Dead(t *testing.T) {
	_, rdb := setup(t)

	r := New(Config{}, nil, rdb, nil)
	if r.IsWorkerAlive(context.Background(), "w1", jobUUID1) {
		t.Error("IsWorkerAlive = true, want false")
	}
}

func TestIsWorkerAlive_RedisDown(t *testing.T) {
	mr, rdb := setup(t)
	_ = mr
	// Close miniredis to simulate connectivity failure.
	mr.Close()

	r := New(Config{}, nil, rdb, nil)
	// Should assume alive (true) on Redis error, not false.
	if !r.IsWorkerAlive(context.Background(), "w1", jobUUID1) {
		t.Error("IsWorkerAlive = false on Redis error, want true (assume alive)")
	}
}

func TestRunOnce_RedisDownSkipsReaping(t *testing.T) {
	mr, rdb := setup(t)
	mr.Close() // simulate Redis outage

	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{staleJob("worker-1", false)}, nil
		},
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			t.Error("FailStaleJob should not be called when Redis is down")
			return 0, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 0 {
		t.Errorf("reaped = %d, want 0 (Redis down, assume workers alive)", n)
	}
}

func TestIsWorkerAlive_DifferentJob(t *testing.T) {
	mr, rdb := setup(t)
	setHeartbeat(t, mr, "w1", fmtUUID(jobUUID2))

	r := New(Config{}, nil, rdb, nil)
	if r.IsWorkerAlive(context.Background(), "w1", jobUUID1) {
		t.Error("IsWorkerAlive = true, want false (different job)")
	}
}

func TestRunOnce_MultipleMixedJobs(t *testing.T) {
	mr, rdb := setup(t)
	// worker-1 alive, reporting job1 → skip job1
	setHeartbeat(t, mr, "worker-1", fmtUUID(jobUUID1))
	// worker-2 dead (no heartbeat) → reap job2
	// worker-3 alive but reporting job1, not job3 → reap job3
	setHeartbeat(t, mr, "worker-3", fmtUUID(jobUUID1))

	var failedIDs []pgtype.UUID
	mq := &mockQuerier{
		listStaleRunningJobsFn: func(_ context.Context, _ pgtype.Interval) ([]db.ListStaleRunningJobsRow, error) {
			return []db.ListStaleRunningJobsRow{
				staleJobWithID(jobUUID1, "worker-1", false), // alive
				staleJobWithID(jobUUID2, "worker-2", false), // dead
				staleJobWithID(jobUUID3, "worker-3", false), // moved on
			}, nil
		},
		failStaleJobFn: func(_ context.Context, arg db.FailStaleJobParams) (int64, error) {
			failedIDs = append(failedIDs, arg.ID)
			return 1, nil
		},
	}

	r := New(Config{StaleThreshold: 5 * time.Minute}, mq, rdb, nil)
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if n != 2 {
		t.Errorf("reaped = %d, want 2", n)
	}
	if len(failedIDs) != 2 {
		t.Fatalf("failed IDs count = %d, want 2", len(failedIDs))
	}
	if failedIDs[0] != jobUUID2 {
		t.Errorf("failedIDs[0] = %v, want %v", failedIDs[0], jobUUID2)
	}
	if failedIDs[1] != jobUUID3 {
		t.Errorf("failedIDs[1] = %v, want %v", failedIDs[1], jobUUID3)
	}
}

func TestFmtUUID(t *testing.T) {
	got := fmtUUID(jobUUID1)
	want := "00000001-0000-0000-0000-000000000001"
	if got != want {
		t.Errorf("fmtUUID = %q, want %q", got, want)
	}

	got = fmtUUID(pgtype.UUID{Valid: false})
	if got != "" {
		t.Errorf("fmtUUID(invalid) = %q, want empty", got)
	}
}
