// Package workspace manages the repo cache directory layout and orchestrates
// git + SSH operations for indexing jobs.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-worker/internal/execution"
	"myjungle/backend-worker/internal/gitclient"
	"myjungle/backend-worker/internal/logger"
	"myjungle/backend-worker/internal/parser/registry"
	"myjungle/backend-worker/internal/sshenv"
)

// HasParserSupport reports whether ext (e.g. ".ts", ".rs") has tree-sitter
// parser support for rich extraction (symbols, chunks, imports).
// All other text files are indexed with raw content chunks.
func HasParserSupport(ext string) bool {
	_, ok := registry.GetLanguageByExtension(ext)
	return ok
}

// MaxFileSize is the maximum file size in bytes for source files to be included.
const MaxFileSize = 1 << 20 // 1 MB

// Result holds the output of a successful workspace preparation.
type Result struct {
	RepoDir     string   // absolute path to the job worktree
	CommitSHA   string   // HEAD SHA after checkout
	SourceFiles []string // filtered, sorted source file paths (relative to RepoDir)
}

// Manager handles workspace directory layout and lifecycle.
type Manager struct {
	baseDir string
	git     *gitclient.Client
	scanner sshenv.Keyscanner
}

// New creates a Manager rooted at the given base directory.
func New(baseDir string, git *gitclient.Client, scanner sshenv.Keyscanner) *Manager {
	return &Manager{baseDir: baseDir, git: git, scanner: scanner}
}

// ProjectRepoDir returns the path to a project's cached repo directory.
func (m *Manager) ProjectRepoDir(projectID pgtype.UUID) string {
	return filepath.Join(m.baseDir, "projects", fmtUUID(projectID), "repo")
}

// JobTmpDir returns the path to a job's temporary directory.
func (m *Manager) JobTmpDir(jobID pgtype.UUID) string {
	return filepath.Join(m.baseDir, "jobs", fmtUUID(jobID), "tmp")
}

// JobWorktreeDir returns the path to a job's git worktree directory.
func (m *Manager) JobWorktreeDir(jobID pgtype.UUID) string {
	return filepath.Join(m.baseDir, "jobs", fmtUUID(jobID), "worktree")
}

