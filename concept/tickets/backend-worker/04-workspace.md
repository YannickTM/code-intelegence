# 04 — Git Workspace & Repository Cache

## Status
Done

## Goal
Implement the project-scoped local workspace model for repo-required workflows: SSH agent setup with decrypted Ed25519 keys, Git clone/fetch into a persistent repo cache, per-job worktree creation for branch isolation, tracked file enumeration, and cleanup on completion.

## Depends On
01-foundation, 03-job-lifecycle

## Scope

### Workspace Layout

The `REPO_CACHE_DIR` (default: `/var/lib/myjungle/repos`) serves as root for worker-local repo state:

```
REPO_CACHE_DIR/
  projects/{project_id}/repo     # Working clone, reusable across jobs
  jobs/{job_id}/tmp              # Disposable temp files (SSH key, etc.)
  jobs/{job_id}/worktree         # Per-job git worktree (isolated checkout)
```

Rules:
- Project repo cache is reusable across jobs (avoids full re-clone)
- Job temp files are disposable and cleaned up after every run
- Workspaces are isolated by project -- cleanup never touches another project's files
- The cache is not the source of truth; it can be deleted without data loss

### SSH Execution (`internal/sshenv/`)

Turns the decrypted SSH private key (from ticket 03) into a safe Git execution environment:

1. Write decrypted private key to `jobs/{job_id}/tmp/id_key` with `0600` permissions
2. Set `GIT_SSH_COMMAND` pointing Git at the key file with `StrictHostKeyChecking=yes`
3. Run `ssh-keyscan` to populate a deterministic `known_hosts` file for the host extracted from `projects.repo_url`

Host-key checking is never disabled globally. The hostname is parsed from the repo URL to scope the keyscan.

### Git Lifecycle (`internal/gitclient/`)

Each project uses a single branch (`projects.default_branch`):

1. **Clone** if the project cache does not exist
2. **Verify** remote URL matches `projects.repo_url` before reuse (mismatch triggers fresh clone)
3. **Fetch** remote refs when cache exists
4. **Create worktree** at `jobs/{job_id}/worktree` checked out at the target branch ref

After checkout, the worker records the exact `git_commit` (HEAD SHA) for the snapshot.

### File Enumeration

Uses `git ls-files` so the parser only sees tracked repo files:
- Include all tracked regular files under 1 MB
- Binary files (non-UTF-8) are included in the file list but their content is skipped by `readSourceFiles`
- Symlinks and non-regular files are excluded
- Return stable ordering for deterministic behavior

### Workspace Preparation (`internal/workspace/`)

The `Prepare` function orchestrates the full workspace lifecycle for a workflow:
1. Set up SSH environment
2. Clone or fetch the repository
3. Create a per-job worktree
4. Enumerate tracked files
5. Return the worktree path, commit SHA, and file list

### Cleanup

Phase 1 policy:
- Job temp files (`jobs/{job_id}/`) are deleted immediately after the workflow exits (success or failure)
- The worktree is removed via `git worktree remove`
- Project repo cache is preserved for reuse across jobs
- Stale cache pruning is deferred to a future background policy

### Multi-Worker Behavior

In scaled deployments:
- Each worker has its own local workspace or cache
- Workers clone or fetch as needed
- Cache reuse is opportunistic, not required
- Correctness does not depend on shared storage

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/workspace/` | Cache layout, directory management, worktree lifecycle, `Prepare()` |
| `internal/gitclient/` | Clone, fetch, worktree add/remove, file-list helpers, remote URL verification |
| `internal/sshenv/` | Temp-key writer, known-hosts via keyscan, hostname parsing |

## Acceptance Criteria
- [x] Worker creates a stable project-scoped repo cache under `REPO_CACHE_DIR`
- [x] Worker clones a repo on first use and fetches on later runs
- [x] Worker checks out `projects.default_branch` safely
- [x] Worker records the exact Git commit SHA used for the snapshot
- [x] SSH key material is written only to job-scoped temp files with `0600` permissions
- [x] Host key trust is handled via `ssh-keyscan`, not by disabling checks
- [x] Remote URL mismatch is detected before reusing a cached clone
- [x] File selection returns all tracked regular files under the size limit
- [x] Each job operates in its own isolated git worktree
- [x] Job temp files are cleaned up when execution ends
- [x] Repo caches remain isolated by project
- [x] Cleanup preserves the persistent project repo cache
