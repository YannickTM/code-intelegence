package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/gitclient"
)

// --- helpers ---

func testUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

func testExecCtx() *execution.Context {
	return &execution.Context{
		JobID:         testUUID(0x01),
		ProjectID:     testUUID(0x02),
		RepoURL:       "git@github.com:org/repo.git",
		Branch:        "main",
		SSHPrivateKey: []byte("FAKE-SSH-KEY"),
	}
}

// --- fake git runner ---

type fakeRunCall struct {
	Dir  string
	Env  []string
	Args []string
}

type fakeRunResult struct {
	Output []byte
	Err    error
}

type fakeGitRunner struct {
	mu      sync.Mutex
	calls   []fakeRunCall
	results []fakeRunResult
	effects []func() // optional side-effects per call index
	idx     int
}

func (f *fakeGitRunner) Run(_ context.Context, dir string, env []string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeRunCall{Dir: dir, Env: env, Args: args})
	if f.idx >= len(f.results) {
		return nil, errors.New("fakeGitRunner: no more results")
	}
	if f.idx < len(f.effects) && f.effects[f.idx] != nil {
		f.effects[f.idx]()
	}
	r := f.results[f.idx]
	f.idx++
	return r.Output, r.Err
}

// --- fake keyscanner ---

type fakeKeyscanner struct {
	output []byte
	err    error
}

func (f *fakeKeyscanner) Scan(_ context.Context, _ string) ([]byte, error) {
	return f.output, f.err
}

// failKeyscanner fails the test if Scan is called.
type failKeyscanner struct {
	t *testing.T
}

func (f *failKeyscanner) Scan(_ context.Context, _ string) ([]byte, error) {
	f.t.Fatal("keyscanner should not be called for HTTPS URLs")
	return nil, nil
}

// --- helper to create test files ---

func createTestFiles(t *testing.T, dir string, files []string) {
	t.Helper()
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// --- Prepare tests ---

func TestPrepare_NewProject_Clones(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	tmpCloneDir := repoDir + ".cloning-" + fmtUUID(execCtx.JobID)
	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	testFiles := []string{"src/index.ts", "src/app.tsx", "README.md", "package.json"}

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("Cloning...\n")},                                                  // Clone
			{Output: []byte("Preparing worktree\n")},                                          // WorktreeAdd
			{Output: []byte("abc123def456789\n")},                                             // HeadSHA
			{Output: []byte("src/index.ts\x00src/app.tsx\x00README.md\x00package.json\x00")},  // ListTrackedFiles
			{Output: []byte("")},                                                              // WorktreeRemove (cleanup)
		},
		effects: []func(){
			func() {
				// Clone: create temp clone dir with .git (simulates real clone).
				os.MkdirAll(filepath.Join(tmpCloneDir, ".git"), 0755)
			},
			func() {
				// WorktreeAdd: create worktree dir with test files.
				createTestFiles(t, worktreeDir, testFiles)
			},
			nil, // HeadSHA
			nil, // ListTrackedFiles
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("github.com ssh-ed25519 AAAA...\n")}
	mgr := New(baseDir, git, scanner)

	result, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if result.CommitSHA != "abc123def456789" {
		t.Errorf("CommitSHA = %q, want %q", result.CommitSHA, "abc123def456789")
	}
	// All text files should pass the filter (no extension restriction).
	if len(result.SourceFiles) != 4 {
		t.Errorf("SourceFiles = %v, want 4 files", result.SourceFiles)
	}
	// RepoDir should point to the worktree.
	if result.RepoDir != worktreeDir {
		t.Errorf("RepoDir = %q, want %q", result.RepoDir, worktreeDir)
	}

	// Verify first call was clone.
	if len(runner.calls) < 1 {
		t.Fatal("expected at least 1 git call")
	}
	cloneCall := runner.calls[0]
	if len(cloneCall.Args) < 1 || cloneCall.Args[0] != "clone" {
		t.Errorf("first call should be clone, got %v", cloneCall.Args)
	}
}

