package repository

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/sshkey"
	db "myjungle/datastore/postgres/sqlc"
)

// syntheticTestSecret is a clearly-fake value used only in unit tests.
// It is NOT a real secret and will never appear in production.
const syntheticTestSecret = "TESTONLY-not-a-real-secret-0123456789"

// --- helpers ---

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

func testEncrypt(t *testing.T, plaintext []byte) []byte {
	t.Helper()
	key := sshkey.DeriveKey(syntheticTestSecret)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil)
}

// --- mock ---

// mockQuerier implements db.Querier for testing.
// Unset function fields will panic on call (intentional test guard).
type mockQuerier struct {
	db.Querier // embed nil interface — unimplemented methods panic

	getIndexingJobFn                 func(ctx context.Context, id pgtype.UUID) (db.IndexingJob, error)
	getProjectSSHKeyForExecutionFn   func(ctx context.Context, id pgtype.UUID) (db.GetProjectSSHKeyForExecutionRow, error)
	getEmbeddingProviderConfigByIDFn func(ctx context.Context, id pgtype.UUID) (db.EmbeddingProviderConfig, error)
	getIndexSnapshotFn               func(ctx context.Context, id pgtype.UUID) (db.IndexSnapshot, error)
	claimQueuedIndexingJobFn         func(ctx context.Context, arg db.ClaimQueuedIndexingJobParams) (int64, error)
	setIndexingJobRunningFn          func(ctx context.Context, id pgtype.UUID) error
	setIndexingJobProgressFn         func(ctx context.Context, arg db.SetIndexingJobProgressParams) error
	setIndexingJobCompletedFn        func(ctx context.Context, id pgtype.UUID) error
	setIndexingJobFailedFn           func(ctx context.Context, arg db.SetIndexingJobFailedParams) error
	createIndexSnapshotFn            func(ctx context.Context, arg db.CreateIndexSnapshotParams) (db.IndexSnapshot, error)
	deactivateActiveSnapshotFn       func(ctx context.Context, arg db.DeactivateActiveSnapshotParams) error
	activateSnapshotFn               func(ctx context.Context, id pgtype.UUID) error

	// Guarded terminal transitions (Task 10).
	setIndexingJobCompletedFromRunningFn func(ctx context.Context, id pgtype.UUID) (int64, error)
	setIndexingJobFailedFromRunningFn    func(ctx context.Context, arg db.SetIndexingJobFailedFromRunningParams) (int64, error)
	tryAdvisoryLockForProjectFn          func(ctx context.Context, arg db.TryAdvisoryLockForProjectParams) (bool, error)
	releaseAdvisoryLockForProjectFn      func(ctx context.Context, arg db.ReleaseAdvisoryLockForProjectParams) (bool, error)
}

func (m *mockQuerier) GetIndexingJob(ctx context.Context, id pgtype.UUID) (db.IndexingJob, error) {
	return m.getIndexingJobFn(ctx, id)
}

func (m *mockQuerier) GetProjectSSHKeyForExecution(ctx context.Context, id pgtype.UUID) (db.GetProjectSSHKeyForExecutionRow, error) {
	return m.getProjectSSHKeyForExecutionFn(ctx, id)
}

func (m *mockQuerier) GetEmbeddingProviderConfigByID(ctx context.Context, id pgtype.UUID) (db.EmbeddingProviderConfig, error) {
	return m.getEmbeddingProviderConfigByIDFn(ctx, id)
}

func (m *mockQuerier) GetIndexSnapshot(ctx context.Context, id pgtype.UUID) (db.IndexSnapshot, error) {
	return m.getIndexSnapshotFn(ctx, id)
}

func (m *mockQuerier) ClaimQueuedIndexingJob(ctx context.Context, arg db.ClaimQueuedIndexingJobParams) (int64, error) {
	return m.claimQueuedIndexingJobFn(ctx, arg)
}

func (m *mockQuerier) SetIndexingJobRunning(ctx context.Context, id pgtype.UUID) error {
	return m.setIndexingJobRunningFn(ctx, id)
}

func (m *mockQuerier) SetIndexingJobProgress(ctx context.Context, arg db.SetIndexingJobProgressParams) error {
	return m.setIndexingJobProgressFn(ctx, arg)
}

