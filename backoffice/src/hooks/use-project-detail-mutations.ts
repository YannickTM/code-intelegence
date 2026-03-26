"use client";

import { toast } from "sonner";
import { useRouter } from "next/navigation";
import { api } from "~/trpc/react";

export function useProjectDetailMutations(projectId: string) {
  const utils = api.useUtils();
  const router = useRouter();

  const triggerIndex = api.projectIndexing.triggerIndex.useMutation({
    onSuccess: () => {
      toast.success("Indexing started");
      void utils.projects.get.invalidate({ id: projectId });
      void utils.users.listMyProjects.invalidate();
      void utils.projectIndexing.listJobs.invalidate({ projectId });
    },
    onError: (error) => {
      toast.error(`Failed to start indexing: ${error.message}`);
    },
  });

  const updateProject = api.projects.update.useMutation({
    onSuccess: () => {
      toast.success("Project updated");
      void utils.projects.get.invalidate({ id: projectId });
      void utils.users.listMyProjects.invalidate();
    },
    onError: (error) => {
      toast.error(`Failed to update project: ${error.message}`);
    },
  });

  const deleteProject = api.projects.delete.useMutation({
    onSuccess: () => {
      toast.success("Project deleted");
      void utils.users.listMyProjects.invalidate();
      router.push("/project");
    },
    onError: (error) => {
      toast.error(`Failed to delete project: ${error.message}`);
    },
  });

  const putSSHKey = api.projects.putSSHKey.useMutation({
    onSuccess: () => {
      toast.success("SSH key updated");
      void utils.projects.getSSHKey.invalidate({ id: projectId });
    },
    onError: (error) => {
      toast.error(`Failed to update SSH key: ${error.message}`);
    },
  });

  const deleteSSHKey = api.projects.deleteSSHKey.useMutation({
    onSuccess: () => {
      toast.success("SSH key removed");
      void utils.projects.getSSHKey.invalidate({ id: projectId });
    },
    onError: (error) => {
      toast.error(`Failed to remove SSH key: ${error.message}`);
    },
  });

  return {
    triggerIndex,
    updateProject,
    deleteProject,
    putSSHKey,
    deleteSSHKey,
  };
}
