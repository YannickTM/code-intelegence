import type { UserProject } from "~/server/api/routers/users";

/** Visual status for the projects list (per ticket 002 spec) */
export type ProjectStatus = "indexing" | "error" | "active" | "paused";

/** Status filter for the toolbar chips */
export type StatusFilter = "all" | ProjectStatus;

/**
 * Derive display status from project data.
 * Priority: Indexing > Error > Paused > Active
 */
export function deriveProjectStatus(project: UserProject): ProjectStatus {
  // Indexing takes highest priority — even paused projects may show indexing
  if (
    project.active_job_status === "queued" ||
    project.active_job_status === "running"
  )
    return "indexing";
  // Error next
  if (project.failed_job_id) return "error";
  // DB-level status
  if (project.status === "paused") return "paused";
  return "active";
}

export function getStatusBadgeConfig(status: ProjectStatus): {
  label: string;
  className: string;
} {
  switch (status) {
    case "indexing":
      return {
        label: "Indexing",
        className: "bg-info/10 text-info",
      };
    case "error":
      return {
        label: "Error",
        className: "bg-destructive/10 text-destructive",
      };
    case "active":
      return {
        label: "Active",
        className: "bg-success/10 text-success",
      };
    case "paused":
      return {
        label: "Paused",
        className: "bg-warning/10 text-warning",
      };
  }
}