func (m *mockQuerier) SetIndexingJobCompleted(ctx context.Context, id pgtype.UUID) error {
	return m.setIndexingJobCompletedFn(ctx, id)
}

func (m *mockQuerier) SetIndexingJobFailed(ctx context.Context, arg db.SetIndexingJobFailedParams) error {
	return m.setIndexingJobFailedFn(ctx, arg)
}

func (m *mockQuerier) CreateIndexSnapshot(ctx context.Context, arg db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
	return m.createIndexSnapshotFn(ctx, arg)
}

func (m *mockQuerier) DeactivateActiveSnapshot(ctx context.Context, arg db.DeactivateActiveSnapshotParams) error {
	return m.deactivateActiveSnapshotFn(ctx, arg)
}
func (m *mockQuerier) ActivateSnapshot(ctx context.Context, id pgtype.UUID) error {
	return m.activateSnapshotFn(ctx, id)
}

func (m *mockQuerier) SetIndexingJobCompletedFromRunning(ctx context.Context, id pgtype.UUID) (int64, error) {
	return m.setIndexingJobCompletedFromRunningFn(ctx, id)
}

func (m *mockQuerier) SetIndexingJobFailedFromRunning(ctx context.Context, arg db.SetIndexingJobFailedFromRunningParams) (int64, error) {
	return m.setIndexingJobFailedFromRunningFn(ctx, arg)
}

func (m *mockQuerier) TryAdvisoryLockForProject(ctx context.Context, arg db.TryAdvisoryLockForProjectParams) (bool, error) {
	return m.tryAdvisoryLockForProjectFn(ctx, arg)
}

func (m *mockQuerier) ReleaseAdvisoryLockForProject(ctx context.Context, arg db.ReleaseAdvisoryLockForProjectParams) (bool, error) {
	return m.releaseAdvisoryLockForProjectFn(ctx, arg)
}

// --- fixtures ---

var (
	jobID      = testUUID(0x01)
	projectID  = testUUID(0x02)
	sshKeyID   = testUUID(0x03)
	embCfgID   = testUUID(0x04)
	snapshotID = testUUID(0x05)

	errNotFound = errors.New("no rows in result set")
)

func validJob() db.IndexingJob {
	return db.IndexingJob{
		ID:                        jobID,
		ProjectID:                 projectID,
		JobType:                   "full-index",
		Status:                    "queued",
		EmbeddingProviderConfigID: embCfgID,
	}
}