func TestPrepare_ExistingCache_Fetches(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	// Pre-create the repo cache with a .git directory.
	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("git@github.com:org/repo.git\n")},  // RemoteURL
			{Output: []byte("")},                                // Fetch
			{Output: []byte("Preparing worktree\n")},           // WorktreeAdd
			{Output: []byte("def456\n")},                       // HeadSHA
			{Output: []byte("lib/helper.js\x00")},              // ListTrackedFiles
			{Output: []byte("")},                                // WorktreeRemove (cleanup)
		},
		effects: []func(){
			nil, // RemoteURL
			nil, // Fetch
			func() {
				// WorktreeAdd: create worktree dir with test files.
				createTestFiles(t, worktreeDir, []string{"lib/helper.js"})
			},
			nil, // HeadSHA
			nil, // ListTrackedFiles
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("github.com ssh-rsa AAAA...\n")}
	mgr := New(baseDir, git, scanner)

	result, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if result.CommitSHA != "def456" {
		t.Errorf("CommitSHA = %q, want %q", result.CommitSHA, "def456")
	}
	if len(result.SourceFiles) != 1 || result.SourceFiles[0] != "lib/helper.js" {
		t.Errorf("SourceFiles = %v, want [lib/helper.js]", result.SourceFiles)
	}

	// First call should be remote get-url (not clone).
	if len(runner.calls) < 1 {
		t.Fatal("expected git calls")
	}
	if runner.calls[0].Args[0] != "remote" {
		t.Errorf("first call should be 'remote', got %v", runner.calls[0].Args)
	}
}

func TestPrepare_ExistingCache_DifferentBranch(t *testing.T) {
	baseDir := t.TempDir()

	execCtx := &execution.Context{
		JobID:         testUUID(0x10),
		ProjectID:     testUUID(0x02),
		RepoURL:       "git@github.com:org/repo.git",
		Branch:        "feature-x",
		SSHPrivateKey: []byte("FAKE-SSH-KEY"),
	}

	// Pre-create the repo cache (simulating a previous clone for "main").
	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("git@github.com:org/repo.git\n")}, // RemoteURL
			{Output: []byte("")},                               // Fetch
			{Output: []byte("Preparing worktree\n")},          // WorktreeAdd
			{Output: []byte("featuresha123\n")},                // HeadSHA
			{Output: []byte("src/feature.ts\x00")},            // ListTrackedFiles
			{Output: []byte("")},                               // WorktreeRemove (cleanup)
		},
		effects: []func(){
			nil, // RemoteURL
			nil, // Fetch
			func() {
				createTestFiles(t, worktreeDir, []string{"src/feature.ts"})
			},
			nil, // HeadSHA
			nil, // ListTrackedFiles
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("github.com ssh-ed25519 AAAA...\n")}
	mgr := New(baseDir, git, scanner)

	result, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if result.CommitSHA != "featuresha123" {
		t.Errorf("CommitSHA = %q, want %q", result.CommitSHA, "featuresha123")
	}

	// First call should be RemoteURL (fetch path, not clone).
	if runner.calls[0].Args[0] != "remote" {
		t.Errorf("first call should be 'remote' (fetch path), got %v", runner.calls[0].Args)
	}

	// Verify WorktreeAdd was called with "origin/feature-x".
	var worktreeAddCall *fakeRunCall
	for i := range runner.calls {
		if len(runner.calls[i].Args) > 0 && runner.calls[i].Args[0] == "worktree" {
			worktreeAddCall = &runner.calls[i]
			break
		}
	}
	if worktreeAddCall == nil {
		t.Fatal("expected a worktree add call")
	}
	lastArg := worktreeAddCall.Args[len(worktreeAddCall.Args)-1]
	if lastArg != "origin/feature-x" {
		t.Errorf("worktree add ref = %q, want %q", lastArg, "origin/feature-x")
	}
}

