package gitclient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fake runner ---

type runCall struct {
	Dir  string
	Env  []string
	Args []string
}

type runResult struct {
	Output []byte
	Err    error
}

type fakeRunner struct {
	mu      sync.Mutex
	calls   []runCall
	results []runResult
	idx     int
}

func (f *fakeRunner) Run(_ context.Context, dir string, env []string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, runCall{Dir: dir, Env: env, Args: args})
	if f.idx >= len(f.results) {
		return nil, errors.New("fakeRunner: no more results")
	}
	r := f.results[f.idx]
	f.idx++
	return r.Output, r.Err
}

// --- tests ---

func TestClone_Success(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("Cloning into 'repo'...\n")},
	}}
	c := New(f)

	err := c.Clone(context.Background(), "git@github.com:org/repo.git", "main", "/tmp/repo", []string{"GIT_SSH_COMMAND=ssh -i key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(f.calls))
	}
	call := f.calls[0]
	if call.Dir != "" {
		t.Errorf("clone dir should be empty, got %q", call.Dir)
	}
	wantArgs := []string{"clone", "--branch", "main", "--", "git@github.com:org/repo.git", "/tmp/repo"}
	if strings.Join(call.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("args = %v, want %v", call.Args, wantArgs)
	}
	if len(call.Env) != 1 || call.Env[0] != "GIT_SSH_COMMAND=ssh -i key" {
		t.Errorf("env = %v, want [GIT_SSH_COMMAND=ssh -i key]", call.Env)
	}
}

func TestClone_Failure(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Err: errors.New("exit status 128")},
	}}
	c := New(f)

	err := c.Clone(context.Background(), "git@github.com:org/repo.git", "main", "/tmp/repo", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitclient: clone") {
		t.Errorf("error should contain 'gitclient: clone', got %q", err.Error())
	}
}

func TestFetch_Success(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
	}}
	c := New(f)

	err := c.Fetch(context.Background(), "/cache/repo", []string{"GIT_SSH_COMMAND=ssh"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := f.calls[0]
	if call.Dir != "/cache/repo" {
		t.Errorf("dir = %q, want /cache/repo", call.Dir)
	}
	wantArgs := []string{"fetch", "origin", "--prune"}
	if strings.Join(call.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("args = %v, want %v", call.Args, wantArgs)
	}
}

func TestCheckout_Success(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("Switched to and reset branch 'main'\n")},
	}}
	c := New(f)

	err := c.Checkout(context.Background(), "/cache/repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(f.calls))
	}
	wantArgs := "checkout -B main origin/main"
	if strings.Join(f.calls[0].Args, " ") != wantArgs {
		t.Errorf("args = %v, want %q", f.calls[0].Args, wantArgs)
	}
}

func TestCheckout_Failure(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Err: errors.New("error: pathspec 'nope' did not match")},
	}}
	c := New(f)

	err := c.Checkout(context.Background(), "/cache/repo", "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitclient: checkout") {
		t.Errorf("error should contain 'gitclient: checkout', got %q", err.Error())
	}
}

func TestHeadSHA_TrimsOutput(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("abc123def456\n")},
	}}
	c := New(f)

	sha, err := c.HeadSHA(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != "abc123def456" {
		t.Errorf("sha = %q, want %q", sha, "abc123def456")
	}
}

func TestRemoteURL_TrimsOutput(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("git@github.com:org/repo.git\n")},
	}}
	c := New(f)

	url, err := c.RemoteURL(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "git@github.com:org/repo.git" {
		t.Errorf("url = %q, want %q", url, "git@github.com:org/repo.git")
	}
}

func TestListTrackedFiles_SplitsNUL(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("src/a.ts\x00src/b.js\x00README.md\x00")},
	}}
	c := New(f)

	files, err := c.ListTrackedFiles(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"src/a.ts", "src/b.js", "README.md"}
	if len(files) != len(want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("files[%d] = %q, want %q", i, files[i], want[i])
		}
	}
	// Verify -z flag was passed.
	if strings.Join(f.calls[0].Args, " ") != "ls-files -z" {
		t.Errorf("args = %v, want [ls-files -z]", f.calls[0].Args)
	}
}

func TestListTrackedFiles_EmptyRepo(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
	}}
	c := New(f)

	files, err := c.ListTrackedFiles(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil for empty repo, got %v", files)
	}
}

func TestRunError_Format(t *testing.T) {
	e := &RunError{
		Args:   []string{"clone", "url", "dir"},
		Stderr: []byte("fatal: repository not found\n"),
		Err:    errors.New("exit status 128"),
	}
	msg := e.Error()
	if !strings.Contains(msg, "git clone url dir") {
		t.Errorf("error should contain command, got %q", msg)
	}
	if !strings.Contains(msg, "exit status 128") {
		t.Errorf("error should contain exit status, got %q", msg)
	}
	if !strings.Contains(msg, "fatal: repository not found") {
		t.Errorf("error should contain stderr, got %q", msg)
	}
}

