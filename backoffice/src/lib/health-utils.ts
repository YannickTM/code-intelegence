import type {
  ProjectWithHealth,
  HealthStatus,
  AlertType,
} from "./dashboard-types";

const STALE_THRESHOLD_MS = 48 * 60 * 60 * 1000; // 48 hours

export function getProjectHealthStatus(
  project: ProjectWithHealth,
): HealthStatus {
  if (project.failed_job_id) return "failed";
  if (project.active_job_id) return "indexing";
  if (!project.index_git_commit) return "never_indexed";
  if (project.index_activated_at) {
    const age = Date.now() - new Date(project.index_activated_at).getTime();
    if (age > STALE_THRESHOLD_MS) return "stale";
  }
  return "healthy";
}

const SEVERITY_ORDER: Record<HealthStatus, number> = {
  failed: 0,
  stale: 1,
  never_indexed: 2,
  indexing: 3,
  healthy: 4,
};

export function sortProjectsByHealth(
  projects: ProjectWithHealth[],
): ProjectWithHealth[] {
  return [...projects].sort((a, b) => {
    const sa = SEVERITY_ORDER[getProjectHealthStatus(a)];
    const sb = SEVERITY_ORDER[getProjectHealthStatus(b)];
    if (sa !== sb) return sa - sb;
    return a.name.localeCompare(b.name);
  });
}

export function getAlertForProject(
  project: ProjectWithHealth,
): AlertType | null {
  const status = getProjectHealthStatus(project);
  switch (status) {
    case "failed":
      return "failed";
    case "never_indexed":
      return "never_indexed";
    case "stale":
      return "stale";
    default:
      return null;
  }
}

// Re-exported from shared utility for backwards compatibility
export { formatRelativeTime } from "~/lib/format";

export function getStatusDotColor(status: HealthStatus): string {
  switch (status) {
    case "failed":
      return "bg-destructive";
    case "stale":
      return "bg-warning";
    case "never_indexed":
      return "bg-muted-foreground";
    case "indexing":
      return "bg-info animate-pulse";
    case "healthy":
      return "bg-success";
  }
}

export function getStatusTextColor(status: HealthStatus): string {
  switch (status) {
    case "failed":
      return "text-destructive";
    case "stale":
      return "text-warning";
    case "never_indexed":
      return "text-muted-foreground";
    case "indexing":
      return "text-info";
    case "healthy":
      return "text-success";
  }
}

export function getHealthLabel(status: HealthStatus): string {
  switch (status) {
    case "failed":
      return "Failed";
    case "stale":
      return "Stale";
    case "never_indexed":
      return "Not indexed";
    case "indexing":
      return "Indexing...";
    case "healthy":
      return "Healthy";
  }
}