func TestPrepare_URLMismatch_ReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()
	// Use HTTPS so SSH setup is skipped, and embed credentials in the
	// cached URL to verify they are redacted in the error message.
	execCtx.RepoURL = "https://github.com/org/repo.git"

	// Pre-create cache.
	mgr := &Manager{baseDir: baseDir}
	repoDir := mgr.ProjectRepoDir(execCtx.ProjectID)
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	runner := &fakeGitRunner{results: []fakeRunResult{
		// RemoteURL — different URL with embedded credentials.
		{Output: []byte("https://user:secret-token@github.com/other/repo.git\n")},
	}}
	git := gitclient.New(runner)
	scanner := &failKeyscanner{t: t}
	mgr = New(baseDir, git, scanner)

	_, _, err := mgr.Prepare(context.Background(), execCtx)
	if err == nil {
		t.Fatal("expected error for URL mismatch")
	}
	if !strings.Contains(err.Error(), "remote URL mismatch") {
		t.Errorf("error should contain 'remote URL mismatch', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "other/repo") {
		t.Errorf("error should contain the cached URL, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("error should not contain credentials, got %q", err.Error())
	}
}

func TestPrepare_CleanupRemovesJobDir(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	tmpCloneDir := repoDir + ".cloning-" + fmtUUID(execCtx.JobID)
	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("")},       // Clone
			{Output: []byte("")},       // WorktreeAdd
			{Output: []byte("abc\n")},  // HeadSHA
			{Output: []byte("")},       // ListTrackedFiles
			{Output: []byte("")},       // WorktreeRemove (cleanup)
		},
		effects: []func(){
			func() { os.MkdirAll(filepath.Join(tmpCloneDir, ".git"), 0755) },
			func() { os.MkdirAll(worktreeDir, 0755) },
			nil,
			nil,
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("h ssh-rsa AAAA\n")}
	mgr := New(baseDir, git, scanner)

	_, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpDir := mgr.JobTmpDir(execCtx.JobID)
	if _, statErr := os.Stat(tmpDir); statErr != nil {
		t.Fatalf("tmp dir should exist before cleanup: %v", statErr)
	}
	if _, statErr := os.Stat(worktreeDir); statErr != nil {
		t.Fatalf("worktree dir should exist before cleanup: %v", statErr)
	}

	cleanup()

	if _, statErr := os.Stat(tmpDir); !os.IsNotExist(statErr) {
		t.Error("tmp dir should be deleted after cleanup")
	}
	if _, statErr := os.Stat(worktreeDir); !os.IsNotExist(statErr) {
		t.Error("worktree dir should be deleted after cleanup")
	}
}

func TestPrepare_CleanupPreservesRepoCache(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	tmpCloneDir := repoDir + ".cloning-" + fmtUUID(execCtx.JobID)
	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("")},       // Clone
			{Output: []byte("")},       // WorktreeAdd
			{Output: []byte("abc\n")},  // HeadSHA
			{Output: []byte("")},       // ListTrackedFiles
			{Output: []byte("")},       // WorktreeRemove (cleanup)
		},
		effects: []func(){
			func() { os.MkdirAll(filepath.Join(tmpCloneDir, ".git"), 0755) },
			func() { os.MkdirAll(worktreeDir, 0755) },
			nil,
			nil,
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("h ssh-rsa AAAA\n")}
	mgr := New(baseDir, git, scanner)

	_, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cleanup()

	// The repo cache should still exist (cleanup only removes job dir).
	if _, statErr := os.Stat(repoDir); os.IsNotExist(statErr) {
		t.Error("repo cache should be preserved after cleanup")
	}
	if _, statErr := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(statErr) {
		t.Error("repo cache .git should be preserved after cleanup")
	}
}