func TestRunError_FallsBackToStdout(t *testing.T) {
	e := &RunError{
		Args:   []string{"log"},
		Output: []byte("partial stdout before error\n"),
		Err:    errors.New("signal: killed"),
	}
	msg := e.Error()
	if !strings.Contains(msg, "partial stdout before error") {
		t.Errorf("error should fall back to stdout when stderr is empty, got %q", msg)
	}
}

func TestRunError_Unwrap(t *testing.T) {
	inner := errors.New("exit status 128")
	e := &RunError{Err: inner}
	if !errors.Is(e, inner) {
		t.Error("Unwrap should return inner error")
	}
}

func TestRunError_TruncatesLongOutput(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}
	e := &RunError{Args: []string{"fetch"}, Output: long, Err: errors.New("fail")}
	if len(e.Error()) > 600 {
		t.Error("error message should truncate output to 500 bytes")
	}
}

func TestWorktreeAdd_Success(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("Preparing worktree\n")},
	}}
	c := New(f)

	err := c.WorktreeAdd(context.Background(), "/cache/repo", "/jobs/1/worktree", "origin/main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := f.calls[0]
	if call.Dir != "/cache/repo" {
		t.Errorf("dir = %q, want /cache/repo", call.Dir)
	}
	wantArgs := []string{"worktree", "add", "--detach", "/jobs/1/worktree", "origin/main"}
	if strings.Join(call.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("args = %v, want %v", call.Args, wantArgs)
	}
}

func TestWorktreeAdd_Failure(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Err: errors.New("fatal: invalid reference")},
	}}
	c := New(f)

	err := c.WorktreeAdd(context.Background(), "/cache/repo", "/jobs/1/worktree", "origin/nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitclient: worktree add") {
		t.Errorf("error should contain 'gitclient: worktree add', got %q", err.Error())
	}
}

func TestWorktreeRemove_Success(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
	}}
	c := New(f)

	err := c.WorktreeRemove(context.Background(), "/cache/repo", "/jobs/1/worktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	call := f.calls[0]
	wantArgs := []string{"worktree", "remove", "--force", "/jobs/1/worktree"}
	if strings.Join(call.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("args = %v, want %v", call.Args, wantArgs)
	}
}

func TestWorktreeRemove_Failure(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Err: errors.New("fatal: not a valid worktree")},
	}}
	c := New(f)

	err := c.WorktreeRemove(context.Background(), "/cache/repo", "/jobs/1/worktree")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitclient: worktree remove") {
		t.Errorf("error should contain 'gitclient: worktree remove', got %q", err.Error())
	}
}

// --- DiffNameStatus tests ---

func TestDiffNameStatus_ParsesEntries(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("M\x00src/a.ts\x00A\x00src/b.ts\x00D\x00src/c.ts\x00")},
	}}
	c := New(f)

	entries, err := c.DiffNameStatus(context.Background(), "/repo", "abc123", "def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	want := []struct{ status, path string }{
		{"M", "src/a.ts"},
		{"A", "src/b.ts"},
		{"D", "src/c.ts"},
	}
	for i, w := range want {
		if entries[i].Status != w.status || entries[i].Path != w.path {
			t.Errorf("entries[%d] = {%q, %q}, want {%q, %q}",
				i, entries[i].Status, entries[i].Path, w.status, w.path)
		}
	}

	call := f.calls[0]
	wantArgs := []string{"diff", "--name-status", "--no-renames", "-z", "abc123", "def456"}
	if strings.Join(call.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("args = %v, want %v", call.Args, wantArgs)
	}
	if call.Dir != "/repo" {
		t.Errorf("dir = %q, want /repo", call.Dir)
	}
}

func TestDiffNameStatus_EmptyDiff(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
	}}
	c := New(f)

	entries, err := c.DiffNameStatus(context.Background(), "/repo", "abc", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty diff, got %v", entries)
	}
}

func TestDiffNameStatus_Failure(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Err: errors.New("exit status 128")},
	}}
	c := New(f)

	_, err := c.DiffNameStatus(context.Background(), "/repo", "bad", "commit")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitclient: diff") {
		t.Errorf("error should contain 'gitclient: diff', got %q", err.Error())
	}
}