func validSSHRow(t *testing.T) db.GetProjectSSHKeyForExecutionRow {
	return db.GetProjectSSHKeyForExecutionRow{
		ID:                  sshKeyID,
		Name:                "deploy-key",
		PublicKey:           "ssh-ed25519 AAAA...",
		PrivateKeyEncrypted: testEncrypt(t, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----\n")),
		KeyType:             "ed25519",
		Fingerprint:         "SHA256:abc123",
		RepoUrl:             "git@github.com:org/repo.git",
		DefaultBranch:       "main",
	}
}

func validEmbeddingConfig() db.EmbeddingProviderConfig {
	return db.EmbeddingProviderConfig{
		ID:          embCfgID,
		Provider:    "ollama",
		EndpointUrl: "http://localhost:11434",
		Model:       "jina/jina-embeddings-v2-base-en",
		Dimensions:  768,
	}
}

func newMockForSuccess(t *testing.T) *mockQuerier {
	return &mockQuerier{
		getIndexingJobFn: func(_ context.Context, _ pgtype.UUID) (db.IndexingJob, error) {
			return validJob(), nil
		},
		getProjectSSHKeyForExecutionFn: func(_ context.Context, _ pgtype.UUID) (db.GetProjectSSHKeyForExecutionRow, error) {
			return validSSHRow(t), nil
		},
		getEmbeddingProviderConfigByIDFn: func(_ context.Context, _ pgtype.UUID) (db.EmbeddingProviderConfig, error) {
			return validEmbeddingConfig(), nil
		},
		getIndexSnapshotFn: func(_ context.Context, _ pgtype.UUID) (db.IndexSnapshot, error) {
			return db.IndexSnapshot{}, errors.New("should not be called")
		},
	}
}

// --- New() tests ---

// noopPinned is a PinnedQueryFunc for tests that don't exercise advisory locks.
var noopPinned = PinnedQueryFunc(func(_ context.Context) (*PinnedConn, error) {
	return &PinnedConn{Querier: &mockQuerier{}, Release: func() {}, Destroy: func() {}}, nil
})

func TestNew_EmptySecret(t *testing.T) {
	_, err := New(&mockQuerier{}, "", noopPinned)
	if err == nil {
		t.Fatal("expected error for empty encryption secret")
	}
	if !strings.Contains(err.Error(), "encryption secret") {
		t.Errorf("error = %q, want to contain %q", err, "encryption secret")
	}
}

func TestNew_NilAcquirePinned(t *testing.T) {
	_, err := New(&mockQuerier{}, syntheticTestSecret, nil)
	if err == nil {
		t.Fatal("expected error for nil acquirePinned")
	}
	if !strings.Contains(err.Error(), "acquirePinned") {
		t.Errorf("error = %q, want to contain %q", err, "acquirePinned")
	}
}

func TestNew_ValidSecret(t *testing.T) {
	repo, err := New(&mockQuerier{}, syntheticTestSecret, noopPinned)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

// --- LoadExecutionContext tests ---

func TestLoadExecutionContext_Success(t *testing.T) {
	repo, err := New(newMockForSuccess(t), syntheticTestSecret, noopPinned)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ec, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ec.JobID != jobID {
		t.Errorf("JobID = %v, want %v", ec.JobID, jobID)
	}
	if ec.JobType != "full-index" {
		t.Errorf("JobType = %q, want %q", ec.JobType, "full-index")
	}
	if ec.ProjectID != projectID {
		t.Errorf("ProjectID = %v, want %v", ec.ProjectID, projectID)
	}
	if ec.RepoURL != "git@github.com:org/repo.git" {
		t.Errorf("RepoURL = %q", ec.RepoURL)
	}
	if ec.Branch != "main" {
		t.Errorf("Branch = %q, want %q", ec.Branch, "main")
	}
	if ec.SSHKeyID != sshKeyID {
		t.Errorf("SSHKeyID = %v, want %v", ec.SSHKeyID, sshKeyID)
	}
	if len(ec.SSHPrivateKey) == 0 {
		t.Error("SSHPrivateKey is empty")
	}
	if ec.Embedding.Provider != "ollama" {
		t.Errorf("Embedding.Provider = %q", ec.Embedding.Provider)
	}
	if ec.Embedding.Model != "jina/jina-embeddings-v2-base-en" {
		t.Errorf("Embedding.Model = %q", ec.Embedding.Model)
	}
	if ec.Embedding.Dimensions != 768 {
		t.Errorf("Embedding.Dimensions = %d", ec.Embedding.Dimensions)
	}
}

func TestLoadExecutionContext_UsesSnapshotBranch(t *testing.T) {
	m := newMockForSuccess(t)
	m.getIndexingJobFn = func(_ context.Context, _ pgtype.UUID) (db.IndexingJob, error) {
		j := validJob()
		j.IndexSnapshotID = snapshotID // valid snapshot reference
		return j, nil
	}
	m.getIndexSnapshotFn = func(_ context.Context, _ pgtype.UUID) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{
			ID:     snapshotID,
			Branch: "feature/custom",
		}, nil
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	ec, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ec.Branch != "feature/custom" {
		t.Errorf("Branch = %q, want %q", ec.Branch, "feature/custom")
	}
}

func TestLoadExecutionContext_FallsBackToDefaultBranch(t *testing.T) {
	// Job has no snapshot — should use project default branch
	repo, _ := New(newMockForSuccess(t), syntheticTestSecret, noopPinned)

	ec, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ec.Branch != "main" {
		t.Errorf("Branch = %q, want %q (project default)", ec.Branch, "main")
	}
}

func TestLoadExecutionContext_JobNotFound(t *testing.T) {
	m := newMockForSuccess(t)
	m.getIndexingJobFn = func(_ context.Context, _ pgtype.UUID) (db.IndexingJob, error) {
		return db.IndexingJob{}, errNotFound
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "load job") {
		t.Errorf("error = %q, want to contain %q", err, "load job")
	}
}

func TestLoadExecutionContext_NoSSHKeyAssignment(t *testing.T) {
	m := newMockForSuccess(t)
	m.getProjectSSHKeyForExecutionFn = func(_ context.Context, _ pgtype.UUID) (db.GetProjectSSHKeyForExecutionRow, error) {
		return db.GetProjectSSHKeyForExecutionRow{}, errNotFound
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "load project SSH key") {
		t.Errorf("error = %q, want to contain %q", err, "load project SSH key")
	}
}

func TestLoadExecutionContext_DecryptionFailure(t *testing.T) {
	m := newMockForSuccess(t)
	m.getProjectSSHKeyForExecutionFn = func(_ context.Context, _ pgtype.UUID) (db.GetProjectSSHKeyForExecutionRow, error) {
		row := validSSHRow(t)
		row.PrivateKeyEncrypted = []byte("not-valid-ciphertext-at-all-need-at-least-12-bytes")
		return row, nil
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "decrypt SSH key") {
		t.Errorf("error = %q, want to contain %q", err, "decrypt SSH key")
	}
}

func TestLoadExecutionContext_WrongEncryptionSecret(t *testing.T) {
	m := newMockForSuccess(t) // SSH key encrypted with syntheticTestSecret
	repo, _ := New(m, "TESTONLY-different-wrong-secret-999999", noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "decrypt SSH key") {
		t.Errorf("error = %q, want to contain %q", err, "decrypt SSH key")
	}
}

func TestLoadExecutionContext_EmbeddingConfigIDNotSet(t *testing.T) {
	m := newMockForSuccess(t)
	m.getIndexingJobFn = func(_ context.Context, _ pgtype.UUID) (db.IndexingJob, error) {
		j := validJob()
		j.EmbeddingProviderConfigID = pgtype.UUID{} // Valid = false
		return j, nil
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no embedding provider config ID") {
		t.Errorf("error = %q, want to contain %q", err, "no embedding provider config ID")
	}
}

func TestLoadExecutionContext_EmbeddingConfigNotFound(t *testing.T) {
	m := newMockForSuccess(t)
	m.getEmbeddingProviderConfigByIDFn = func(_ context.Context, _ pgtype.UUID) (db.EmbeddingProviderConfig, error) {
		return db.EmbeddingProviderConfig{}, errNotFound
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "load embedding config") {
		t.Errorf("error = %q, want to contain %q", err, "load embedding config")
	}
}

func TestLoadExecutionContext_SnapshotNotFound(t *testing.T) {
	m := newMockForSuccess(t)
	m.getIndexingJobFn = func(_ context.Context, _ pgtype.UUID) (db.IndexingJob, error) {
		j := validJob()
		j.IndexSnapshotID = snapshotID
		return j, nil
	}
	m.getIndexSnapshotFn = func(_ context.Context, _ pgtype.UUID) (db.IndexSnapshot, error) {
		return db.IndexSnapshot{}, errNotFound
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	_, err := repo.LoadExecutionContext(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "load index snapshot") {
		t.Errorf("error = %q, want to contain %q", err, "load index snapshot")
	}
}

// --- ClaimJob tests ---

func TestClaimJob_Success(t *testing.T) {
	m := &mockQuerier{
		claimQueuedIndexingJobFn: func(_ context.Context, arg db.ClaimQueuedIndexingJobParams) (int64, error) {
			if arg.ID != jobID {
				t.Errorf("called with %v, want %v", arg.ID, jobID)
			}
			if arg.WorkerID.String != "test-worker" {
				t.Errorf("worker_id = %q, want %q", arg.WorkerID.String, "test-worker")
			}
			return 1, nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	if err := repo.ClaimJob(context.Background(), jobID, "test-worker"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClaimJob_NotQueued(t *testing.T) {
	m := &mockQuerier{
		claimQueuedIndexingJobFn: func(_ context.Context, _ db.ClaimQueuedIndexingJobParams) (int64, error) {
			return 0, nil // no rows affected — job was not in queued status
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	err := repo.ClaimJob(context.Background(), jobID, "test-worker")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not in queued status") {
		t.Errorf("error = %q, want to contain %q", err, "not in queued status")
	}
}

func TestClaimJob_DBError(t *testing.T) {
	m := &mockQuerier{
		claimQueuedIndexingJobFn: func(_ context.Context, _ db.ClaimQueuedIndexingJobParams) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	err := repo.ClaimJob(context.Background(), jobID, "test-worker")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claim job") {
		t.Errorf("error = %q, want to contain %q", err, "claim job")
	}
}

// --- Delegation tests ---

func TestSetJobRunning(t *testing.T) {
	var called pgtype.UUID
	m := &mockQuerier{
		setIndexingJobRunningFn: func(_ context.Context, id pgtype.UUID) error {
			called = id
			return nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	if err := repo.SetJobRunning(context.Background(), jobID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != jobID {
		t.Errorf("called with %v, want %v", called, jobID)
	}
}

func TestSetJobProgress(t *testing.T) {
	var got db.SetIndexingJobProgressParams
	m := &mockQuerier{
		setIndexingJobProgressFn: func(_ context.Context, arg db.SetIndexingJobProgressParams) error {
			got = arg
			return nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	if err := repo.SetJobProgress(context.Background(), jobID, 10, 20, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != jobID || got.FilesProcessed != 10 || got.ChunksUpserted != 20 || got.VectorsDeleted != 5 {
		t.Errorf("params = %+v", got)
	}
}

func TestSetJobCompleted_Transitioned(t *testing.T) {
	var called pgtype.UUID
	m := &mockQuerier{
		setIndexingJobCompletedFromRunningFn: func(_ context.Context, id pgtype.UUID) (int64, error) {
			called = id
			return 1, nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	ok, err := repo.SetJobCompleted(context.Background(), jobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected transitioned=true")
	}
	if called != jobID {
		t.Errorf("called with %v, want %v", called, jobID)
	}
}

func TestSetJobCompleted_Stale(t *testing.T) {
	m := &mockQuerier{
		setIndexingJobCompletedFromRunningFn: func(_ context.Context, _ pgtype.UUID) (int64, error) {
			return 0, nil // job not in running status
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	ok, err := repo.SetJobCompleted(context.Background(), jobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected transitioned=false for stale job")
	}
}

func TestSetJobCompleted_DBError(t *testing.T) {
	m := &mockQuerier{
		setIndexingJobCompletedFromRunningFn: func(_ context.Context, _ pgtype.UUID) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	ok, err := repo.SetJobCompleted(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if ok {
		t.Error("expected transitioned=false on error")
	}
	if !strings.Contains(err.Error(), "set job completed") {
		t.Errorf("error = %q, want to contain %q", err, "set job completed")
	}
}

func TestSetJobFailed_Transitioned(t *testing.T) {
	var got db.SetIndexingJobFailedFromRunningParams
	m := &mockQuerier{
		setIndexingJobFailedFromRunningFn: func(_ context.Context, arg db.SetIndexingJobFailedFromRunningParams) (int64, error) {
			got = arg
			return 1, nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	details := []byte(`[{"message":"boom"}]`)
	ok, err := repo.SetJobFailed(context.Background(), jobID, details)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected transitioned=true")
	}
	if got.ID != jobID || string(got.ErrorDetails) != string(details) {
		t.Errorf("params = %+v", got)
	}
}

func TestSetJobFailed_Stale(t *testing.T) {
	m := &mockQuerier{
		setIndexingJobFailedFromRunningFn: func(_ context.Context, _ db.SetIndexingJobFailedFromRunningParams) (int64, error) {
			return 0, nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	ok, err := repo.SetJobFailed(context.Background(), jobID, []byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected transitioned=false for stale job")
	}
}

func TestSetJobFailed_DBError(t *testing.T) {
	m := &mockQuerier{
		setIndexingJobFailedFromRunningFn: func(_ context.Context, _ db.SetIndexingJobFailedFromRunningParams) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	ok, err := repo.SetJobFailed(context.Background(), jobID, []byte(`[]`))
	if err == nil {
		t.Fatal("expected error")
	}
	if ok {
		t.Error("expected transitioned=false on error")
	}
	if !strings.Contains(err.Error(), "set job failed") {
		t.Errorf("error = %q, want to contain %q", err, "set job failed")
	}
}

func TestCreateSnapshot(t *testing.T) {
	want := db.IndexSnapshot{ID: snapshotID}
	m := &mockQuerier{
		createIndexSnapshotFn: func(_ context.Context, _ db.CreateIndexSnapshotParams) (db.IndexSnapshot, error) {
			return want, nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	got, err := repo.CreateSnapshot(context.Background(), db.CreateIndexSnapshotParams{
		ProjectID: projectID,
		Branch:    "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("snapshot ID = %v, want %v", got.ID, want.ID)
	}
}

// --- ActivateSnapshot tests ---

func TestActivateSnapshot_Success(t *testing.T) {
	var gotDeactivate db.DeactivateActiveSnapshotParams
	var gotActivateID pgtype.UUID
	var seq int
	var deactivateOrder, activateOrder int
	m := &mockQuerier{
		deactivateActiveSnapshotFn: func(_ context.Context, arg db.DeactivateActiveSnapshotParams) error {
			seq++
			deactivateOrder = seq
			gotDeactivate = arg
			return nil
		},
		activateSnapshotFn: func(_ context.Context, id pgtype.UUID) error {
			seq++
			activateOrder = seq
			gotActivateID = id
			return nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	err := repo.ActivateSnapshot(context.Background(), projectID, "main", snapshotID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDeactivate.ProjectID != projectID || gotDeactivate.Branch != "main" {
		t.Errorf("deactivate params = %+v", gotDeactivate)
	}
	if gotActivateID != snapshotID {
		t.Errorf("activate ID = %v, want %v", gotActivateID, snapshotID)
	}
	if deactivateOrder >= activateOrder {
		t.Errorf("deactivation (order=%d) must happen before activation (order=%d)", deactivateOrder, activateOrder)
	}
}

func TestActivateSnapshot_Error(t *testing.T) {
	m := &mockQuerier{
		deactivateActiveSnapshotFn: func(_ context.Context, _ db.DeactivateActiveSnapshotParams) error {
			return nil
		},
		activateSnapshotFn: func(_ context.Context, _ pgtype.UUID) error {
			return errors.New("db error")
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	err := repo.ActivateSnapshot(context.Background(), projectID, "main", snapshotID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "activate snapshot") {
		t.Errorf("error = %q, want to contain %q", err, "activate snapshot")
	}
}

func TestSetJobRunning_Error(t *testing.T) {
	m := &mockQuerier{
		setIndexingJobRunningFn: func(_ context.Context, _ pgtype.UUID) error {
			return errors.New("db error")
		},
	}
	repo, _ := New(m, syntheticTestSecret, noopPinned)

	err := repo.SetJobRunning(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "set job running") {
		t.Errorf("error = %q, want to contain %q", err, "set job running")
	}
}

// --- TryProjectLock tests ---

// pinnedFnFor returns a PinnedQueryFunc that returns the given mock querier
// with no-op release/destroy functions.
func pinnedFnFor(m *mockQuerier) PinnedQueryFunc {
	return func(_ context.Context) (*PinnedConn, error) {
		return &PinnedConn{Querier: m, Release: func() {}, Destroy: func() {}}, nil
	}
}

func TestTryProjectLock_Acquired(t *testing.T) {
	var lockCalled bool
	var released bool
	m := &mockQuerier{
		tryAdvisoryLockForProjectFn: func(_ context.Context, _ db.TryAdvisoryLockForProjectParams) (bool, error) {
			lockCalled = true
			return true, nil
		},
		releaseAdvisoryLockForProjectFn: func(_ context.Context, _ db.ReleaseAdvisoryLockForProjectParams) (bool, error) {
			released = true
			return true, nil
		},
	}
	repo, _ := New(m, syntheticTestSecret, pinnedFnFor(m))

	unlock, err := repo.TryProjectLock(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unlock == nil {
		t.Fatal("expected non-nil unlock function")
	}
	if !lockCalled {
		t.Error("TryAdvisoryLockForProject should have been called")
	}

	// Release the lock.
	unlock(context.Background())
	if !released {
		t.Error("ReleaseAdvisoryLockForProject should have been called")
	}
}

func TestTryProjectLock_NotAcquired(t *testing.T) {
	var connReleased bool
	m := &mockQuerier{
		tryAdvisoryLockForProjectFn: func(_ context.Context, _ db.TryAdvisoryLockForProjectParams) (bool, error) {
			return false, nil
		},
	}
	pinned := func(_ context.Context) (*PinnedConn, error) {
		return &PinnedConn{Querier: m, Release: func() { connReleased = true }, Destroy: func() {}}, nil
	}
	repo, _ := New(m, syntheticTestSecret, pinned)

	unlock, err := repo.TryProjectLock(context.Background(), projectID)
	if !errors.Is(err, ErrProjectLocked) {
		t.Fatalf("expected ErrProjectLocked, got: %v", err)
	}
	if unlock != nil {
		t.Error("expected nil unlock function when lock not acquired")
	}
	if !connReleased {
		t.Error("connection should be released when lock not acquired")
	}
}

func TestTryProjectLock_DBError(t *testing.T) {
	var connReleased bool
	m := &mockQuerier{
		tryAdvisoryLockForProjectFn: func(_ context.Context, _ db.TryAdvisoryLockForProjectParams) (bool, error) {
			return false, errors.New("connection refused")
		},
	}
	pinned := func(_ context.Context) (*PinnedConn, error) {
		return &PinnedConn{Querier: m, Release: func() { connReleased = true }, Destroy: func() {}}, nil
	}
	repo, _ := New(m, syntheticTestSecret, pinned)

	unlock, err := repo.TryProjectLock(context.Background(), projectID)
	if err == nil {
		t.Fatal("expected error")
	}
	if unlock != nil {
		t.Error("expected nil unlock function on error")
	}
	if !connReleased {
		t.Error("connection should be released on error")
	}
	if !strings.Contains(err.Error(), "try project lock") {
		t.Errorf("error = %q, want to contain %q", err, "try project lock")
	}
}

func TestTryProjectLock_AcquireConnError(t *testing.T) {
	pinned := func(_ context.Context) (*PinnedConn, error) {
		return nil, errors.New("pool exhausted")
	}
	repo, _ := New(&mockQuerier{}, syntheticTestSecret, pinned)

	unlock, err := repo.TryProjectLock(context.Background(), projectID)
	if err == nil {
		t.Fatal("expected error")
	}
	if unlock != nil {
		t.Error("expected nil unlock function on error")
	}
	if !strings.Contains(err.Error(), "acquire connection") {
		t.Errorf("error = %q, want to contain %q", err, "acquire connection")
	}
}

func TestTryProjectLock_UnlockFailure_DestroysConnection(t *testing.T) {
	var destroyed bool
	var released bool
	m := &mockQuerier{
		tryAdvisoryLockForProjectFn: func(_ context.Context, _ db.TryAdvisoryLockForProjectParams) (bool, error) {
			return true, nil
		},
		releaseAdvisoryLockForProjectFn: func(_ context.Context, _ db.ReleaseAdvisoryLockForProjectParams) (bool, error) {
			return false, errors.New("unlock failed")
		},
	}
	pinned := func(_ context.Context) (*PinnedConn, error) {
		return &PinnedConn{
			Querier: m,
			Release: func() { released = true },
			Destroy: func() { destroyed = true },
		}, nil
	}
	repo, _ := New(m, syntheticTestSecret, pinned)

	unlock, err := repo.TryProjectLock(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unlock(context.Background())
	if !destroyed {
		t.Error("connection should be destroyed when unlock fails")
	}
	if released {
		t.Error("connection should NOT be released back to pool when unlock fails")
	}
}

func TestAdvisoryLockKeys_DeterministicFromUUID(t *testing.T) {
	id := testUUID(0x02) // projectID
	keys := advisoryLockKeys(id)
	// First 4 bytes: 0x02, 0x00, 0x00, 0x00 → big-endian int32 = 0x02000000
	if keys.Column1 != 0x02000000 {
		t.Errorf("Column1 = %d, want %d", keys.Column1, 0x02000000)
	}
	if keys.Column2 != 0 {
		t.Errorf("Column2 = %d, want 0", keys.Column2)
	}
}
