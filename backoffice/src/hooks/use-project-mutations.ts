"use client";

import { toast } from "sonner";
import { api } from "~/trpc/react";
import type { UserProject } from "~/server/api/routers/users";

/**
 * Encapsulates project list mutations (trigger index, toggle status).
 * Handles success toasts, error toasts, and cache invalidation.
 */
export function useProjectMutations(projectItems: UserProject[] | undefined) {
  const utils = api.useUtils();

  const triggerIndex = api.projectIndexing.triggerIndex.useMutation({
    onSuccess: (_data, variables) => {
      const project = projectItems?.find((p) => p.id === variables.projectId);
      toast.success(`Indexing started for ${project?.name ?? "project"}`);
      void utils.users.listMyProjects.invalidate();
      void utils.dashboard.summary.invalidate();
    },
    onError: (error) => {
      toast.error(`Failed to start indexing: ${error.message}`);
    },
  });

  const toggleStatus = api.projects.update.useMutation({
    onSuccess: () => {
      toast.success("Project status updated");
      void utils.users.listMyProjects.invalidate();
    },
    onError: (error) => {
      toast.error(`Failed to update status: ${error.message}`);
    },
  });

  return { triggerIndex, toggleStatus } as const;
}