func TestParseDiffNameStatusZ_UnknownStatusNormalisedToM(t *testing.T) {
	// Git statuses like "T" (type change) or "U" (unmerged) are not handled
	// by the downstream incremental handler. parseDiffNameStatusZ should
	// normalise them to "M" so the file is re-parsed.
	data := []byte("T\x00src/link.ts\x00A\x00src/new.ts\x00U\x00src/conflict.ts\x00")
	entries, err := parseDiffNameStatusZ(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	want := []struct{ status, path string }{
		{"M", "src/link.ts"},     // T → M
		{"A", "src/new.ts"},      // A stays A
		{"M", "src/conflict.ts"}, // U → M
	}
	for i, w := range want {
		if entries[i].Status != w.status || entries[i].Path != w.path {
			t.Errorf("entries[%d] = {%q, %q}, want {%q, %q}",
				i, entries[i].Status, entries[i].Path, w.status, w.path)
		}
	}
}

func TestParseDiffNameStatusZ_OddFields(t *testing.T) {
	_, err := parseDiffNameStatusZ([]byte("M\x00src/a.ts\x00A"))
	if err == nil {
		t.Fatal("expected error for odd number of fields")
	}
	if !strings.Contains(err.Error(), "odd number of fields") {
		t.Errorf("error = %q", err)
	}
}

// --- credential redaction tests ---

func TestRedactCredentials(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"https with user:pass", "https://user:ghp_token123@github.com/org/repo", "https://<redacted>@github.com/org/repo"},
		{"https with token only", "https://x-access-token:ghp_abc@github.com/org/repo", "https://<redacted>@github.com/org/repo"},
		{"http with creds", "http://user:pass@example.com/repo", "http://<redacted>@example.com/repo"},
		{"ssh with user:token", "ssh://user:secret@github.com/org/repo", "ssh://<redacted>@github.com/org/repo"},
		{"ssh with user only", "ssh://git@github.com/org/repo", "ssh://<redacted>@github.com/org/repo"},
		{"ftp with creds", "ftp://admin:pass@files.example.com/data", "ftp://<redacted>@files.example.com/data"},
		{"scp-style unchanged", "git@github.com:org/repo.git", "git@github.com:org/repo.git"},
		{"plain https unchanged", "https://github.com/org/repo", "https://github.com/org/repo"},
		{"empty", "", ""},
		{"multiple URLs", "clone https://a:b@host/r ssh://x:y@host/r2", "clone https://<redacted>@host/r ssh://<redacted>@host/r2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactCredentials(tt.input)
			if got != tt.want {
				t.Errorf("redactCredentials(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRunError_RedactsCredentials_TruncationBoundary(t *testing.T) {
	// Build stderr where a credential-bearing URL straddles the 500-byte
	// boundary of the raw output. Without redact-before-truncate, the regex
	// would fail on the partial URL and leak the credential.
	//
	// Raw:   460 padding + 8 " fatal: " + 50 URL = 518 bytes (>500, old code truncates)
	// After: 460 padding + 8 " fatal: " + 42 redacted URL = 510 chars (truncated to 500,
	//        but <redacted> at offset 476 is fully within the 500-char window)
	credURL := "https://user:secret-token@github.com/org/repo.git"
	padding := make([]byte, 460)
	for i := range padding {
		padding[i] = 'x'
	}
	stderr := append(padding, []byte(" fatal: "+credURL)...)

	e := &RunError{
		Args:   []string{"fetch"},
		Stderr: stderr,
		Err:    errors.New("exit status 128"),
	}
	msg := e.Error()
	if strings.Contains(msg, "secret-token") {
		t.Errorf("error message should not contain credentials after truncation, got %q", msg)
	}
	if !strings.Contains(msg, "<redacted>") {
		t.Errorf("error message should contain <redacted>, got %q", msg)
	}
}

func TestRunError_RedactsCredentials(t *testing.T) {
	e := &RunError{
		Args:   []string{"clone", "https://user:secret-token@github.com/org/repo.git", "/tmp/repo"},
		Output: []byte("fatal: could not read from https://user:secret-token@github.com/org/repo.git"),
		Err:    errors.New("exit status 128"),
	}
	msg := e.Error()
	if strings.Contains(msg, "secret-token") {
		t.Errorf("error message should not contain credentials, got %q", msg)
	}
	if !strings.Contains(msg, "<redacted>") {
		t.Errorf("error message should contain <redacted>, got %q", msg)
	}
}

// --- LogCommits tests ---

// buildLogOutput builds a fake git log output with \x1E/\x1F separators.
func buildLogOutput(commits ...struct {
	hash, parents, an, ae, ad, cn, ce, cd, msg string
}) []byte {
	var sb strings.Builder
	for _, c := range commits {
		sb.WriteString(c.hash)
		sb.WriteByte('\x1F')
		sb.WriteString(c.parents)
		sb.WriteByte('\x1F')
		sb.WriteString(c.an)
		sb.WriteByte('\x1F')
		sb.WriteString(c.ae)
		sb.WriteByte('\x1F')
		sb.WriteString(c.ad)
		sb.WriteByte('\x1F')
		sb.WriteString(c.cn)
		sb.WriteByte('\x1F')
		sb.WriteString(c.ce)
		sb.WriteByte('\x1F')
		sb.WriteString(c.cd)
		sb.WriteByte('\x1F')
		sb.WriteString(c.msg)
		sb.WriteByte('\x1E')
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

func TestLogCommits_SingleCommit(t *testing.T) {
	output := buildLogOutput(struct {
		hash, parents, an, ae, ad, cn, ce, cd, msg string
	}{
		"abc123", "def456",
		"Alice", "alice@example.com", "2025-01-15T10:30:00+00:00",
		"Alice", "alice@example.com", "2025-01-15T10:31:00+00:00",
		"Fix login bug",
	})

	f := &fakeRunner{results: []runResult{{Output: output}}}
	c := New(f)

	commits, err := c.LogCommits(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	cm := commits[0]
	if cm.Hash != "abc123" {
		t.Errorf("Hash = %q, want %q", cm.Hash, "abc123")
	}
	if len(cm.ParentHashes) != 1 || cm.ParentHashes[0] != "def456" {
		t.Errorf("ParentHashes = %v, want [def456]", cm.ParentHashes)
	}
	if cm.AuthorName != "Alice" {
		t.Errorf("AuthorName = %q, want Alice", cm.AuthorName)
	}
	if cm.AuthorEmail != "alice@example.com" {
		t.Errorf("AuthorEmail = %q", cm.AuthorEmail)
	}
	if cm.Message != "Fix login bug" {
		t.Errorf("Message = %q, want %q", cm.Message, "Fix login bug")
	}
	wantDate := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	if !cm.AuthorDate.Equal(wantDate) {
		t.Errorf("AuthorDate = %v, want %v", cm.AuthorDate, wantDate)
	}
}

func TestLogCommits_MultipleCommits(t *testing.T) {
	output := buildLogOutput(
		struct {
			hash, parents, an, ae, ad, cn, ce, cd, msg string
		}{"aaa", "bbb", "A", "a@x", "2025-01-15T10:00:00+00:00", "A", "a@x", "2025-01-15T10:00:00+00:00", "newer"},
		struct {
			hash, parents, an, ae, ad, cn, ce, cd, msg string
		}{"bbb", "ccc", "B", "b@x", "2025-01-14T10:00:00+00:00", "B", "b@x", "2025-01-14T10:00:00+00:00", "older"},
	)

	f := &fakeRunner{results: []runResult{{Output: output}}}
	c := New(f)

	commits, err := c.LogCommits(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	if commits[0].Hash != "aaa" || commits[1].Hash != "bbb" {
		t.Errorf("order: got [%s, %s], want [aaa, bbb]", commits[0].Hash, commits[1].Hash)
	}
}

func TestLogCommits_MergeCommit(t *testing.T) {
	output := buildLogOutput(struct {
		hash, parents, an, ae, ad, cn, ce, cd, msg string
	}{
		"merge1", "parent1 parent2",
		"A", "a@x", "2025-01-15T10:00:00+00:00",
		"A", "a@x", "2025-01-15T10:00:00+00:00",
		"Merge branch 'feature'",
	})

	f := &fakeRunner{results: []runResult{{Output: output}}}
	c := New(f)

	commits, err := c.LogCommits(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits[0].ParentHashes) != 2 {
		t.Fatalf("ParentHashes = %v, want 2 parents", commits[0].ParentHashes)
	}
	if commits[0].ParentHashes[0] != "parent1" || commits[0].ParentHashes[1] != "parent2" {
		t.Errorf("ParentHashes = %v", commits[0].ParentHashes)
	}
}

func TestLogCommits_RootCommit(t *testing.T) {
	output := buildLogOutput(struct {
		hash, parents, an, ae, ad, cn, ce, cd, msg string
	}{
		"root1", "", // no parents
		"A", "a@x", "2025-01-15T10:00:00+00:00",
		"A", "a@x", "2025-01-15T10:00:00+00:00",
		"Initial commit",
	})

	f := &fakeRunner{results: []runResult{{Output: output}}}
	c := New(f)

	commits, err := c.LogCommits(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits[0].ParentHashes) != 0 {
		t.Errorf("root commit should have no parents, got %v", commits[0].ParentHashes)
	}
}

func TestLogCommits_EmptyOutput(t *testing.T) {
	f := &fakeRunner{results: []runResult{{Output: []byte("")}}}
	c := New(f)

	commits, err := c.LogCommits(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commits != nil {
		t.Errorf("expected nil, got %v", commits)
	}
}

func TestLogCommits_FullHistory_UsesFirstParent(t *testing.T) {
	f := &fakeRunner{results: []runResult{{Output: []byte("")}}}
	c := New(f)

	_, _ = c.LogCommits(context.Background(), "/repo", "", 0)

	args := strings.Join(f.calls[0].Args, " ")
	if !strings.Contains(args, "--first-parent") {
		t.Errorf("full history should use --first-parent, got args: %s", args)
	}
}

func TestLogCommits_SinceCommit_UsesRange(t *testing.T) {
	f := &fakeRunner{results: []runResult{{Output: []byte("")}}}
	c := New(f)

	_, _ = c.LogCommits(context.Background(), "/repo", "abc123", 0)

	args := strings.Join(f.calls[0].Args, " ")
	if !strings.Contains(args, "abc123..HEAD") {
		t.Errorf("since mode should use range, got args: %s", args)
	}
	if strings.Contains(args, "--first-parent") {
		t.Errorf("since mode should NOT use --first-parent, got args: %s", args)
	}
}

func TestLogCommits_MaxCount(t *testing.T) {
	f := &fakeRunner{results: []runResult{{Output: []byte("")}}}
	c := New(f)

	_, _ = c.LogCommits(context.Background(), "/repo", "", 100)

	args := strings.Join(f.calls[0].Args, " ")
	if !strings.Contains(args, "--max-count=100") {
		t.Errorf("should pass --max-count=100, got args: %s", args)
	}
}

func TestLogCommits_RunnerError(t *testing.T) {
	f := &fakeRunner{results: []runResult{{Err: errors.New("exit status 128")}}}
	c := New(f)

	_, err := c.LogCommits(context.Background(), "/repo", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitclient: log commits") {
		t.Errorf("error = %q", err.Error())
	}
}

// --- DiffStatLog tests ---

func TestDiffStatLog_MergesOutputs(t *testing.T) {
	// NUL-delimited (-z) format: name-status has status\0path\0 pairs,
	// numstat has "adds\tdels\tpath\0" records.
	nsOutput := []byte("COMMIT:aaa\x00\nA\x00src/new.ts\x00M\x00src/mod.ts\x00COMMIT:bbb\x00\nD\x00src/old.ts\x00")
	numOutput := []byte("COMMIT:aaa\x00\n10\t0\tsrc/new.ts\x005\t3\tsrc/mod.ts\x00COMMIT:bbb\x00\n0\t20\tsrc/old.ts\x00")
	patchOutput := []byte("COMMIT:aaa\n" +
		"diff --git a/src/new.ts b/src/new.ts\n" +
		"new file mode 100644\n" +
		"index 0000000..abc1234\n" +
		"--- /dev/null\n" +
		"+++ b/src/new.ts\n" +
		"@@ -0,0 +1,10 @@\n" +
		"+line1\n" +
		"diff --git a/src/mod.ts b/src/mod.ts\n" +
		"index abc1234..def5678 100644\n" +
		"--- a/src/mod.ts\n" +
		"+++ b/src/mod.ts\n" +
		"@@ -1,3 +1,5 @@\n" +
		" context\n" +
		"-old\n" +
		"+new\n" +
		"COMMIT:bbb\n" +
		"diff --git a/src/old.ts b/src/old.ts\n" +
		"deleted file mode 100644\n" +
		"index abc1234..0000000\n" +
		"--- a/src/old.ts\n" +
		"+++ /dev/null\n" +
		"@@ -1,20 +0,0 @@\n" +
		"-deleted\n")

	f := &fakeRunner{results: []runResult{
		{Output: nsOutput},
		{Output: numOutput},
		{Output: patchOutput},
	}}
	c := New(f)

	result, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check commit aaa.
	aaaDiffs := result["aaa"]
	if len(aaaDiffs) != 2 {
		t.Fatalf("aaa: got %d diffs, want 2", len(aaaDiffs))
	}
	// Find each file by path.
	for _, d := range aaaDiffs {
		switch d.Path {
		case "src/new.ts":
			if d.Status != "A" || d.Additions != 10 || d.Deletions != 0 {
				t.Errorf("src/new.ts: got %+v", d)
			}
			if d.Patch == "" {
				t.Error("src/new.ts: expected non-empty patch")
			}
			if !strings.Contains(d.Patch, "+line1") {
				t.Errorf("src/new.ts: patch should contain +line1, got %q", d.Patch)
			}
		case "src/mod.ts":
			if d.Status != "M" || d.Additions != 5 || d.Deletions != 3 {
				t.Errorf("src/mod.ts: got %+v", d)
			}
			if d.Patch == "" {
				t.Error("src/mod.ts: expected non-empty patch")
			}
		default:
			t.Errorf("unexpected path in aaa: %s", d.Path)
		}
	}

	// Check commit bbb.
	bbbDiffs := result["bbb"]
	if len(bbbDiffs) != 1 {
		t.Fatalf("bbb: got %d diffs, want 1", len(bbbDiffs))
	}
	if bbbDiffs[0].Path != "src/old.ts" || bbbDiffs[0].Status != "D" {
		t.Errorf("bbb: got %+v", bbbDiffs[0])
	}
	if bbbDiffs[0].Additions != 0 || bbbDiffs[0].Deletions != 20 {
		t.Errorf("bbb deletions: got %+v", bbbDiffs[0])
	}
	if bbbDiffs[0].Patch == "" {
		t.Error("src/old.ts: expected non-empty patch")
	}
}

func TestDiffStatLog_MissingNumstat(t *testing.T) {
	nsOutput := []byte("COMMIT:aaa\x00\nA\x00src/new.ts\x00")
	numOutput := []byte("COMMIT:aaa\x00") // no numstat for this file

	f := &fakeRunner{results: []runResult{
		{Output: nsOutput},
		{Output: numOutput},
		{Output: []byte("")}, // no patch output
	}}
	c := New(f)

	result, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	diffs := result["aaa"]
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	if diffs[0].Additions != 0 || diffs[0].Deletions != 0 {
		t.Errorf("additions/deletions should be 0 when numstat missing, got %+v", diffs[0])
	}
}

func TestDiffStatLog_EmptyOutput(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
		{Output: []byte("")},
		{Output: []byte("")},
	}}
	c := New(f)

	result, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestDiffStatLog_FirstCommandFails(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Err: errors.New("git error")},
	}}
	c := New(f)

	_, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name-status") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestDiffStatLog_SecondCommandFails(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("COMMIT:aaa\x00\nA\x00src/new.ts\x00")},
		{Err: errors.New("git error")},
	}}
	c := New(f)

	_, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "numstat") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestDiffStatLog_FullHistory_UsesFirstParent(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
		{Output: []byte("")},
		{Output: []byte("")},
	}}
	c := New(f)

	_, _ = c.DiffStatLog(context.Background(), "/repo", "", 0)

	// All three commands should have --first-parent.
	for i, call := range f.calls {
		args := strings.Join(call.Args, " ")
		if !strings.Contains(args, "--first-parent") {
			t.Errorf("call %d should use --first-parent, got: %s", i, args)
		}
	}
	// First two commands use -z, third (patch) does not.
	for i := 0; i < 2; i++ {
		args := strings.Join(f.calls[i].Args, " ")
		if !strings.Contains(args, "-z") {
			t.Errorf("call %d should use -z for NUL-delimited output, got: %s", i, args)
		}
	}
	// Third command should have -p.
	args := strings.Join(f.calls[2].Args, " ")
	if !strings.Contains(args, "-p") {
		t.Errorf("call 2 (patch) should use -p, got: %s", args)
	}
}

