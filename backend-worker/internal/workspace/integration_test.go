//go:build integration

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/gitclient"
	"myjungle/backend-worker/internal/sshenv"
)

// Integration tests that exercise the full workspace Prepare flow against
// a real public repository.
// Run with: go test -tags integration -v ./internal/workspace/...

const (
	httpsURL      = "https://github.com/anthropics/skills.git"
	sshURL        = "git@github.com:anthropics/skills.git"
	defaultBranch = "main"
)

// panicKeyscanner panics if called, ensuring HTTPS paths never invoke keyscan.
type panicKeyscanner struct{}

func (panicKeyscanner) Scan(_ context.Context, _ string) ([]byte, error) {
	panic("keyscanner must not be called for HTTPS URLs")
}

func TestIntegration_Prepare_HTTPS_FirstClone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	baseDir := t.TempDir()
	client := gitclient.New(gitclient.ExecRunner{})
	mgr := New(baseDir, client, panicKeyscanner{})

	execCtx := &execution.Context{
		JobID:     testUUID(0x10),
		ProjectID: testUUID(0x20),
		RepoURL:   httpsURL,
		Branch:    defaultBranch,
	}

	result, cleanup, err := mgr.Prepare(ctx, execCtx)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer cleanup()

	// Commit SHA should be 40 hex chars.
	if len(result.CommitSHA) != 40 {
		t.Errorf("CommitSHA = %q, want 40-char hex", result.CommitSHA)
	}
	t.Logf("CommitSHA: %s", result.CommitSHA)

	// RepoDir should exist and contain .git.
	if _, err := os.Stat(filepath.Join(result.RepoDir, ".git")); err != nil {
		t.Errorf(".git not found in RepoDir %s: %v", result.RepoDir, err)
	}

	// SourceFiles should contain text files (no restriction on extensions).
	if len(result.SourceFiles) == 0 {
		t.Error("expected at least some source files")
	}
	t.Logf("SourceFiles: %d", len(result.SourceFiles))

	// Log a few files for visibility.
	for i, f := range result.SourceFiles {
		if i >= 10 {
			t.Logf("  ... and %d more", len(result.SourceFiles)-10)
			break
		}
		t.Logf("  %s", f)
	}
}

func TestIntegration_Prepare_HTTPS_SecondRunFetches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	baseDir := t.TempDir()
	client := gitclient.New(gitclient.ExecRunner{})
	mgr := New(baseDir, client, panicKeyscanner{})

	execCtx := &execution.Context{
		JobID:     testUUID(0x10),
		ProjectID: testUUID(0x20),
		RepoURL:   httpsURL,
		Branch:    defaultBranch,
	}

	// First run: clone.
	result1, cleanup1, err := mgr.Prepare(ctx, execCtx)
	if err != nil {
		t.Fatalf("First Prepare failed: %v", err)
	}
	cleanup1()

	// Cache should survive job cleanup.
	cacheDir := mgr.ProjectRepoDir(execCtx.ProjectID)
	if _, err := os.Stat(filepath.Join(cacheDir, ".git")); err != nil {
		t.Fatalf("project repo cache should survive cleanup: %v", err)
	}

	// Second run: should reuse cache and fetch.
	execCtx.JobID = testUUID(0x11) // different job ID
	result2, cleanup2, err := mgr.Prepare(ctx, execCtx)
	if err != nil {
		t.Fatalf("Second Prepare failed: %v", err)
	}
	defer cleanup2()

	// Worktree dirs should differ between jobs.
	if result1.RepoDir == result2.RepoDir {
		t.Error("worktree dirs should differ between jobs")
	}

	// Both runs should produce the same commit SHA (no new commits between runs).
	if result1.CommitSHA != result2.CommitSHA {
		t.Logf("SHA changed between runs (repo updated): %s -> %s", result1.CommitSHA, result2.CommitSHA)
	} else {
		t.Logf("SHA unchanged: %s", result2.CommitSHA)
	}

	// Shared cache should still exist after the second run.
	if _, err := os.Stat(filepath.Join(cacheDir, ".git")); err != nil {
		t.Errorf("project repo cache should exist: %v", err)
	}
}

func TestIntegration_Prepare_HTTPS_URLMismatchDetected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	baseDir := t.TempDir()
	client := gitclient.New(gitclient.ExecRunner{})
	mgr := New(baseDir, client, panicKeyscanner{})

	execCtx := &execution.Context{
		JobID:     testUUID(0x10),
		ProjectID: testUUID(0x20),
		RepoURL:   httpsURL,
		Branch:    defaultBranch,
	}

	// First run: clone.
	_, cleanup1, err := mgr.Prepare(ctx, execCtx)
	if err != nil {
		t.Fatalf("First Prepare failed: %v", err)
	}
	cleanup1()

	// Second run with different URL for same project: should fail.
	execCtx.JobID = testUUID(0x11)
	execCtx.RepoURL = "https://github.com/anthropics/courses.git"

	_, _, err = mgr.Prepare(ctx, execCtx)
	if err == nil {
		t.Fatal("expected error for URL mismatch, got nil")
	}
	t.Logf("Got expected error: %v", err)
}

func TestIntegration_Prepare_SSH(t *testing.T) {
	keyPath := os.Getenv("SSH_INTEGRATION_KEY")
	if keyPath == "" {
		t.Skip("skipping SSH test: SSH_INTEGRATION_KEY not set")
	}

	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read SSH key %s: %v", keyPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	baseDir := t.TempDir()
	client := gitclient.New(gitclient.ExecRunner{})
	mgr := New(baseDir, client, sshenv.ExecKeyscanner{})

	execCtx := &execution.Context{
		JobID:         testUUID(0x30),
		ProjectID:     testUUID(0x40),
		RepoURL:       sshURL,
		Branch:        defaultBranch,
		SSHPrivateKey: privateKey,
	}

	result, cleanup, err := mgr.Prepare(ctx, execCtx)
	if err != nil {
		t.Fatalf("Prepare SSH failed: %v", err)
	}
	defer cleanup()

	if len(result.CommitSHA) != 40 {
		t.Errorf("CommitSHA = %q, want 40-char hex", result.CommitSHA)
	}
	t.Logf("SSH clone CommitSHA: %s", result.CommitSHA)
	t.Logf("SSH clone SourceFiles: %d", len(result.SourceFiles))
}

func TestIntegration_Cleanup_RemovesTmpPreservesCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	baseDir := t.TempDir()
	client := gitclient.New(gitclient.ExecRunner{})
	mgr := New(baseDir, client, panicKeyscanner{})

	execCtx := &execution.Context{
		JobID:     testUUID(0x50),
		ProjectID: testUUID(0x60),
		RepoURL:   httpsURL,
		Branch:    defaultBranch,
	}

	result, cleanup, err := mgr.Prepare(ctx, execCtx)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	tmpDir := mgr.JobTmpDir(execCtx.JobID)
	worktreeDir := result.RepoDir
	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatalf("tmp dir should exist before cleanup: %v", err)
	}

	cleanup()

	// Job tmp dir should be removed.
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("tmp dir should be removed after cleanup")
	}

	// Worktree dir should be removed.
	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Error("worktree dir should be removed after cleanup")
	}

	// Project repo cache should still exist.
	cacheDir := mgr.ProjectRepoDir(execCtx.ProjectID)
	if _, err := os.Stat(filepath.Join(cacheDir, ".git")); err != nil {
		t.Error("repo cache should be preserved after cleanup")
	}
}