func TestPrepare_StaleJobDirRemoved(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	tmpCloneDir := repoDir + ".cloning-" + fmtUUID(execCtx.JobID)
	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	// Create a stale job directory with leftover files from a previous run.
	staleJobDir := filepath.Join(baseDir, "jobs", fmtUUID(execCtx.JobID))
	staleFile := filepath.Join(staleJobDir, "tmp", "stale-ssh-key")
	if err := os.MkdirAll(filepath.Dir(staleFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleFile, []byte("old-key-material"), 0600); err != nil {
		t.Fatal(err)
	}

	testFiles := []string{"src/index.ts"}

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("")},                  // Clone
			{Output: []byte("")},                  // WorktreeAdd
			{Output: []byte("abc\n")},             // HeadSHA
			{Output: []byte("src/index.ts\x00")},  // ListTrackedFiles
			{Output: []byte("")},                  // WorktreeRemove (cleanup)
		},
		effects: []func(){
			func() { os.MkdirAll(filepath.Join(tmpCloneDir, ".git"), 0755) },
			func() { createTestFiles(t, worktreeDir, testFiles) },
			nil,
			nil,
			nil,
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("github.com ssh-ed25519 AAAA...\n")}
	mgr := New(baseDir, git, scanner)

	// Verify stale file exists before Prepare.
	if _, err := os.Stat(staleFile); err != nil {
		t.Fatalf("stale file should exist before Prepare: %v", err)
	}

	result, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	// Stale file should have been removed.
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Error("stale file should be removed by Prepare")
	}

	// New workspace should be functional.
	if result.CommitSHA != "abc" {
		t.Errorf("CommitSHA = %q, want %q", result.CommitSHA, "abc")
	}
	if len(result.SourceFiles) != 1 {
		t.Errorf("SourceFiles = %v, want 1 file", result.SourceFiles)
	}
}

func TestLockProject_CreatesLockFile(t *testing.T) {
	baseDir := t.TempDir()
	mgr := &Manager{baseDir: baseDir}

	unlock, err := mgr.lockProject(context.Background(), "test-project")
	if err != nil {
		t.Fatalf("lockProject failed: %v", err)
	}
	defer unlock()

	lockPath := filepath.Join(baseDir, "projects", "test-project", ".lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file should exist at %s: %v", lockPath, err)
	}
}

func TestLockProject_Exclusivity(t *testing.T) {
	baseDir := t.TempDir()
	mgr := &Manager{baseDir: baseDir}

	unlock1, err := mgr.lockProject(context.Background(), "excl-project")
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		unlock2, err := mgr.lockProject(context.Background(), "excl-project")
		if err != nil {
			t.Errorf("second lock failed: %v", err)
			return
		}
		close(acquired)
		unlock2()
	}()

	// The second lock should be blocked while the first is held.
	select {
	case <-acquired:
		t.Fatal("second lock acquired while first is still held")
	case <-time.After(100 * time.Millisecond):
		// expected — still blocked
	}

	// Release the first lock; the second should now acquire.
	unlock1()

	select {
	case <-acquired:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("second lock not acquired after first was released")
	}
}

// --- Directory path tests ---

func TestProjectRepoDir_Format(t *testing.T) {
	mgr := &Manager{baseDir: "/cache"}
	dir := mgr.ProjectRepoDir(testUUID(0xAB))
	wantPrefix := filepath.Join("/cache", "projects") + string(os.PathSeparator)
	if !strings.HasPrefix(dir, wantPrefix) {
		t.Errorf("dir should start with %s, got %q", wantPrefix, dir)
	}
	wantSuffix := string(os.PathSeparator) + "repo"
	if !strings.HasSuffix(dir, wantSuffix) {
		t.Errorf("dir should end with %s, got %q", wantSuffix, dir)
	}
}

func TestJobTmpDir_Format(t *testing.T) {
	mgr := &Manager{baseDir: "/cache"}
	dir := mgr.JobTmpDir(testUUID(0xCD))
	wantPrefix := filepath.Join("/cache", "jobs") + string(os.PathSeparator)
	if !strings.HasPrefix(dir, wantPrefix) {
		t.Errorf("dir should start with %s, got %q", wantPrefix, dir)
	}
	wantSuffix := string(os.PathSeparator) + "tmp"
	if !strings.HasSuffix(dir, wantSuffix) {
		t.Errorf("dir should end with %s, got %q", wantSuffix, dir)
	}
}