func TestDiffStatLog_SpecialFilenames(t *testing.T) {
	// Verify filenames with tabs, spaces, and quotes are preserved verbatim
	// when using NUL-delimited (-z) parsing.
	tabFile := "path\twith\ttabs.txt"
	spaceFile := "path with spaces.txt"

	nsOutput := []byte("COMMIT:aaa\x00\nA\x00" + tabFile + "\x00M\x00" + spaceFile + "\x00")
	numOutput := []byte("COMMIT:aaa\x00\n1\t0\t" + tabFile + "\x002\t1\t" + spaceFile + "\x00")

	f := &fakeRunner{results: []runResult{
		{Output: nsOutput},
		{Output: numOutput},
		{Output: []byte("")}, // no patch output for special filenames
	}}
	c := New(f)

	result, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	diffs := result["aaa"]
	if len(diffs) != 2 {
		t.Fatalf("got %d diffs, want 2", len(diffs))
	}
	for _, d := range diffs {
		switch d.Path {
		case tabFile:
			if d.Status != "A" || d.Additions != 1 {
				t.Errorf("tab file: got %+v", d)
			}
		case spaceFile:
			if d.Status != "M" || d.Additions != 2 || d.Deletions != 1 {
				t.Errorf("space file: got %+v", d)
			}
		default:
			t.Errorf("unexpected path: %q", d.Path)
		}
	}
}

