// Package sshenv manages temporary SSH key files and known_hosts for git operations.
package sshenv

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Keyscanner runs ssh-keyscan and returns the known_hosts content.
type Keyscanner interface {
	Scan(ctx context.Context, hostname string) ([]byte, error)
}

// ExecKeyscanner is the production implementation using the ssh-keyscan binary.
type ExecKeyscanner struct{}

// Scan runs ssh-keyscan for the given hostname and returns the output.
func (ExecKeyscanner) Scan(ctx context.Context, hostname string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "ssh-keyscan", "-t", "ed25519,rsa,ecdsa", hostname).Output()
	if err != nil {
		return nil, fmt.Errorf("sshenv: ssh-keyscan %s: %w", hostname, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sshenv: ssh-keyscan %s: no keys returned", hostname)
	}
	return out, nil
}

// Env holds the SSH environment state for a single job.
type Env struct {
	KeyPath        string   // path to the written private key file
	KnownHostsPath string   // path to the known_hosts file
	EnvVars        []string // ["GIT_SSH_COMMAND=..."] ready for exec.Cmd.Env
}

// Setup writes the SSH private key to tmpDir/id_key with 0600 permissions,
// runs ssh-keyscan to generate known_hosts for the given hostname,
// and returns an Env with the GIT_SSH_COMMAND configured.
//
// The caller must create tmpDir before calling Setup.
func Setup(ctx context.Context, tmpDir string, privateKey []byte, hostname string, scanner Keyscanner) (*Env, error) {
	keyPath := filepath.Join(tmpDir, "id_key")
	if err := writeNewFile(keyPath, privateKey, 0600); err != nil {
		return nil, fmt.Errorf("sshenv: write key: %w", err)
	}

	knownHostsContent, err := scanner.Scan(ctx, hostname)
	if err != nil {
		os.Remove(keyPath) // best-effort: don't leave private key on disk
		return nil, fmt.Errorf("sshenv: keyscan: %w", err)
	}

	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	if err := writeNewFile(knownHostsPath, knownHostsContent, 0600); err != nil {
		os.Remove(keyPath) // best-effort: don't leave private key on disk
		return nil, fmt.Errorf("sshenv: write known_hosts: %w", err)
	}

	gitSSHCmd := fmt.Sprintf("ssh -i %s -o UserKnownHostsFile=%s -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes",
		shellQuote(keyPath), shellQuote(knownHostsPath))

	return &Env{
		KeyPath:        keyPath,
		KnownHostsPath: knownHostsPath,
		EnvVars:        []string{"GIT_SSH_COMMAND=" + gitSSHCmd},
	}, nil
}

// Cleanup removes the temporary SSH key and known_hosts files.
// It is safe to call multiple times.
func (e *Env) Cleanup() {
	if e == nil {
		return
	}
	for _, p := range []string{e.KeyPath, e.KnownHostsPath} {
		if p == "" {
			continue
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("sshenv: cleanup failed", slog.String("path", p), slog.Any("error", err))
		}
	}
}

// shellQuote wraps s in single quotes for safe shell interpolation.
// Embedded single quotes are escaped as '"'"'.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// writeNewFile creates a new file at path with the given permissions and writes data.
// It returns an error if the file already exists (O_EXCL), ensuring permissions
// are always set correctly on creation.
func writeNewFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		os.Remove(path) // best-effort: don't leave partial file on disk
		return writeErr
	}
	if closeErr != nil {
		os.Remove(path) // best-effort: don't leave partial file on disk
		return closeErr
	}
	return nil
}

// ParseHostname extracts the hostname from a git SSH URL.
// It supports the scp-style "git@host:org/repo.git" format and
// the ssh:// URL format "ssh://git@host/org/repo.git".
func ParseHostname(repoURL string) (string, error) {
	// Try ssh:// URL format first.
	if strings.HasPrefix(repoURL, "ssh://") || strings.Contains(repoURL, "://") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("sshenv: parse url: invalid repository URL")
		}
		// Strip all userinfo for safe logging.
		u.User = nil
		safeURL := u.String()
		host := u.Hostname()
		if host == "" {
			return "", fmt.Errorf("sshenv: no hostname in url %q", safeURL)
		}
		// Reject non-default SSH ports — ssh-keyscan only targets port 22.
		if port := u.Port(); port != "" && port != "22" && u.Scheme == "ssh" {
			return "", fmt.Errorf("sshenv: non-default SSH port %s not supported in %q", port, safeURL)
		}
		return host, nil
	}

	// SCP-style: git@host:org/repo.git
	atIdx := strings.Index(repoURL, "@")
	if atIdx < 0 {
		return "", fmt.Errorf("sshenv: cannot parse hostname from %q", repoURL)
	}
	rest := repoURL[atIdx+1:]
	colonIdx := strings.Index(rest, ":")
	if colonIdx <= 0 {
		return "", fmt.Errorf("sshenv: cannot parse hostname from %q", repoURL)
	}
	return rest[:colonIdx], nil
}
