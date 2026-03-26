import { describe, expect, it } from "vitest";
import type { UserProject } from "~/server/api/routers/users";
import { deriveProjectStatus, getStatusBadgeConfig } from "./project-status";

function makeProject(overrides: Partial<UserProject> = {}): UserProject {
  return {
    id: "proj-1",
    name: "Test Project",
    repo_url: "https://github.com/org/repo.git",
    default_branch: "main",
    status: "active",
    role: "owner",
    created_by: "user-1",
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    index_git_commit: null,
    index_branch: null,
    index_activated_at: null,
    active_job_id: null,
    active_job_status: null,
    failed_job_id: null,
    failed_job_finished_at: null,
    failed_job_type: null,
    ...overrides,
  };
}

describe("deriveProjectStatus", () => {
  it("returns 'active' for a normal active project", () => {
    expect(deriveProjectStatus(makeProject())).toBe("active");
  });

  it("returns 'paused' when status is paused", () => {
    expect(deriveProjectStatus(makeProject({ status: "paused" }))).toBe(
      "paused",
    );
  });

  it("returns 'indexing' when active_job_status is 'queued'", () => {
    expect(
      deriveProjectStatus(
        makeProject({
          active_job_id: "job-1",
          active_job_status: "queued",
        }),
      ),
    ).toBe("indexing");
  });

  it("returns 'indexing' when active_job_status is 'running'", () => {
    expect(
      deriveProjectStatus(
        makeProject({
          active_job_id: "job-1",
          active_job_status: "running",
        }),
      ),
    ).toBe("indexing");
  });

  it("returns 'error' when failed_job_id is set", () => {
    expect(
      deriveProjectStatus(
        makeProject({
          failed_job_id: "job-1",
          failed_job_finished_at: "2025-01-01T00:00:00Z",
          failed_job_type: "index",
        }),
      ),
    ).toBe("error");
  });

  // ── Priority tests ──────────────────────────────────────────────────

  it("indexing takes priority over paused", () => {
    expect(
      deriveProjectStatus(
        makeProject({
          status: "paused",
          active_job_id: "job-1",
          active_job_status: "running",
        }),
      ),
    ).toBe("indexing");
  });

  it("indexing takes priority over error", () => {
    expect(
      deriveProjectStatus(
        makeProject({
          failed_job_id: "job-old",
          active_job_id: "job-new",
          active_job_status: "queued",
        }),
      ),
    ).toBe("indexing");
  });

  it("error takes priority over paused", () => {
    expect(
      deriveProjectStatus(
        makeProject({
          status: "paused",
          failed_job_id: "job-1",
        }),
      ),
    ).toBe("error");
  });
});

describe("getStatusBadgeConfig", () => {
  it.each([
    ["indexing", "Indexing"],
    ["error", "Error"],
    ["active", "Active"],
    ["paused", "Paused"],
  ] as const)("returns label '%s' for status '%s'", (status, expectedLabel) => {
    expect(getStatusBadgeConfig(status).label).toBe(expectedLabel);
  });

  it("returns a className string for every status", () => {
    for (const status of ["indexing", "error", "active", "paused"] as const) {
      expect(getStatusBadgeConfig(status).className).toBeTruthy();
    }
  });
});