func TestDiffStatLog_SinceCommit_UsesRange(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("")},
		{Output: []byte("")},
		{Output: []byte("")},
	}}
	c := New(f)

	_, _ = c.DiffStatLog(context.Background(), "/repo", "abc123", 0)

	for i, call := range f.calls {
		args := strings.Join(call.Args, " ")
		if !strings.Contains(args, "abc123..HEAD") {
			t.Errorf("call %d should use range, got: %s", i, args)
		}
		if strings.Contains(args, "--first-parent") {
			t.Errorf("call %d should NOT use --first-parent in since mode, got: %s", i, args)
		}
	}
}

func TestDiffStatLog_ThirdCommandFails(t *testing.T) {
	f := &fakeRunner{results: []runResult{
		{Output: []byte("COMMIT:aaa\x00\nA\x00src/new.ts\x00")},
		{Output: []byte("COMMIT:aaa\x00\n10\t0\tsrc/new.ts\x00")},
		{Err: errors.New("git error")},
	}}
	c := New(f)

	_, err := c.DiffStatLog(context.Background(), "/repo", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "patch") {
		t.Errorf("error = %q, want it to mention patch", err.Error())
	}
}

// --- mergePatchLog tests ---

func TestMergePatchLog_BasicPatch(t *testing.T) {
	entries := map[string]map[string]*FileDiffEntry{
		"aaa": {
			"src/mod.ts": {Status: "M", Path: "src/mod.ts"},
		},
	}

	data := []byte("COMMIT:aaa\n" +
		"diff --git a/src/mod.ts b/src/mod.ts\n" +
		"index abc1234..def5678 100644\n" +
		"--- a/src/mod.ts\n" +
		"+++ b/src/mod.ts\n" +
		"@@ -1,3 +1,5 @@\n" +
		" context\n" +
		"-old\n" +
		"+new\n" +
		"+added\n")

	mergePatchLog(data, entries)

	patch := entries["aaa"]["src/mod.ts"].Patch
	if patch == "" {
		t.Fatal("expected non-empty patch")
	}
	if !strings.Contains(patch, "--- a/src/mod.ts") {
		t.Errorf("patch should contain --- header, got %q", patch)
	}
	if !strings.Contains(patch, "+++ b/src/mod.ts") {
		t.Errorf("patch should contain +++ header, got %q", patch)
	}
	if !strings.Contains(patch, "@@ -1,3 +1,5 @@") {
		t.Errorf("patch should contain hunk header, got %q", patch)
	}
	if !strings.Contains(patch, "-old") {
		t.Errorf("patch should contain deletion, got %q", patch)
	}
	if !strings.Contains(patch, "+new") {
		t.Errorf("patch should contain addition, got %q", patch)
	}
}