func TestJobWorktreeDir_Format(t *testing.T) {
	mgr := &Manager{baseDir: "/cache"}
	dir := mgr.JobWorktreeDir(testUUID(0xEF))
	wantPrefix := filepath.Join("/cache", "jobs") + string(os.PathSeparator)
	if !strings.HasPrefix(dir, wantPrefix) {
		t.Errorf("dir should start with %s, got %q", wantPrefix, dir)
	}
	wantSuffix := string(os.PathSeparator) + "worktree"
	if !strings.HasSuffix(dir, wantSuffix) {
		t.Errorf("dir should end with %s, got %q", wantSuffix, dir)
	}
}

// --- filterSourceFiles tests ---

func TestFilterSourceFiles_AllTextFilesIncluded(t *testing.T) {
	repoDir := t.TempDir()

	// Create test files — all text files should be included.
	files := []string{
		"src/index.ts",
		"src/app.tsx",
		"lib/util.js",
		"lib/helper.jsx",
		"config.mjs",
		"setup.cjs",
		"README.md",
		"package.json",
		"style.css",
		"data.go",
	}
	createTestFiles(t, repoDir, files)

	result, err := filterSourceFiles(repoDir, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All text files should pass — sorted alphabetically.
	want := []string{
		"README.md",
		"config.mjs",
		"data.go",
		"lib/helper.jsx",
		"lib/util.js",
		"package.json",
		"setup.cjs",
		"src/app.tsx",
		"src/index.ts",
		"style.css",
	}
	if len(result) != len(want) {
		t.Fatalf("result = %v, want %v", result, want)
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want[i])
		}
	}
}