// lockProject acquires an exclusive file lock for the given project,
// serializing repo-mutating operations across concurrent workers.
// The lock attempt respects the provided context: if ctx is cancelled
// while waiting, lockProject returns an error wrapping ctx.Err().
// The returned function releases the lock and must be called when done.
func (m *Manager) lockProject(ctx context.Context, projectID string) (unlock func(), err error) {
	lockPath := filepath.Join(m.baseDir, "projects", projectID, ".lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("workspace: create lock dir for project %s: %w", projectID, err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("workspace: open lock file for project %s: %w", projectID, err)
	}
	backoff := 10 * time.Millisecond
	const maxBackoff = 1 * time.Second
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		} else if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("workspace: acquire lock for project %s: %w", projectID, err)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("workspace: acquire lock for project %s: %w", projectID, ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// Prepare sets up the workspace for a job:
//  1. Creates jobs/{jobID}/tmp directory for SSH keys
//  2. Sets up SSH environment (key file, known_hosts, GIT_SSH_COMMAND)
//  3. Ensures projects/{projectID}/repo exists and is up-to-date
//  4. Creates a per-job worktree from the cached repo
//  5. Records the HEAD commit SHA
//  6. Lists and filters source files
//
// On success, the caller receives a Result whose RepoDir points at the
// per-job worktree. The cleanup function must be called when the workflow
// is done (success or failure) to remove the job worktree and temp files.
// The project repo cache is intentionally preserved.
func (m *Manager) Prepare(ctx context.Context, execCtx *execution.Context) (result *Result, cleanup func(), err error) {
	if execCtx == nil {
		return nil, noop, fmt.Errorf("workspace: nil execution context")
	}
	if !execCtx.JobID.Valid {
		return nil, noop, fmt.Errorf("workspace: invalid JobID")
	}
	if !execCtx.ProjectID.Valid {
		return nil, noop, fmt.Errorf("workspace: invalid ProjectID")
	}

	log := logger.FromContext(ctx)
	jobID := fmtUUID(execCtx.JobID)
	projectID := fmtUUID(execCtx.ProjectID)
	safeURL := sanitizeURL(execCtx.RepoURL)

	tmpDir := m.JobTmpDir(execCtx.JobID)

	// Remove stale job root from a previous failed run so retries start clean.
	jobRoot := filepath.Dir(tmpDir)
	if err := os.RemoveAll(jobRoot); err != nil {
		return nil, noop, fmt.Errorf("workspace: remove stale job dir for job %s: %w", jobID, err)
	}

	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return nil, noop, fmt.Errorf("workspace: create tmp dir for job %s: %w", jobID, err)
	}

	repoDir := m.ProjectRepoDir(execCtx.ProjectID)
	worktreeDir := m.JobWorktreeDir(execCtx.JobID)

	cleanupFn := func() {
		// Best-effort: unregister git worktree under project lock.
		worktreeClean := true
		if isGitRepo(repoDir) {
			// Only attempt worktree removal if the directory exists
			// (it may not have been created yet if we're cleaning up early).
			if _, statErr := os.Stat(worktreeDir); statErr == nil {
				lockCtx, lockCancel := context.WithTimeout(context.Background(), 30*time.Second)
				unlock, lockErr := m.lockProject(lockCtx, projectID)
				lockCancel()
				if lockErr != nil {
					worktreeClean = false
					log.Warn("workspace: failed to acquire lock for worktree cleanup",
						slog.Any("error", lockErr))
				} else {
					wtCtx, wtCancel := context.WithTimeout(context.Background(), 10*time.Second)
					wtErr := m.git.WorktreeRemove(wtCtx, repoDir, worktreeDir)
					wtCancel()
					unlock()
					if wtErr != nil {
						worktreeClean = false
						log.Warn("workspace: worktree remove failed",
							slog.String("worktree", worktreeDir),
							slog.Any("error", wtErr))
					}
				}
			}
		}

		jobDir := filepath.Join(m.baseDir, "jobs", jobID)
		if worktreeClean {
			// Worktree was unregistered (or never created): safe to remove everything.
			if removeErr := os.RemoveAll(jobDir); removeErr != nil {
				log.Warn("workspace: cleanup failed",
					slog.String("path", jobDir),
					slog.Any("error", removeErr))
			}
		} else {
			// Preserve worktree directory so the orphaned .git/worktrees entry
			// can be resolved via `git worktree remove` or `git worktree prune`.
			if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
				log.Warn("workspace: cleanup tmpdir failed",
					slog.String("path", tmpDir),
					slog.Any("error", removeErr))
			}
		}
	}

	// Set up SSH environment only for SSH transports.
	var sshEnvVars []string
	if isSSHTransport(execCtx.RepoURL) {
		if len(execCtx.SSHPrivateKey) == 0 {
			cleanupFn()
			return nil, noop, fmt.Errorf("workspace: SSH private key is required for project %s", projectID)
		}
		if m.scanner == nil {
			cleanupFn()
			return nil, noop, fmt.Errorf("workspace: SSH keyscanner is required for project %s", projectID)
		}
		hostname, err := sshenv.ParseHostname(execCtx.RepoURL)
		if err != nil {
			cleanupFn()
			return nil, noop, fmt.Errorf("workspace: parse hostname for project %s: %w", projectID, err)
		}
		sshEnv, err := sshenv.Setup(ctx, tmpDir, execCtx.SSHPrivateKey, hostname, m.scanner)
		if err != nil {
			cleanupFn()
			return nil, noop, fmt.Errorf("workspace: ssh setup for job %s: %w", jobID, err)
		}
		sshEnvVars = sshEnv.EnvVars
	}

	// Acquire project lock and perform repo-mutating operations.
	// The inline closure ensures the lock is released via defer before
	// cleanupFn is called, avoiding deadlock (cleanupFn re-acquires the lock).
	repoErr := func() error {
		unlock, lockErr := m.lockProject(ctx, projectID)
		if lockErr != nil {
			return lockErr
		}
		defer unlock()

		if isGitRepo(repoDir) {
			// Verify remote URL matches before reuse.
			cachedURL, err := m.git.RemoteURL(ctx, repoDir)
			if err != nil {
				return fmt.Errorf("workspace: read cached remote URL for project %s: %w", projectID, err)
			}
			if normalizeURL(cachedURL) != normalizeURL(execCtx.RepoURL) {
				return fmt.Errorf("workspace: remote URL mismatch for project %s: cached=%q expected=%q",
					projectID, sanitizeURL(cachedURL), safeURL)
			}

			log.Info("workspace: fetching existing repo cache",
				slog.String("project_id", projectID),
				slog.String("repo_dir", repoDir))

			if err := m.git.Fetch(ctx, repoDir, sshEnvVars); err != nil {
				return fmt.Errorf("workspace: fetch for project %s: %w", projectID, err)
			}
		} else {
			// Atomic clone: clone into temp sibling dir, rename on success.
			if err := os.MkdirAll(filepath.Dir(repoDir), 0755); err != nil {
				return fmt.Errorf("workspace: create project dir for %s: %w", projectID, err)
			}

			tmpCloneDir := repoDir + ".cloning-" + jobID
			os.RemoveAll(tmpCloneDir) // remove leftover from previous failed attempt

			log.Info("workspace: cloning repo",
				slog.String("project_id", projectID),
				slog.String("repo_url", safeURL),
				slog.String("branch", execCtx.Branch))

			if err := m.git.Clone(ctx, execCtx.RepoURL, execCtx.Branch, tmpCloneDir, sshEnvVars); err != nil {
				os.RemoveAll(tmpCloneDir) // best-effort cleanup
				return fmt.Errorf("workspace: clone for project %s: %w", projectID, err)
			}

			if err := os.Rename(tmpCloneDir, repoDir); err != nil {
				os.RemoveAll(tmpCloneDir) // best-effort cleanup
				// Another concurrent clone may have won the race.
				// If repoDir is now a valid git repo, verify its remote URL before reuse.
				if isGitRepo(repoDir) {
					cachedURL, urlErr := m.git.RemoteURL(ctx, repoDir)
					if urlErr != nil {
						return fmt.Errorf("workspace: read remote URL after clone race for project %s: %w", projectID, urlErr)
					}
					if normalizeURL(cachedURL) != normalizeURL(execCtx.RepoURL) {
						return fmt.Errorf("workspace: remote URL mismatch after clone race for project %s: cached=%q expected=%q",
							projectID, sanitizeURL(cachedURL), safeURL)
					}
					log.Info("workspace: concurrent clone won race, fetching instead",
						slog.String("project_id", projectID))
					if fetchErr := m.git.Fetch(ctx, repoDir, sshEnvVars); fetchErr != nil {
						return fmt.Errorf("workspace: fetch after clone race for project %s: %w", projectID, fetchErr)
					}
				} else {
					return fmt.Errorf("workspace: finalize clone for project %s: %w", projectID, err)
				}
			}
		}

		// Create a per-job worktree for isolated checkout.
		ref := "origin/" + execCtx.Branch
		if err := m.git.WorktreeAdd(ctx, repoDir, worktreeDir, ref); err != nil {
			return fmt.Errorf("workspace: worktree add for job %s: %w", jobID, err)
		}

		return nil
	}()
	if repoErr != nil {
		cleanupFn()
		return nil, noop, repoErr
	}

	// Record HEAD SHA from the worktree.
	commitSHA, err := m.git.HeadSHA(ctx, worktreeDir)
	if err != nil {
		cleanupFn()
		return nil, noop, fmt.Errorf("workspace: head sha for project %s: %w", projectID, err)
	}

	// List and filter source files from the worktree.
	tracked, err := m.git.ListTrackedFiles(ctx, worktreeDir)
	if err != nil {
		cleanupFn()
		return nil, noop, fmt.Errorf("workspace: list files for project %s: %w", projectID, err)
	}

	sourceFiles, err := filterSourceFiles(worktreeDir, tracked)
	if err != nil {
		cleanupFn()
		return nil, noop, fmt.Errorf("workspace: filter files for project %s: %w", projectID, err)
	}

	log.Info("workspace: ready",
		slog.String("project_id", projectID),
		slog.String("commit", commitSHA),
		slog.Int("source_files", len(sourceFiles)))

	return &Result{
		RepoDir:     worktreeDir,
		CommitSHA:   commitSHA,
		SourceFiles: sourceFiles,
	}, cleanupFn, nil
}

