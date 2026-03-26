/** Regex: accepts https://, http://, ssh://, or git@host:org/repo */
const REPO_URL_RE = /^(https?:\/\/.+|ssh:\/\/.+|git@.+:.+)$/;

export function isValidRepoUrl(url: string): boolean {
  return REPO_URL_RE.test(url.trim());
}

/**
 * Extract a project name from a repo URL.
 *
 * "https://github.com/org/my-repo.git" → "my-repo"
 * "git@github.com:org/my-repo.git"     → "my-repo"
 */
export function extractProjectName(repoUrl: string): string {
  const trimmed = repoUrl.trim().replace(/\.git$/, "");
  const lastSlash = trimmed.lastIndexOf("/");
  const lastColon = trimmed.lastIndexOf(":");
  const pos = Math.max(lastSlash, lastColon);
  if (pos >= 0 && pos < trimmed.length - 1) {
    return trimmed.slice(pos + 1);
  }
  return "";
}

export type GitProvider = "github" | "gitlab" | "bitbucket" | "unknown";

/** Detect git provider from repo URL host. */
export function detectProvider(repoUrl: string): GitProvider {
  const lower = repoUrl.toLowerCase();
  if (lower.includes("github.com")) return "github";
  if (lower.includes("gitlab.com")) return "gitlab";
  if (lower.includes("bitbucket.org")) return "bitbucket";
  return "unknown";
}

/** Build a deep link to the provider's deploy key settings page. */
export function getDeployKeyUrl(
  repoUrl: string,
  provider: GitProvider,
): string | null {
  if (provider === "github") {
    const match = /github\.com[/:](.+?)(?:\.git\s*)?$/.exec(repoUrl);
    if (match?.[1]) return `https://github.com/${match[1]}/settings/keys/new`;
  }

  if (provider === "gitlab") {
    const match = /gitlab\.com[/:](.+?)(?:\.git\s*)?$/.exec(repoUrl);
    if (match?.[1])
      return `https://gitlab.com/${match[1]}/-/settings/repository`;
  }

  if (provider === "bitbucket") {
    const match = /bitbucket\.org[/:](.+?)(?:\.git\s*)?$/.exec(repoUrl);
    if (match?.[1])
      return `https://bitbucket.org/${match[1]}/admin/deploy-keys/add`;
  }

  return null;
}

/** Human-readable provider label. */
export function getProviderLabel(provider: GitProvider): string {
  switch (provider) {
    case "github":
      return "GitHub";
    case "gitlab":
      return "GitLab";
    case "bitbucket":
      return "Bitbucket";
    default:
      return "Git provider";
  }
}