func TestFilterSourceFiles_SizeFilter(t *testing.T) {
	repoDir := t.TempDir()

	// Create a small file and a large file.
	smallPath := filepath.Join(repoDir, "small.ts")
	if err := os.WriteFile(smallPath, []byte("const x = 1;"), 0644); err != nil {
		t.Fatal(err)
	}

	largePath := filepath.Join(repoDir, "large.ts")
	bigContent := make([]byte, MaxFileSize+1)
	if err := os.WriteFile(largePath, bigContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a file exactly at the boundary (should be excluded).
	boundaryPath := filepath.Join(repoDir, "boundary.ts")
	boundaryContent := make([]byte, MaxFileSize)
	if err := os.WriteFile(boundaryPath, boundaryContent, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := filterSourceFiles(repoDir, []string{"small.ts", "large.ts", "boundary.ts"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "small.ts" {
		t.Errorf("result = %v, want [small.ts] (boundary and large should be excluded)", result)
	}
}

func TestFilterSourceFiles_BinaryIncluded(t *testing.T) {
	repoDir := t.TempDir()

	// Create a text file.
	if err := os.WriteFile(filepath.Join(repoDir, "readme.md"), []byte("# Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a binary file (contains NULL bytes).
	if err := os.WriteFile(filepath.Join(repoDir, "image.png"), []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := filterSourceFiles(repoDir, []string{"readme.md", "image.png"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("result = %v, want both files (binary files are included)", result)
	}
}

func TestFilterSourceFiles_StableOrdering(t *testing.T) {
	repoDir := t.TempDir()

	files := []string{"z.ts", "a.ts", "m.tsx"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(repoDir, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := filterSourceFiles(repoDir, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a.ts", "m.tsx", "z.ts"}
	if len(result) != len(want) {
		t.Fatalf("result = %v, want %v", result, want)
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want[i])
		}
	}
}

func TestFilterSourceFiles_EmptyList(t *testing.T) {
	repoDir := t.TempDir()
	result, err := filterSourceFiles(repoDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestFilterSourceFiles_MissingFileSkipped(t *testing.T) {
	repoDir := t.TempDir()

	// Only create one of the two files.
	if err := os.WriteFile(filepath.Join(repoDir, "exists.ts"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := filterSourceFiles(repoDir, []string{"exists.ts", "missing.ts"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "exists.ts" {
		t.Errorf("result = %v, want [exists.ts]", result)
	}
}

func TestFilterSourceFiles_SymlinkSkipped(t *testing.T) {
	repoDir := t.TempDir()

	// Create a real file and a symlink to it.
	realPath := filepath.Join(repoDir, "real.ts")
	if err := os.WriteFile(realPath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(repoDir, "link.ts")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}

	result, err := filterSourceFiles(repoDir, []string{"real.ts", "link.ts"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "real.ts" {
		t.Errorf("result = %v, want [real.ts] (symlink should be skipped)", result)
	}
}

// --- normalizeURL tests ---

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"git@github.com:org/repo.git", "git@github.com:org/repo", true},
		{"git@github.com:org/repo.git", "git@github.com:org/repo.git", true},
		{"git@github.com:org/repo/", "git@github.com:org/repo", true},
		{"git@github.com:org/repo.git/", "git@github.com:org/repo", true},
		{"git@github.com:org/repo.git", "git@github.com:other/repo.git", false},
		// Userinfo stripping — same repo with different credentials.
		{"https://user:token@github.com/org/repo.git", "https://github.com/org/repo", true},
		{"https://x-access-token:ghp_abc@github.com/org/repo", "https://github.com/org/repo", true},
		{"https://user:token@github.com/org/repo.git", "https://other:pass@github.com/org/repo.git", true},
		// SSH usernames are preserved — different users are different repos.
		{"ssh://git@github.com/org/repo", "ssh://github.com/org/repo", false},
		{"ssh://alice@github.com/org/repo", "ssh://bob@github.com/org/repo", false},
		// HTTPS credentials are still stripped.
		{"https://user:pass@github.com/org/repo", "https://github.com/org/repo", true},
		// SSH form equivalence — scp-style vs ssh:// vs ssh:// with port 22.
		{"git@github.com:org/repo.git", "ssh://git@github.com/org/repo.git", true},
		{"ssh://git@github.com:22/org/repo.git", "git@github.com:org/repo.git", true},
		{"ssh://git@github.com:22/org/repo.git", "ssh://git@github.com/org/repo.git", true},
		// Non-default SSH port — should not match.
		{"ssh://git@github.com:2222/org/repo", "git@github.com:org/repo.git", false},
		// Different hosts still mismatch across SSH forms.
		{"git@github.com:org/repo.git", "ssh://git@gitlab.com/org/repo.git", false},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			got := normalizeURL(tc.a) == normalizeURL(tc.b)
			if got != tc.want {
				t.Errorf("normalizeURL(%q) == normalizeURL(%q) = %v, want %v",
					tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// --- sanitizeURL tests ---

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "scp-style unchanged",
			url:  "git@github.com:org/repo.git",
			want: "git@github.com:org/repo.git",
		},
		{
			name: "https no credentials",
			url:  "https://github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "https with token",
			url:  "https://user:ghp_secret@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "https with username only",
			url:  "https://x-access-token@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "ssh with user",
			url:  "ssh://git@github.com/org/repo.git",
			want: "ssh://github.com/org/repo.git",
		},
		{
			name: "ssh with user:token",
			url:  "ssh://user:secret@github.com/org/repo.git",
			want: "ssh://github.com/org/repo.git",
		},
		{
			name: "unparseable URL returns placeholder",
			url:  "https://user:pass@host\x7f/repo",
			want: "<redacted-url>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeURL(tc.url)
			if got != tc.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// --- fmtUUID tests ---

func TestFmtUUID_Valid(t *testing.T) {
	u := testUUID(0xAB)
	s := fmtUUID(u)
	if !strings.HasPrefix(s, "ab") {
		t.Errorf("fmtUUID = %q, want prefix 'ab'", s)
	}
	if strings.Count(s, "-") != 4 {
		t.Errorf("fmtUUID = %q, want 4 dashes", s)
	}
}

func TestFmtUUID_Invalid(t *testing.T) {
	var u pgtype.UUID
	if fmtUUID(u) != "<nil>" {
		t.Errorf("fmtUUID(invalid) = %q, want %q", fmtUUID(u), "<nil>")
	}
}

// --- isSSHTransport tests ---

func TestIsSSHTransport(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"git@github.com:org/repo.git", true},
		{"ssh://git@github.com/org/repo.git", true},
		{"https://github.com/org/repo.git", false},
		{"http://github.com/org/repo.git", false},
		{"https://user:token@github.com/org/repo.git", false},
		{"not-a-url", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			if got := isSSHTransport(tc.url); got != tc.want {
				t.Errorf("isSSHTransport(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// --- HTTPS skips SSH setup test ---

func TestPrepare_HTTPS_SkipsSSHSetup(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := &execution.Context{
		JobID:     testUUID(0x01),
		ProjectID: testUUID(0x02),
		RepoURL:   "https://github.com/org/repo.git",
		Branch:    "main",
		// SSHPrivateKey intentionally omitted.
	}

	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	tmpCloneDir := repoDir + ".cloning-" + fmtUUID(execCtx.JobID)
	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("")},      // Clone
			{Output: []byte("")},      // WorktreeAdd
			{Output: []byte("abc\n")}, // HeadSHA
			{Output: []byte("")},      // ListTrackedFiles
			{Output: []byte("")},      // WorktreeRemove (cleanup)
		},
		effects: []func(){
			func() { os.MkdirAll(filepath.Join(tmpCloneDir, ".git"), 0755) },
			func() { os.MkdirAll(worktreeDir, 0755) },
			nil,
			nil,
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	mgr := New(baseDir, git, &failKeyscanner{t: t})

	_, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	// Verify Clone was called without GIT_SSH_COMMAND.
	cloneCall := runner.calls[0]
	for _, e := range cloneCall.Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			t.Errorf("HTTPS clone should not have GIT_SSH_COMMAND, got %v", cloneCall.Env)
		}
	}

	// Verify no SSH files were written.
	tmpDir := mgr.JobTmpDir(execCtx.JobID)
	if _, err := os.Stat(filepath.Join(tmpDir, "id_key")); !os.IsNotExist(err) {
		t.Error("id_key should not exist for HTTPS URLs")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "known_hosts")); !os.IsNotExist(err) {
		t.Error("known_hosts should not exist for HTTPS URLs")
	}
}

// --- Prepare UUID validation tests ---

func TestPrepare_NilExecCtx(t *testing.T) {
	mgr := New(t.TempDir(), gitclient.New(&fakeGitRunner{}), &fakeKeyscanner{})
	_, _, err := mgr.Prepare(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil execution context")
	}
	if !strings.Contains(err.Error(), "nil execution context") {
		t.Errorf("error should mention nil execution context, got %q", err.Error())
	}
}

func TestPrepare_InvalidJobID(t *testing.T) {
	mgr := New(t.TempDir(), gitclient.New(&fakeGitRunner{}), &fakeKeyscanner{})
	execCtx := &execution.Context{
		JobID:     pgtype.UUID{Valid: false},
		ProjectID: testUUID(0x01),
		RepoURL:   "https://github.com/org/repo.git",
		Branch:    "main",
	}
	_, _, err := mgr.Prepare(context.Background(), execCtx)
	if err == nil {
		t.Fatal("expected error for invalid JobID")
	}
	if !strings.Contains(err.Error(), "JobID") {
		t.Errorf("error should mention JobID, got %q", err.Error())
	}
}

func TestPrepare_InvalidProjectID(t *testing.T) {
	mgr := New(t.TempDir(), gitclient.New(&fakeGitRunner{}), &fakeKeyscanner{})
	execCtx := &execution.Context{
		JobID:     testUUID(0x01),
		ProjectID: pgtype.UUID{Valid: false},
		RepoURL:   "https://github.com/org/repo.git",
		Branch:    "main",
	}
	_, _, err := mgr.Prepare(context.Background(), execCtx)
	if err == nil {
		t.Fatal("expected error for invalid ProjectID")
	}
	if !strings.Contains(err.Error(), "ProjectID") {
		t.Errorf("error should mention ProjectID, got %q", err.Error())
	}
}

// --- HasParserSupport tests ---

func TestHasParserSupport(t *testing.T) {
	supported := []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".go", ".rs", ".py", ".java", ".c", ".cpp", ".json", ".md", ".yaml"}
	for _, ext := range supported {
		if !HasParserSupport(ext) {
			t.Errorf("HasParserSupport(%q) = false, want true", ext)
		}
	}
	unsupported := []string{".xyz", ".unknown", ""}
	for _, ext := range unsupported {
		if HasParserSupport(ext) {
			t.Errorf("HasParserSupport(%q) = true, want false", ext)
		}
	}
}

// --- SSH prerequisite validation tests ---

func TestPrepare_SSH_EmptyKey_FailsFast(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()
	execCtx.SSHPrivateKey = nil // empty key

	runner := &fakeGitRunner{}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("github.com ssh-ed25519 AAAA...\n")}
	mgr := New(baseDir, git, scanner)

	_, _, err := mgr.Prepare(context.Background(), execCtx)
	if err == nil {
		t.Fatal("expected error for empty SSH private key")
	}
	if !strings.Contains(err.Error(), "SSH private key") {
		t.Errorf("error should mention 'SSH private key', got %q", err.Error())
	}
	// Verify no key file was written to disk.
	tmpDir := mgr.JobTmpDir(execCtx.JobID)
	if _, statErr := os.Stat(filepath.Join(tmpDir, "id_key")); !os.IsNotExist(statErr) {
		t.Error("id_key should not be written when SSHPrivateKey is empty")
	}
}

func TestPrepare_SSH_NilScanner_FailsFast(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	runner := &fakeGitRunner{}
	git := gitclient.New(runner)
	mgr := New(baseDir, git, nil) // nil scanner

	_, _, err := mgr.Prepare(context.Background(), execCtx)
	if err == nil {
		t.Fatal("expected error for nil keyscanner")
	}
	if !strings.Contains(err.Error(), "SSH keyscanner") {
		t.Errorf("error should mention 'SSH keyscanner', got %q", err.Error())
	}
}

// --- Context-aware lock test ---

func TestLockProject_RespectsContext(t *testing.T) {
	baseDir := t.TempDir()
	mgr := &Manager{baseDir: baseDir}

	// Acquire first lock.
	unlock1, err := mgr.lockProject(context.Background(), "ctx-project")
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer unlock1()

	// Try second lock with a short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = mgr.lockProject(ctx, "ctx-project")
	if err == nil {
		t.Fatal("expected error when context times out")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// --- Cleanup worktree preservation test ---

func TestCleanup_PreservesWorktreeOnFailedRemove(t *testing.T) {
	baseDir := t.TempDir()
	execCtx := testExecCtx()

	mgr0 := &Manager{baseDir: baseDir}
	repoDir := mgr0.ProjectRepoDir(execCtx.ProjectID)
	tmpCloneDir := repoDir + ".cloning-" + fmtUUID(execCtx.JobID)
	worktreeDir := mgr0.JobWorktreeDir(execCtx.JobID)

	runner := &fakeGitRunner{
		results: []fakeRunResult{
			{Output: []byte("")},      // Clone
			{Output: []byte("")},      // WorktreeAdd
			{Output: []byte("abc\n")}, // HeadSHA
			{Output: []byte("")},      // ListTrackedFiles
			{Err: errors.New("fatal: worktree remove failed")}, // WorktreeRemove FAILS
		},
		effects: []func(){
			func() { os.MkdirAll(filepath.Join(tmpCloneDir, ".git"), 0755) },
			func() { os.MkdirAll(worktreeDir, 0755) },
			nil, // HeadSHA
			nil, // ListTrackedFiles
			nil, // WorktreeRemove
		},
	}
	git := gitclient.New(runner)
	scanner := &fakeKeyscanner{output: []byte("h ssh-rsa AAAA\n")}
	mgr := New(baseDir, git, scanner)

	_, cleanup, err := mgr.Prepare(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpDir := mgr.JobTmpDir(execCtx.JobID)

	cleanup()

	// tmp dir should be removed.
	if _, statErr := os.Stat(tmpDir); !os.IsNotExist(statErr) {
		t.Error("tmp dir should be removed after cleanup")
	}
	// worktree dir should be preserved (WorktreeRemove failed).
	if _, statErr := os.Stat(worktreeDir); statErr != nil {
		t.Error("worktree dir should be preserved when WorktreeRemove fails")
	}
}