func TestMergePatchLog_AddedDeletedModified(t *testing.T) {
	entries := map[string]map[string]*FileDiffEntry{
		"aaa": {
			"src/new.ts":  {Status: "A", Path: "src/new.ts"},
			"src/old.ts":  {Status: "D", Path: "src/old.ts"},
			"src/edit.ts": {Status: "M", Path: "src/edit.ts"},
		},
	}

	data := []byte("COMMIT:aaa\n" +
		"diff --git a/src/new.ts b/src/new.ts\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/src/new.ts\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+line1\n" +
		"+line2\n" +
		"diff --git a/src/old.ts b/src/old.ts\n" +
		"deleted file mode 100644\n" +
		"--- a/src/old.ts\n" +
		"+++ /dev/null\n" +
		"@@ -1,3 +0,0 @@\n" +
		"-del1\n" +
		"-del2\n" +
		"-del3\n" +
		"diff --git a/src/edit.ts b/src/edit.ts\n" +
		"--- a/src/edit.ts\n" +
		"+++ b/src/edit.ts\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-old\n" +
		"+new\n")

	mergePatchLog(data, entries)

	if entries["aaa"]["src/new.ts"].Patch == "" {
		t.Error("added file should have patch")
	}
	if entries["aaa"]["src/old.ts"].Patch == "" {
		t.Error("deleted file should have patch")
	}
	if entries["aaa"]["src/edit.ts"].Patch == "" {
		t.Error("modified file should have patch")
	}
}

