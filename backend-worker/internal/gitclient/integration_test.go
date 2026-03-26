//go:build integration

package gitclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myjungle/backend-worker/internal/sshenv"
)

// Integration tests that exercise the real git binary against public repos.
// Run with: go test -tags integration -v ./internal/gitclient/...

const (
	httpsURL      = "https://github.com/anthropics/skills.git"
	sshURL        = "git@github.com:anthropics/skills.git"
	defaultBranch = "main"
)

func TestIntegration_Clone_HTTPS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	repoDir := filepath.Join(t.TempDir(), "repo")
	client := New(ExecRunner{})

	if err := client.Clone(ctx, httpsURL, defaultBranch, repoDir, nil); err != nil {
		t.Fatalf("Clone HTTPS failed: %v", err)
	}

	// Verify .git exists.
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Fatal(".git directory not found after clone")
	}

	// Verify remote URL.
	url, err := client.RemoteURL(ctx, repoDir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != httpsURL {
		t.Errorf("RemoteURL = %q, want %q", url, httpsURL)
	}

	// Verify HEAD SHA is a valid 40-char hex string.
	sha, err := client.HeadSHA(ctx, repoDir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("HeadSHA = %q, want 40-char hex", sha)
	}
	t.Logf("HEAD SHA: %s", sha)

	// Verify ListTrackedFiles returns files.
	files, err := client.ListTrackedFiles(ctx, repoDir)
	if err != nil {
		t.Fatalf("ListTrackedFiles: %v", err)
	}
	if len(files) == 0 {
		t.Error("ListTrackedFiles returned 0 files")
	}
	t.Logf("Tracked files: %d", len(files))

	// Spot-check: should contain at least a README or similar.
	hasReadme := false
	for _, f := range files {
		if strings.Contains(strings.ToLower(f), "readme") {
			hasReadme = true
			break
		}
	}
	if !hasReadme {
		t.Log("Warning: no README found in tracked files (not fatal)")
	}
}

func TestIntegration_Clone_SSH(t *testing.T) {
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

	// Set up SSH environment using the sshenv plumbing.
	hostname, err := sshenv.ParseHostname(sshURL)
	if err != nil {
		t.Fatalf("ParseHostname: %v", err)
	}

	tmpDir := t.TempDir()
	sshEnv, err := sshenv.Setup(ctx, tmpDir, privateKey, hostname, sshenv.ExecKeyscanner{})
	if err != nil {
		t.Fatalf("sshenv.Setup: %v", err)
	}
	defer sshEnv.Cleanup()

	repoDir := filepath.Join(t.TempDir(), "repo")
	client := New(ExecRunner{})

	// Pass the SSH env vars to Clone, exercising the full auth plumbing.
	if err := client.Clone(ctx, sshURL, defaultBranch, repoDir, sshEnv.EnvVars); err != nil {
		t.Fatalf("Clone SSH failed: %v", err)
	}

	// Verify remote URL.
	url, err := client.RemoteURL(ctx, repoDir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != sshURL {
		t.Errorf("RemoteURL = %q, want %q", url, sshURL)
	}

	sha, err := client.HeadSHA(ctx, repoDir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("HeadSHA = %q, want 40-char hex", sha)
	}
	t.Logf("HEAD SHA: %s", sha)
}

func TestIntegration_FetchAfterClone_HTTPS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	repoDir := filepath.Join(t.TempDir(), "repo")
	client := New(ExecRunner{})

	// Clone.
	if err := client.Clone(ctx, httpsURL, defaultBranch, repoDir, nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	shaBeforeFetch, err := client.HeadSHA(ctx, repoDir)
	if err != nil {
		t.Fatalf("HeadSHA before fetch: %v", err)
	}

	// Fetch (should succeed even with no new changes).
	if err := client.Fetch(ctx, repoDir, nil); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Checkout and reset.
	if err := client.Checkout(ctx, repoDir, defaultBranch); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	shaAfterFetch, err := client.HeadSHA(ctx, repoDir)
	if err != nil {
		t.Fatalf("HeadSHA after fetch: %v", err)
	}

	// SHAs should be the same (no new commits between clone and fetch).
	if shaBeforeFetch != shaAfterFetch {
		t.Logf("SHA changed during test (repo was updated): before=%s after=%s", shaBeforeFetch, shaAfterFetch)
	} else {
		t.Logf("SHA unchanged: %s", shaAfterFetch)
	}
}

