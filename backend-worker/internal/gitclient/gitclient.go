// Package gitclient wraps git CLI commands for clone, fetch, checkout, and file listing.
package gitclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitRunner abstracts git command execution so tests can substitute a fake.
type GitRunner interface {
	// Run executes a git command in dir with optional extra env vars
	// and returns stdout.
	Run(ctx context.Context, dir string, env []string, args ...string) ([]byte, error)
}

// ExecRunner is the production GitRunner using os/exec.
type ExecRunner struct{}

// Run executes "git <args...>" in dir with the given env vars appended
// to the current process environment. On success it returns stdout only.
// On failure stderr is captured separately via exec.ExitError.
func (ExecRunner) Run(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.Output()
	if err != nil {
		re := &RunError{Args: args, Output: out, Err: err}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			re.Stderr = exitErr.Stderr
		}
		return nil, re
	}
	return out, nil
}

// RunError wraps a failed git command with its output for debugging.
type RunError struct {
	Args   []string
	Output []byte // stdout captured before the error
	Stderr []byte // stderr from exec.ExitError (if available)
	Err    error
}

// credentialPattern matches userinfo in scheme-based URLs for redaction.
// It handles any scheme (http, https, ssh, ftp, etc.) per RFC 3986.
var credentialPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)([^@/]+)@`)

// redactCredentials replaces userinfo in scheme-based URLs with <redacted>.
func redactCredentials(s string) string {
	return credentialPattern.ReplaceAllString(s, "${1}<redacted>@")
}

func (e *RunError) Error() string {
	// Prefer stderr for diagnostics; fall back to stdout.
	snippet := e.Stderr
	if len(snippet) == 0 {
		snippet = e.Output
	}
	// Redact credentials before truncating so partial URLs at the
	// boundary cannot leak secrets past the regex.
	args := redactCredentials(strings.Join(e.Args, " "))
	output := redactCredentials(string(bytes.TrimSpace(snippet)))
	if len(output) > 500 {
		output = output[:500]
	}
	return fmt.Sprintf("git %s: %v: %s", args, e.Err, output)
}

func (e *RunError) Unwrap() error { return e.Err }

// Client executes git operations on the filesystem.
type Client struct {
	runner GitRunner
}

// New creates a Client with the given GitRunner.
func New(runner GitRunner) *Client {
	return &Client{runner: runner}
}

// Clone performs a full git clone into repoDir. The directory must not already
// contain a git repository. env is appended to the command environment (for
// GIT_SSH_COMMAND).
func (c *Client) Clone(ctx context.Context, repoURL, branch, repoDir string, env []string) error {
	_, err := c.runner.Run(ctx, "", env,
		"clone", "--branch", branch, "--", repoURL, repoDir)
	if err != nil {
		return fmt.Errorf("gitclient: clone: %w", err)
	}
	return nil
}

// Fetch runs git fetch origin --prune inside an existing repo at repoDir.
// env is appended to the command environment.
func (c *Client) Fetch(ctx context.Context, repoDir string, env []string) error {
	_, err := c.runner.Run(ctx, repoDir, env, "fetch", "origin", "--prune")
	if err != nil {
		return fmt.Errorf("gitclient: fetch: %w", err)
	}
	return nil
}

// Checkout force-creates or resets the local branch to match origin/{branch}.
// Uses "checkout -B" to do both in a single git invocation.
func (c *Client) Checkout(ctx context.Context, repoDir, branch string) error {
	_, err := c.runner.Run(ctx, repoDir, nil, "checkout", "-B", branch, "origin/"+branch)
	if err != nil {
		return fmt.Errorf("gitclient: checkout: %w", err)
	}
	return nil
}

// HeadSHA returns the full SHA of HEAD in the repo at repoDir.
func (c *Client) HeadSHA(ctx context.Context, repoDir string) (string, error) {
	out, err := c.runner.Run(ctx, repoDir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("gitclient: head sha: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RemoteURL returns the configured origin URL for the repo at repoDir.
func (c *Client) RemoteURL(ctx context.Context, repoDir string) (string, error) {
	out, err := c.runner.Run(ctx, repoDir, nil, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("gitclient: remote url: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ListTrackedFiles runs git ls-files -z in the repo and returns the file paths.
// NUL-delimited output avoids issues with filenames containing newlines.
func (c *Client) ListTrackedFiles(ctx context.Context, repoDir string) ([]string, error) {
	out, err := c.runner.Run(ctx, repoDir, nil, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("gitclient: ls-files: %w", err)
	}
	raw := string(out)
	if raw == "" {
		return nil, nil
	}
	// -z flag uses NUL as delimiter; split and drop trailing empty element.
	files := strings.Split(raw, "\x00")
	if len(files) > 0 && files[len(files)-1] == "" {
		files = files[:len(files)-1]
	}
	if len(files) == 0 {
		return nil, nil
	}
	return files, nil
}

// WorktreeAdd creates a new git worktree at worktreeDir checked out at the
// given ref (typically "origin/<branch>"). The worktree uses a detached HEAD.
func (c *Client) WorktreeAdd(ctx context.Context, repoDir, worktreeDir, ref string) error {
	_, err := c.runner.Run(ctx, repoDir, nil, "worktree", "add", "--detach", worktreeDir, ref)
	if err != nil {
		return fmt.Errorf("gitclient: worktree add: %w", err)
	}
	return nil
}

// WorktreeRemove removes a git worktree and its administrative files.
// The --force flag allows removal even if the worktree contains modifications.
func (c *Client) WorktreeRemove(ctx context.Context, repoDir, worktreeDir string) error {
	_, err := c.runner.Run(ctx, repoDir, nil, "worktree", "remove", "--force", worktreeDir)
	if err != nil {
		return fmt.Errorf("gitclient: worktree remove: %w", err)
	}
	return nil
}

// DiffEntry represents a single file change from git diff --name-status.
type DiffEntry struct {
	Status string // "A" (added), "M" (modified), "D" (deleted)
	Path   string
}

// DiffNameStatus computes the changed file set between baseCommit and
// targetCommit. It returns one DiffEntry per changed file.
// Uses --no-renames so renames appear as a delete + add pair.
func (c *Client) DiffNameStatus(ctx context.Context, repoDir, baseCommit, targetCommit string) ([]DiffEntry, error) {
	out, err := c.runner.Run(ctx, repoDir, nil,
		"diff", "--name-status", "--no-renames", "-z", baseCommit, targetCommit)
	if err != nil {
		return nil, fmt.Errorf("gitclient: diff: %w", err)
	}
	return parseDiffNameStatusZ(out)
}

// parseDiffNameStatusZ parses NUL-delimited git diff --name-status -z output.
// Format: STATUS\0PATH\0STATUS\0PATH\0...
func parseDiffNameStatusZ(data []byte) ([]DiffEntry, error) {
	raw := string(data)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "\x00")
	// Drop trailing empty element from final NUL.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("gitclient: diff output has odd number of fields (%d)", len(parts))
	}
	entries := make([]DiffEntry, 0, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		status := parts[i]
		switch status {
		case "A", "M", "D":
			// Known statuses — use as-is.
		default:
			// Git can emit statuses like "T" (type change), "U" (unmerged),
			// "X" (unknown), "B" (broken pairing). The downstream handler
			// only acts on A/M/D, so normalise unexpected statuses to "M"
			// to ensure the file is re-parsed rather than silently skipped.
			slog.Warn("gitclient: unknown diff status, normalising to M",
				slog.String("status", status),
				slog.String("path", parts[i+1]),
			)
			status = "M"
		}
		entries = append(entries, DiffEntry{
			Status: status,
			Path:   parts[i+1],
		})
	}
	return entries, nil
}

// CommitLog holds metadata for a single git commit extracted from git log.
type CommitLog struct {
	Hash           string
	ParentHashes   []string
	AuthorName     string
	AuthorEmail    string
	AuthorDate     time.Time
	CommitterName  string
	CommitterEmail string
	CommitterDate  time.Time
	Message        string
}

// FileDiffEntry holds per-file change information from git log --name-status / --numstat / -p.
type FileDiffEntry struct {
	Status    string // "A", "M", "D"
	Path      string
	Additions int
	Deletions int
	Patch     string // unified diff content; empty for binary/omitted files
}

const (
	// maxPatchBytes is the maximum size of a single file's patch content.
	// Patches exceeding this are discarded to avoid excessive DB storage.
	maxPatchBytes = 5 * 1024 * 1024 // 5 MB
)

// logArgs builds the common git log arguments shared by LogCommits and DiffStatLog.
// sinceCommit controls history range (empty = full with --first-parent; non-empty = sinceCommit..HEAD).
// maxCount maps to --max-count (0 = no limit). extra args are appended after the common flags.
func logArgs(sinceCommit string, maxCount int, extra ...string) []string {
	args := []string{"log"}
	if sinceCommit == "" {
		args = append(args, "--first-parent")
	}
	if maxCount > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", maxCount))
	}
	args = append(args, extra...)
	if sinceCommit != "" {
		args = append(args, sinceCommit+"..HEAD")
	}
	return args
}

// LogCommits extracts commit metadata from the repo via git log.
//
// When sinceCommit is empty the full history is returned (following --first-parent
// to stay on the mainline). When sinceCommit is set only commits in the range
// sinceCommit..HEAD are returned. maxCount limits the number of commits (0 = no limit).
//
// Commits are returned newest-first (default git log order).
func (c *Client) LogCommits(ctx context.Context, repoDir, sinceCommit string, maxCount int) ([]CommitLog, error) {
	// Record separator \x1E between commits, unit separator \x1F between fields.
	// Fields: hash, parent hashes, author name, author email, author date (ISO),
	// committer name, committer email, committer date (ISO), raw body.
	format := "--format=%H\x1F%P\x1F%an\x1F%ae\x1F%aI\x1F%cn\x1F%ce\x1F%cI\x1F%B\x1E"
	args := logArgs(sinceCommit, maxCount, format)

	out, err := c.runner.Run(ctx, repoDir, nil, args...)
	if err != nil {
		return nil, fmt.Errorf("gitclient: log commits: %w", err)
	}
	return parseLogCommits(out)
}

// parseLogCommits parses the output of git log with record/unit separators.
func parseLogCommits(data []byte) ([]CommitLog, error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return nil, nil
	}

	records := strings.Split(raw, "\x1E")
	var commits []CommitLog
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		fields := strings.SplitN(rec, "\x1F", 9)
		if len(fields) < 9 {
			return nil, fmt.Errorf("gitclient: log parse: expected 9 fields, got %d", len(fields))
		}

		authorDate, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[4]))
		if err != nil {
			return nil, fmt.Errorf("gitclient: log parse author date %q: %w", fields[4], err)
		}
		committerDate, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[7]))
		if err != nil {
			return nil, fmt.Errorf("gitclient: log parse committer date %q: %w", fields[7], err)
		}

		var parents []string
		if p := strings.TrimSpace(fields[1]); p != "" {
			parents = strings.Fields(p)
		}

		commits = append(commits, CommitLog{
			Hash:           strings.TrimSpace(fields[0]),
			ParentHashes:   parents,
			AuthorName:     strings.TrimSpace(fields[2]),
			AuthorEmail:    strings.TrimSpace(fields[3]),
			AuthorDate:     authorDate,
			CommitterName:  strings.TrimSpace(fields[5]),
			CommitterEmail: strings.TrimSpace(fields[6]),
			CommitterDate:  committerDate,
			Message:        strings.TrimSpace(fields[8]),
		})
	}
	return commits, nil
}

// DiffStatLog extracts per-commit file-level change types and line statistics.
//
// It runs two git commands:
//  1. git log --name-status  → change types (A/M/D per file)
//  2. git log --numstat      → additions/deletions per file
//
// The outputs are merged by commit hash and file path into a map keyed by commit hash.
// Uses --no-renames so renames appear as delete+add, consistent with DiffNameStatus.
func (c *Client) DiffStatLog(ctx context.Context, repoDir, sinceCommit string, maxCount int) (map[string][]FileDiffEntry, error) {
	commonFlags := []string{"--no-renames", "--diff-filter=ACDMT"}

	// Command 1: name-status with -z for NUL-delimited output (safe for special filenames).
	nsArgs := logArgs(sinceCommit, maxCount, append([]string{"--name-status", "-z", "--format=COMMIT:%H"}, commonFlags...)...)
	nsOut, err := c.runner.Run(ctx, repoDir, nil, nsArgs...)
	if err != nil {
		return nil, fmt.Errorf("gitclient: diff stat log (name-status): %w", err)
	}

	// Command 2: numstat with -z for NUL-delimited output (safe for special filenames).
	numArgs := logArgs(sinceCommit, maxCount, append([]string{"--numstat", "-z", "--format=COMMIT:%H"}, commonFlags...)...)
	numOut, err := c.runner.Run(ctx, repoDir, nil, numArgs...)
	if err != nil {
		return nil, fmt.Errorf("gitclient: diff stat log (numstat): %w", err)
	}

	entries := parseNameStatusLogZ(nsOut)
	mergeNumstatLogZ(numOut, entries)

	// Command 3: patch content for unified diffs (newline-delimited, not -z).
	patchArgs := logArgs(sinceCommit, maxCount, append([]string{"-p", "--format=COMMIT:%H"}, commonFlags...)...)
	patchOut, err := c.runner.Run(ctx, repoDir, nil, patchArgs...)
	if err != nil {
		return nil, fmt.Errorf("gitclient: diff stat log (patch): %w", err)
	}
	mergePatchLog(patchOut, entries)

	// Flatten to result map.
	result := make(map[string][]FileDiffEntry, len(entries))
	for hash, fileMap := range entries {
		diffs := make([]FileDiffEntry, 0, len(fileMap))
		for _, entry := range fileMap {
			diffs = append(diffs, *entry)
		}
		result[hash] = diffs
	}
	return result, nil
}

// parseNameStatusLogZ parses NUL-delimited git log --name-status -z --format=COMMIT:%H output.
// With -z, status and path are in separate NUL-delimited fields (alternating),
// which avoids quoting/escaping of filenames with unusual characters.
// Returns a nested map: commitHash → filePath → *FileDiffEntry.
func parseNameStatusLogZ(data []byte) map[string]map[string]*FileDiffEntry {
	result := make(map[string]map[string]*FileDiffEntry)
	var currentHash string
	var pendingStatus string

	for _, field := range strings.Split(string(data), "\x00") {
		field = strings.TrimLeft(field, "\n\r")
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, "COMMIT:") {
			currentHash = strings.TrimPrefix(field, "COMMIT:")
			if _, ok := result[currentHash]; !ok {
				result[currentHash] = make(map[string]*FileDiffEntry)
			}
			pendingStatus = ""
			continue
		}
		if currentHash == "" {
			continue
		}
		if pendingStatus == "" {
			// This field is a status letter (A, M, D, etc.).
			pendingStatus = field
		} else {
			// This field is a file path.
			result[currentHash][field] = &FileDiffEntry{
				Status: pendingStatus,
				Path:   field,
			}
			pendingStatus = ""
		}
	}
	return result
}

// mergeNumstatLogZ parses NUL-delimited git log --numstat -z --format=COMMIT:%H output
// and merges additions/deletions into existing entries. With -z, each numstat
// record (adds\tdels\tpath) is NUL-terminated, keeping file paths verbatim.
// If a file appears in numstat but not in the name-status map, it is added with status "M".
func mergeNumstatLogZ(data []byte, entries map[string]map[string]*FileDiffEntry) {
	var currentHash string

	for _, field := range strings.Split(string(data), "\x00") {
		field = strings.TrimLeft(field, "\n\r")
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, "COMMIT:") {
			currentHash = strings.TrimPrefix(field, "COMMIT:")
			if _, ok := entries[currentHash]; !ok {
				entries[currentHash] = make(map[string]*FileDiffEntry)
			}
			continue
		}
		if currentHash == "" {
			continue
		}
		parts := strings.SplitN(field, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		// Binary files show "-" for additions/deletions.
		adds, _ := strconv.Atoi(parts[0])
		dels, _ := strconv.Atoi(parts[1])
		path := parts[2]

		if entry, ok := entries[currentHash][path]; ok {
			entry.Additions = adds
			entry.Deletions = dels
		} else {
			entries[currentHash][path] = &FileDiffEntry{
				Status:    "M",
				Path:      path,
				Additions: adds,
				Deletions: dels,
			}
		}
	}
}

// mergePatchLog parses newline-delimited git log -p --format=COMMIT:%H output
// and merges unified diff patches into existing entries.
// Binary files and patches exceeding maxPatchBytes are skipped.
func mergePatchLog(data []byte, entries map[string]map[string]*FileDiffEntry) {
	lines := strings.Split(string(data), "\n")

	var currentHash string
	var currentPath string
	var patchLines []string
	var patchSize int
	var oversized bool

	flush := func() {
		if currentHash == "" || currentPath == "" {
			return
		}
		if oversized || patchSize == 0 {
			return
		}
		patch := strings.Join(patchLines, "\n")
		fileMap, ok := entries[currentHash]
		if !ok {
			return
		}
		if entry, ok := fileMap[currentPath]; ok {
			entry.Patch = patch
		}
	}

	for _, line := range lines {
		// Detect commit boundary.
		if strings.HasPrefix(line, "COMMIT:") {
			flush()
			currentHash = strings.TrimPrefix(line, "COMMIT:")
			currentPath = ""
			patchLines = nil
			patchSize = 0
			oversized = false
			if _, ok := entries[currentHash]; !ok {
				entries[currentHash] = make(map[string]*FileDiffEntry)
			}
			continue
		}
		if currentHash == "" {
			continue
		}

		// Detect file boundary.
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			currentPath = parseDiffGitPath(line)
			patchLines = nil
			patchSize = 0
			oversized = false
			continue
		}
		if currentPath == "" {
			continue
		}

		// Skip binary file markers.
		if strings.HasPrefix(line, "Binary files ") {
			currentPath = ""
			patchLines = nil
			patchSize = 0
			continue
		}

		// Skip diff header lines (index, old mode, new mode, ---/+++ handled below).
		// We include --- and +++ lines plus hunk headers and content in the patch.
		if strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "old mode ") ||
			strings.HasPrefix(line, "new mode ") ||
			strings.HasPrefix(line, "new file mode ") ||
			strings.HasPrefix(line, "deleted file mode ") ||
			strings.HasPrefix(line, "similarity index ") ||
			strings.HasPrefix(line, "copy from ") ||
			strings.HasPrefix(line, "copy to ") ||
			strings.HasPrefix(line, "rename from ") ||
			strings.HasPrefix(line, "rename to ") {
			continue
		}

		// Accumulate patch content (---, +++, @@, context, +, - lines).
		if oversized {
			continue
		}
		patchSize += len(line) + 1 // +1 for newline
		if patchSize > maxPatchBytes {
			oversized = true
			patchLines = nil
			slog.Debug("gitclient: patch too large, discarding",
				slog.String("commit", currentHash),
				slog.String("path", currentPath),
				slog.Int("bytes", patchSize))
			continue
		}
		patchLines = append(patchLines, line)
	}
	flush()
}

// parseDiffGitPath extracts the file path from a "diff --git a/<path> b/<path>" line.
// With --no-renames, both paths are always identical, so the line format is
// deterministic. We exploit the identical-path invariant to split by halving
// the remainder, which is safe even when the path itself contains " b/" segments.
//
// Git quotes paths containing non-ASCII or control characters using C-style quoting:
//
//	diff --git "a/na\303\257ve.txt" "b/na\303\257ve.txt"
//
// The function detects quoted paths, extracts via halving, and unescapes
// C-style sequences so the returned path matches the raw UTF-8 path in the
// entries map (populated from NUL-delimited name-status output).
func parseDiffGitPath(line string) string {
	const prefix = "diff --git "
	rest := strings.TrimPrefix(line, prefix)

	if strings.HasPrefix(rest, "\"a/") {
		// Quoted format: "a/<escaped>" "b/<escaped>"
		// Total: 3 + escapedLen + 4 + escapedLen + 1 = 8 + 2*escapedLen.
		if len(rest) < 8 {
			return ""
		}
		escapedLen := (len(rest) - 8) / 2
		if escapedLen <= 0 {
			return ""
		}
		return unquoteGitPath(rest[3 : 3+escapedLen])
	}

	// Unquoted format: a/<path> b/<path>
	// Total: 2 + pathLen + 1 + 2 + pathLen = 5 + 2*pathLen.
	if len(rest) < 5 || !strings.HasPrefix(rest, "a/") {
		return ""
	}
	pathLen := (len(rest) - 5) / 2
	if pathLen <= 0 {
		return ""
	}
	return rest[2 : 2+pathLen]
}

// unquoteGitPath decodes Git's C-style escape sequences in a path string.
// Handles octal bytes (\nnn), standard escapes (\n, \t, \\, \"), and
// passes through unknown sequences as-is.
func unquoteGitPath(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		next := s[i+1]
		switch next {
		case 'n':
			b.WriteByte('\n')
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case 'r':
			b.WriteByte('\r')
			i++
		case 'a':
			b.WriteByte('\a')
			i++
		case 'b':
			b.WriteByte('\b')
			i++
		case 'f':
			b.WriteByte('\f')
			i++
		case 'v':
			b.WriteByte('\v')
			i++
		case '\\':
			b.WriteByte('\\')
			i++
		case '"':
			b.WriteByte('"')
			i++
		default:
			// Octal: \nnn (3 digits, 0-7).
			if next >= '0' && next <= '7' && i+3 < len(s) &&
				s[i+2] >= '0' && s[i+2] <= '7' &&
				s[i+3] >= '0' && s[i+3] <= '7' {
				val := (next-'0')*64 + (s[i+2]-'0')*8 + (s[i+3] - '0')
				b.WriteByte(val)
				i += 3
			} else {
				// Unknown escape — pass through.
				b.WriteByte(s[i])
			}
		}
	}
	return b.String()
}
