"use client";

import { api } from "~/trpc/react";
import type { ProjectWithHealth } from "~/lib/dashboard-types";

export function useProjectDetail(projectId: string) {
  const projectQuery = api.projects.get.useQuery({ id: projectId });

  // Get role from listMyProjects cache (role is per-user, not in GET /projects/{id})
  const utils = api.useUtils();
  const cachedProjects = utils.users.listMyProjects.getData();
  const cachedProject = cachedProjects?.items?.find((p) => p.id === projectId);

  // Fallback: fetch listMyProjects if not in cache
  const projectsQuery = api.users.listMyProjects.useQuery(undefined, {
    enabled: !cachedProject,
  });

  // Resolve role: cache hit → query data → undefined while loading → "member" fallback
  const roleLoading = !cachedProject && projectsQuery.isLoading;
  const resolvedRole =
    cachedProject?.role ??
    projectsQuery.data?.items?.find((p) => p.id === projectId)?.role;
  const role = resolvedRole ?? "member";

  const isAdminOrOwner = !roleLoading && (role === "admin" || role === "owner");

  // Merge backend response with role into ProjectWithHealth shape
  const project: ProjectWithHealth | undefined =
    projectQuery.data && !roleLoading
      ? { ...projectQuery.data, role }
      : undefined;

  return {
    project,
    role,
    isAdminOrOwner,
    isLoading: projectQuery.isLoading || roleLoading,
    isError: projectQuery.isError,
    error: projectQuery.error,
    refetch: projectQuery.refetch,
  };
}
