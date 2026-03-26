package sshenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fake keyscanner ---

type fakeKeyscanner struct {
	output []byte
	err    error
}

func (f *fakeKeyscanner) Scan(_ context.Context, _ string) ([]byte, error) {
	return f.output, f.err
}

// --- Setup tests ---

func TestSetup_WritesKeyWithCorrectPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := &fakeKeyscanner{output: []byte("github.com ssh-ed25519 AAAA...\n")}

	env, err := Setup(context.Background(), tmpDir, []byte("PRIVATE-KEY"), "github.com", scanner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(env.KeyPath)
	if err != nil {
		t.Fatalf("key file not found: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("key permissions = %o, want 0600", info.Mode().Perm())
	}

	content, err := os.ReadFile(env.KeyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if string(content) != "PRIVATE-KEY" {
		t.Errorf("key content = %q, want %q", content, "PRIVATE-KEY")
	}
}

func TestSetup_WritesKnownHosts(t *testing.T) {
	tmpDir := t.TempDir()
	hostLine := "github.com ssh-ed25519 AAAA...\n"
	scanner := &fakeKeyscanner{output: []byte(hostLine)}

	env, err := Setup(context.Background(), tmpDir, []byte("KEY"), "github.com", scanner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(env.KnownHostsPath)
	if err != nil {
		t.Fatalf("known_hosts not found: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("known_hosts permissions = %o, want 0600", info.Mode().Perm())
	}

	content, err := os.ReadFile(env.KnownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if string(content) != hostLine {
		t.Errorf("known_hosts content = %q, want %q", content, hostLine)
	}
}

func TestSetup_GitSSHCmdFormat(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := &fakeKeyscanner{output: []byte("host ssh-rsa AAAA...\n")}

	env, err := Setup(context.Background(), tmpDir, []byte("KEY"), "host", scanner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(env.EnvVars) != 1 {
		t.Fatalf("EnvVars length = %d, want 1", len(env.EnvVars))
	}
	sshCmd := env.EnvVars[0]
	if !strings.HasPrefix(sshCmd, "GIT_SSH_COMMAND=") {
		t.Errorf("EnvVars[0] should start with GIT_SSH_COMMAND=, got %q", sshCmd)
	}

	keyPath := filepath.Join(tmpDir, "id_key")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	quotedKey := "'" + keyPath + "'"
	quotedKnown := "'" + knownHostsPath + "'"
	if !strings.Contains(sshCmd, quotedKey) {
		t.Errorf("GIT_SSH_COMMAND should contain quoted key path %q, got %q", quotedKey, sshCmd)
	}
	if !strings.Contains(sshCmd, quotedKnown) {
		t.Errorf("GIT_SSH_COMMAND should contain quoted known_hosts path %q, got %q", quotedKnown, sshCmd)
	}
	if !strings.Contains(sshCmd, "StrictHostKeyChecking=yes") {
		t.Errorf("GIT_SSH_COMMAND should contain StrictHostKeyChecking=yes, got %q", sshCmd)
	}
	if !strings.Contains(sshCmd, "IdentitiesOnly=yes") {
		t.Errorf("GIT_SSH_COMMAND should contain IdentitiesOnly=yes, got %q", sshCmd)
	}
}

func TestSetup_KeyscanFailure(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := &fakeKeyscanner{err: errors.New("connection refused")}

	_, err := Setup(context.Background(), tmpDir, []byte("KEY"), "host", scanner)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "keyscan") {
		t.Errorf("error should mention keyscan, got %q", err.Error())
	}
}

func TestSetup_KeyscanFailure_RemovesKey(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := &fakeKeyscanner{err: errors.New("connection refused")}

	_, _ = Setup(context.Background(), tmpDir, []byte("SECRET-KEY"), "host", scanner)

	keyPath := filepath.Join(tmpDir, "id_key")
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("private key should be deleted after keyscan failure")
	}
}

func TestSetup_KnownHostsWriteFailure_RemovesKey(t *testing.T) {
	// Create a read-only directory so writing known_hosts fails.
	tmpDir := t.TempDir()
	scanner := &fakeKeyscanner{output: []byte("host ssh-rsa AAAA...\n")}

	// Write key first (Setup writes it), then make dir read-only before known_hosts write.
	// Instead, we use a trick: write the key file, then make tmpDir read-only.
	// But Setup writes both — so we need to let key write succeed but known_hosts fail.
	// Create a file at the known_hosts path to make it a directory (causes write error).
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	if err := os.Mkdir(knownHostsPath, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Setup(context.Background(), tmpDir, []byte("SECRET-KEY"), "host", scanner)
	if err == nil {
		t.Fatal("expected error writing known_hosts")
	}

	keyPath := filepath.Join(tmpDir, "id_key")
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("private key should be deleted after known_hosts write failure")
	}
}

// --- ParseHostname tests ---

func TestParseHostname(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "scp-style",
			url:  "git@github.com:org/repo.git",
			want: "github.com",
		},
		{
			name: "ssh-scheme",
			url:  "ssh://git@github.com/org/repo.git",
			want: "github.com",
		},
		{
			name:    "ssh-scheme-with-non-default-port",
			url:     "ssh://git@github.com:2222/org/repo.git",
			wantErr: true,
		},
		{
			name: "ssh-scheme-with-default-port",
			url:  "ssh://git@github.com:22/org/repo.git",
			want: "github.com",
		},
		{
			name: "gitlab-scp",
			url:  "git@gitlab.com:group/project.git",
			want: "gitlab.com",
		},
		{
			name:    "no-at-sign",
			url:     "https://github.com/org/repo.git",
			want:    "github.com",
			wantErr: false,
		},
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
		{
			name:    "garbage",
			url:     "not-a-url",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHostname(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("hostname = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Cleanup tests ---

func TestCleanup_RemovesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_key")
	knownPath := filepath.Join(tmpDir, "known_hosts")

	if err := os.WriteFile(keyPath, []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownPath, []byte("hosts"), 0600); err != nil {
		t.Fatal(err)
	}

	env := &Env{KeyPath: keyPath, KnownHostsPath: knownPath}
	env.Cleanup()

	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("key file should be deleted")
	}
	if _, err := os.Stat(knownPath); !os.IsNotExist(err) {
		t.Error("known_hosts should be deleted")
	}
}

func TestCleanup_Idempotent(t *testing.T) {
	env := &Env{KeyPath: "/nonexistent/key", KnownHostsPath: "/nonexistent/hosts"}
	// Should not panic.
	env.Cleanup()
	env.Cleanup()
}

func TestCleanup_NilEnv(t *testing.T) {
	var env *Env
	// Should not panic.
	env.Cleanup()
}

// --- shellQuote tests ---

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "/tmp/id_key", "'/tmp/id_key'"},
		{"space", "/tmp/my dir/id_key", "'/tmp/my dir/id_key'"},
		{"single-quote", "/tmp/it's/key", "'/tmp/it'\"'\"'s/key'"},
		{"empty", "", "''"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shellQuote(tc.input)
			if got != tc.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- writeNewFile tests ---

func TestWriteNewFile(t *testing.T) {
	t.Run("creates-new-file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test")
		if err := writeNewFile(path, []byte("data"), 0600); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
		}
		content, _ := os.ReadFile(path)
		if string(content) != "data" {
			t.Errorf("content = %q, want %q", content, "data")
		}
	})

	t.Run("fails-if-exists", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test")
		if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		err := writeNewFile(path, []byte("new"), 0600)
		if err == nil {
			t.Fatal("expected error for existing file")
		}
	})

	t.Run("no-file-left-on-open-failure", func(t *testing.T) {
		dir := t.TempDir()
		// Use a directory as the target path — OpenFile on a directory
		// always fails, regardless of the user (including root).
		pathDir := filepath.Join(dir, "testfile")
		if err := os.Mkdir(pathDir, 0755); err != nil {
			t.Fatal(err)
		}
		err := writeNewFile(pathDir, []byte("data"), 0600)
		if err == nil {
			t.Fatal("expected error when target is a directory")
		}
	})
}

func TestSetup_FailsIfKeyExists(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_key")
	if err := os.WriteFile(keyPath, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	scanner := &fakeKeyscanner{output: []byte("host ssh-rsa AAAA...\n")}

	_, err := Setup(context.Background(), tmpDir, []byte("NEW-KEY"), "host", scanner)
	if err == nil {
		t.Fatal("expected error when key file already exists")
	}
	if !strings.Contains(err.Error(), "write key") {
		t.Errorf("error should mention 'write key', got %q", err.Error())
	}
}

// --- ParseHostname credential redaction test ---

func TestParseHostname_RedactsCredentials(t *testing.T) {
	_, err := ParseHostname("https://user:secret-token@")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("error should not contain password, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "user") {
		t.Errorf("error should not contain username, got %q", err.Error())
	}
}
