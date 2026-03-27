package jobhealth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"myjungle/backend-api/internal/redisclient"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// mockQuerier implements the subset of db.Querier used by Checker.
type mockQuerier struct {
	db.Querier // embed to satisfy interface; only override what we need

	failStaleJobFn               func(ctx context.Context, arg db.FailStaleJobParams) (int64, error)
	deleteOrphanedSnapshotFn     func(ctx context.Context, id pgtype.UUID) error
}

func (m *mockQuerier) FailStaleJob(ctx context.Context, arg db.FailStaleJobParams) (int64, error) {
	if m.failStaleJobFn != nil {
		return m.failStaleJobFn(ctx, arg)
	}
	return 1, nil
}

func (m *mockQuerier) DeleteOrphanedBuildingSnapshot(ctx context.Context, id pgtype.UUID) error {
	if m.deleteOrphanedSnapshotFn != nil {
		return m.deleteOrphanedSnapshotFn(ctx, id)
	}
	return nil
}

const (
	testJobID     = "00000000-0000-0000-0000-000000000001"
	testWorkerID  = "worker-abc"
	testProjectID = "00000000-0000-0000-0000-000000000002"
	testSnapID    = "00000000-0000-0000-0000-000000000003"
)

func newTestChecker(t *testing.T, mr *miniredis.Miniredis, mq *mockQuerier) *Checker {
	t.Helper()
	r, err := redisclient.NewReader("redis://"+mr.Addr(), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return &Checker{
		redis:          r,
		queries:        mq,
		evtPublisher:   nil, // nil-safe
		staleThreshold: 5 * time.Minute,
	}
}

func setWorkerStatus(t *testing.T, mr *miniredis.Miniredis, workerID, jobID string) {
	t.Helper()
	ws := workerStatus{CurrentJobID: jobID}
	if jobID != "" {
		ws.ActiveJobs = map[string]string{jobID: "proj-placeholder"}
	}
	b, _ := json.Marshal(ws)
	mr.Set("worker:status:"+workerID, string(b))
}

func TestNilChecker(t *testing.T) {
	var c *Checker
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q", got, "running")
	}
}

func TestNonRunningStatus(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestChecker(t, mr, &mockQuerier{})
	for _, s := range []string{"queued", "completed", "failed"} {
		got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, s, time.Time{}, testProjectID, "")
		if got != s {
			t.Errorf("status %q: got %q, want %q", s, got, s)
		}
	}
}

func TestEmptyWorkerID_FreshJob(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestChecker(t, mr, &mockQuerier{})
	// Fresh job with no worker ID → skip (not stale yet).
	started := time.Now().Add(-1 * time.Minute)
	got := c.CheckAndReapIfDead(context.Background(), testJobID, "", "running", started, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q", got, "running")
	}
}

func TestEmptyWorkerID_StaleJob(t *testing.T) {
	mr := miniredis.RunT(t)
	mq := &mockQuerier{}
	c := newTestChecker(t, mr, mq)
	// Stale legacy job with no worker ID → reap.
	got := c.CheckAndReapIfDead(context.Background(), testJobID, "", "running", time.Time{}, testProjectID, "")
	if got != "failed" {
		t.Errorf("got %q, want %q", got, "failed")
	}
}

func TestFreshJob(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestChecker(t, mr, &mockQuerier{})
	// Job started 1 minute ago, threshold is 5 minutes → skip.
	started := time.Now().Add(-1 * time.Minute)
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", started, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q", got, "running")
	}
}

func TestWorkerAlive(t *testing.T) {
	mr := miniredis.RunT(t)
	setWorkerStatus(t, mr, testWorkerID, testJobID)
	c := newTestChecker(t, mr, &mockQuerier{})
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q", got, "running")
	}
}

func TestWorkerDead_KeyExpired(t *testing.T) {
	mr := miniredis.RunT(t)
	// No key set → worker is dead.
	mq := &mockQuerier{}
	c := newTestChecker(t, mr, mq)
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "failed" {
		t.Errorf("got %q, want %q", got, "failed")
	}
}

func TestWorkerMovedOn(t *testing.T) {
	mr := miniredis.RunT(t)
	// Worker is alive but working on a different job.
	setWorkerStatus(t, mr, testWorkerID, "00000000-0000-0000-0000-000000000099")
	mq := &mockQuerier{}
	c := newTestChecker(t, mr, mq)
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "failed" {
		t.Errorf("got %q, want %q", got, "failed")
	}
}

func TestWorkerDead_WithSnapshot(t *testing.T) {
	mr := miniredis.RunT(t)
	snapshotDeleted := false
	mq := &mockQuerier{
		deleteOrphanedSnapshotFn: func(_ context.Context, _ pgtype.UUID) error {
			snapshotDeleted = true
			return nil
		},
	}
	c := newTestChecker(t, mr, mq)
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, testSnapID)
	if got != "failed" {
		t.Errorf("got %q, want %q", got, "failed")
	}
	if !snapshotDeleted {
		t.Error("expected orphaned snapshot to be deleted")
	}
}

func TestDBErrorOnFailStaleJob(t *testing.T) {
	mr := miniredis.RunT(t)
	mq := &mockQuerier{
		failStaleJobFn: func(_ context.Context, _ db.FailStaleJobParams) (int64, error) {
			return 0, context.DeadlineExceeded
		},
	}
	c := newTestChecker(t, mr, mq)
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q (graceful degradation on DB error)", got, "running")
	}
}

func TestMalformedHeartbeat(t *testing.T) {
	mr := miniredis.RunT(t)
	// Write corrupted JSON payload.
	mr.Set("worker:status:"+testWorkerID, "not-valid-json{{{")
	c := newTestChecker(t, mr, &mockQuerier{})
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q (assume alive on malformed payload)", got, "running")
	}
}

func TestRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestChecker(t, mr, &mockQuerier{})
	mr.Close() // Force Redis error.
	got := c.CheckAndReapIfDead(context.Background(), testJobID, testWorkerID, "running", time.Time{}, testProjectID, "")
	if got != "running" {
		t.Errorf("got %q, want %q (graceful degradation on Redis error)", got, "running")
	}
}
