export type DashboardSummary = {
  projects_total: number;
  jobs_active: number;
  jobs_failed_24h: number;
  query_count_24h: number;
  p95_latency_ms_24h: number;
};

export type ProjectRole = "owner" | "admin" | "member";

export type ProjectWithHealth = {
  id: string;
  name: string;
  repo_url: string;
  default_branch: string;
  status: string;
  role: ProjectRole;
  created_by: string;
  created_at: string;
  updated_at: string;
  index_git_commit: string | null;
  index_branch: string | null;
  index_activated_at: string | null;
  active_job_id: string | null;
  active_job_status: string | null;
  failed_job_id: string | null;
  failed_job_finished_at: string | null;
  failed_job_type: string | null;
};

export type HealthStatus =
  | "failed"
  | "indexing"
  | "stale"
  | "never_indexed"
  | "healthy";

export type AlertType = "failed" | "never_indexed" | "stale";