// isGitRepo checks whether dir contains a .git directory.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// scpStylePattern matches scp-style git URLs: user@host:path
var scpStylePattern = regexp.MustCompile(`^([^@]+)@([^:]+):/?(.+)$`)

// normalizeURL canonicalizes a git remote URL for comparison.
// It converts scp-style URLs to ssh://, strips ".git" suffix,
// trailing slashes, userinfo, and default SSH port 22 so that
// equivalent URLs compare equal regardless of form or credentials.
func normalizeURL(rawURL string) string {
	rawURL = strings.TrimRight(rawURL, "/")
	rawURL = strings.TrimSuffix(rawURL, ".git")
	// Convert scp-style "user@host:path" → "ssh://user@host/path"
	// so all SSH variants can be parsed uniformly.
	if !strings.Contains(rawURL, "://") {
		if m := scpStylePattern.FindStringSubmatch(rawURL); m != nil {
			rawURL = fmt.Sprintf("ssh://%s@%s/%s", m[1], m[2], m[3])
		} else {
			return rawURL // local path or unknown format
		}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// Strip userinfo for non-SSH schemes (credentials).
	// SSH usernames identify remote accounts and must be preserved.
	if parsed.Scheme != "ssh" {
		parsed.User = nil
	}
	// Remove default SSH port so ssh://host:22/path matches ssh://host/path.
	if parsed.Scheme == "ssh" && parsed.Port() == "22" {
		parsed.Host = parsed.Hostname()
	}
	return parsed.String()
}

// sanitizeURL removes all userinfo from URLs for safe logging.
// On parse errors it returns a safe placeholder to avoid leaking credentials.
func sanitizeURL(rawURL string) string {
	if !strings.Contains(rawURL, "://") {
		return rawURL // SCP-style URLs don't embed passwords.
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<redacted-url>"
	}
	if parsed.User != nil {
		parsed.User = nil
	}
	return parsed.String()
}

// isSSHTransport returns true if the URL uses SSH transport.
// It matches "ssh://" scheme and SCP-style "user@host:path" patterns.
func isSSHTransport(rawURL string) bool {
	if strings.HasPrefix(rawURL, "ssh://") {
		return true
	}
	// SCP-style: must have "@" and ":" but not "://".
	if strings.Contains(rawURL, "://") {
		return false
	}
	atIdx := strings.Index(rawURL, "@")
	if atIdx < 0 {
		return false
	}
	rest := rawURL[atIdx+1:]
	return strings.Contains(rest, ":")
}

// filterSourceFiles returns tracked files under MaxFileSize.
// Symlinks and non-regular files are excluded.
// The result is sorted for stable ordering.
func filterSourceFiles(repoDir string, tracked []string) ([]string, error) {
	var result []string
	for _, f := range tracked {
		info, err := os.Lstat(filepath.Join(repoDir, f))
		if err != nil {
			if os.IsNotExist(err) {
				continue // File listed by git but not on disk.
			}
			return nil, fmt.Errorf("stat %s: %w", f, err)
		}
		if !info.Mode().IsRegular() {
			continue // Skip symlinks, directories, etc.
		}
		if info.Size() >= MaxFileSize {
			continue
		}
		result = append(result, f)
	}
	sort.Strings(result)
	return result, nil
}

// fmtUUID formats a pgtype.UUID as a hex string with dashes.
func fmtUUID(u pgtype.UUID) string {
	if !u.Valid {
		return "<nil>"
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// noop is a no-op cleanup function returned when cleanup is not needed.
func noop() {}