func TestMergePatchLog_BinaryFile(t *testing.T) {
	entries := map[string]map[string]*FileDiffEntry{
		"aaa": {
			"image.png": {Status: "A", Path: "image.png"},
			"src/ok.ts": {Status: "M", Path: "src/ok.ts"},
		},
	}

	data := []byte("COMMIT:aaa\n" +
		"diff --git a/image.png b/image.png\n" +
		"new file mode 100644\n" +
		"Binary files /dev/null and b/image.png differ\n" +
		"diff --git a/src/ok.ts b/src/ok.ts\n" +
		"--- a/src/ok.ts\n" +
		"+++ b/src/ok.ts\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n")

	mergePatchLog(data, entries)

	if entries["aaa"]["image.png"].Patch != "" {
		t.Errorf("binary file should have empty patch, got %q", entries["aaa"]["image.png"].Patch)
	}
	if entries["aaa"]["src/ok.ts"].Patch == "" {
		t.Error("text file should have patch")
	}
}

func TestMergePatchLog_OversizedPatch(t *testing.T) {
	entries := map[string]map[string]*FileDiffEntry{
		"aaa": {
			"big.ts": {Status: "M", Path: "big.ts"},
		},
	}

	// Build a patch larger than maxPatchBytes (100KB).
	var sb strings.Builder
	sb.WriteString("COMMIT:aaa\n")
	sb.WriteString("diff --git a/big.ts b/big.ts\n")
	sb.WriteString("--- a/big.ts\n")
	sb.WriteString("+++ b/big.ts\n")
	sb.WriteString("@@ -1,1 +1,10000 @@\n")
	bigLine := "+" + strings.Repeat("x", 200) + "\n"
	for sb.Len() < maxPatchBytes+1000 {
		sb.WriteString(bigLine)
	}

	mergePatchLog([]byte(sb.String()), entries)

	if entries["aaa"]["big.ts"].Patch != "" {
		t.Errorf("oversized patch should be discarded, got %d bytes", len(entries["aaa"]["big.ts"].Patch))
	}
}

