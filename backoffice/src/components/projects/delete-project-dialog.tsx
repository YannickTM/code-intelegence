"use client";

import { useEffect, useRef } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import type { UserProject } from "~/server/api/routers/users";
import { api } from "~/trpc/react";

export function DeleteProjectDialog({
  project,
  onClose,
}: {
  project: UserProject | null;
  onClose: () => void;
}) {
  const utils = api.useUtils();
  const deletingNameRef = useRef<string>("");

  const deleteProject = api.projects.delete.useMutation({
    onSuccess: () => {
      toast.success(`Project "${deletingNameRef.current}" deleted`);
      void utils.users.listMyProjects.invalidate();
      onClose();
    },
    onError: (error) => {
      toast.error(`Failed to delete project: ${error.message}`);
    },
  });

  useEffect(() => {
    if (!project) {
      deleteProject.reset();
    }
  }, [project]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <Dialog
      open={project !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Delete Project</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete &ldquo;{project?.name}&rdquo;? This
            action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => {
              if (project) {
                deletingNameRef.current = project.name;
                deleteProject.mutate({ id: project.id });
              }
            }}
            disabled={deleteProject.isPending}
          >
            {deleteProject.isPending && (
              <Loader2 className="size-4 animate-spin" />
            )}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