func TestMergePatchLog_MultipleFilesPerCommit(t *testing.T) {
	entries := map[string]map[string]*FileDiffEntry{
		"aaa": {
			"a.ts": {Status: "M", Path: "a.ts"},
			"b.ts": {Status: "M", Path: "b.ts"},
			"c.ts": {Status: "M", Path: "c.ts"},
		},
	}

	data := []byte("COMMIT:aaa\n" +
		"diff --git a/a.ts b/a.ts\n" +
		"--- a/a.ts\n" +
		"+++ b/a.ts\n" +
		"@@ -1 +1 @@\n" +
		"-a_old\n" +
		"+a_new\n" +
		"diff --git a/b.ts b/b.ts\n" +
		"--- a/b.ts\n" +
		"+++ b/b.ts\n" +
		"@@ -1 +1 @@\n" +
		"-b_old\n" +
		"+b_new\n" +
		"diff --git a/c.ts b/c.ts\n" +
		"--- a/c.ts\n" +
		"+++ b/c.ts\n" +
		"@@ -1 +1 @@\n" +
		"-c_old\n" +
		"+c_new\n")

	mergePatchLog(data, entries)

	for _, path := range []string{"a.ts", "b.ts", "c.ts"} {
		if entries["aaa"][path].Patch == "" {
			t.Errorf("%s should have patch", path)
		}
	}
	if !strings.Contains(entries["aaa"]["a.ts"].Patch, "+a_new") {
		t.Error("a.ts patch should contain +a_new")
	}
	if !strings.Contains(entries["aaa"]["b.ts"].Patch, "+b_new") {
		t.Error("b.ts patch should contain +b_new")
	}
	if !strings.Contains(entries["aaa"]["c.ts"].Patch, "+c_new") {
		t.Error("c.ts patch should contain +c_new")
	}
}

func TestMergePatchLog_EmptyOutput(t *testing.T) {
	entries := map[string]map[string]*FileDiffEntry{}
	mergePatchLog([]byte(""), entries)
	if len(entries) != 0 {
		t.Errorf("expected empty entries, got %v", entries)
	}
}

func TestMergePatchLog_FileNotInEntries(t *testing.T) {
	// A file appears in patch output but not in the pre-existing entries map.
	entries := map[string]map[string]*FileDiffEntry{
		"aaa": {
			"known.ts": {Status: "M", Path: "known.ts"},
		},
	}

	data := []byte("COMMIT:aaa\n" +
		"diff --git a/unknown.ts b/unknown.ts\n" +
		"--- a/unknown.ts\n" +
		"+++ b/unknown.ts\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n" +
		"diff --git a/known.ts b/known.ts\n" +
		"--- a/known.ts\n" +
		"+++ b/known.ts\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n")

	mergePatchLog(data, entries)

	// unknown.ts should be silently skipped (not in entries map).
	if _, ok := entries["aaa"]["unknown.ts"]; ok {
		t.Error("unknown file should not be added to entries")
	}
	// known.ts should still get its patch.
	if entries["aaa"]["known.ts"].Patch == "" {
		t.Error("known.ts should have patch")
	}
}

// --- parseDiffGitPath tests ---

func TestParseDiffGitPath(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"modified file", "diff --git a/src/mod.ts b/src/mod.ts", "src/mod.ts"},
		{"added file", "diff --git a/src/new.ts b/src/new.ts", "src/new.ts"},
		{"deleted file", "diff --git a/src/old.ts b/src/old.ts", "src/old.ts"},
		{"nested path", "diff --git a/pkg/internal/handler.go b/pkg/internal/handler.go", "pkg/internal/handler.go"},
		{"path with spaces", "diff --git a/path with spaces.txt b/path with spaces.txt", "path with spaces.txt"},
		{"path containing b/ segment", "diff --git a/lib b/utils.ts b/lib b/utils.ts", "lib b/utils.ts"},
		{"path starting with b/", "diff --git a/b/file.txt b/b/file.txt", "b/file.txt"},
		{"quoted non-ASCII path", "diff --git \"a/na\\303\\257ve.txt\" \"b/na\\303\\257ve.txt\"", "na\xc3\xafve.txt"},
		{"quoted path with tab", "diff --git \"a/has\\ttab.txt\" \"b/has\\ttab.txt\"", "has\ttab.txt"},
		{"quoted path with backslash", "diff --git \"a/back\\\\slash.txt\" \"b/back\\\\slash.txt\"", "back\\slash.txt"},
		{"empty prefix", "not a diff line", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiffGitPath(tt.line)
			if got != tt.want {
				t.Errorf("parseDiffGitPath(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestUnquoteGitPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escapes", "plain.txt", "plain.txt"},
		{"octal UTF-8 bytes", "na\\303\\257ve.txt", "na\xc3\xafve.txt"},
		{"tab escape", "has\\ttab.txt", "has\ttab.txt"},
		{"newline escape", "has\\nnewline.txt", "has\nnewline.txt"},
		{"backslash escape", "back\\\\slash.txt", "back\\slash.txt"},
		{"quote escape", "has\\\"quote.txt", "has\"quote.txt"},
		{"multiple octals", "\\303\\251\\303\\250.txt", "\xc3\xa9\xc3\xa8.txt"},
		{"mixed escapes", "a\\tb\\nc\\\\d\\303\\257", "a\tb\nc\\d\xc3\xaf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unquoteGitPath(tt.input)
			if got != tt.want {
				t.Errorf("unquoteGitPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
