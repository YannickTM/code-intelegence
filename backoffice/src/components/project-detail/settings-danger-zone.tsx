"use client";

import { useState } from "react";
import { AlertTriangle } from "lucide-react";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { Separator } from "~/components/ui/separator";
import type { ProjectWithHealth } from "~/lib/dashboard-types";
import { useProjectDetailMutations } from "~/hooks/use-project-detail-mutations";
import { LeaveProjectDialog } from "./leave-project-dialog";

export function DangerZoneSection({ project }: { project: ProjectWithHealth }) {
  const { deleteProject } = useProjectDetailMutations(project.id);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [confirmName, setConfirmName] = useState("");
  const [leaveDialogOpen, setLeaveDialogOpen] = useState(false);

  const isOwner = project.role === "owner";
  const canDelete = isOwner && confirmName === project.name;

  function handleDelete() {
    if (!canDelete) return;
    deleteProject.mutate({ id: project.id });
  }

  return (
    <Card className="border-destructive/50">
      <CardHeader>
        <CardTitle className="text-destructive">Danger Zone</CardTitle>
        <CardDescription>
          Irreversible actions that permanently affect this project.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Leave Project — visible to all members */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">Leave this project</p>
            <p className="text-muted-foreground text-sm">
              Leave the project and lose access to all resources.
            </p>
          </div>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => setLeaveDialogOpen(true)}
          >
            Leave Project
          </Button>
        </div>

        {/* Delete Project — owner only */}
        {isOwner && (
          <>
            <Separator />
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Delete this project</p>
                <p className="text-muted-foreground text-sm">
                  Permanently delete the project and all associated data.
                </p>
              </div>
              <Dialog
                open={deleteOpen}
                onOpenChange={(newOpen) => {
                  setDeleteOpen(newOpen);
                  if (!newOpen) setConfirmName("");
                }}
              >
                <DialogTrigger asChild>
                  <Button variant="destructive" size="sm">
                    Delete Project
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle className="flex items-center gap-2">
                      <AlertTriangle className="text-destructive size-5" />
                      Delete Project
                    </DialogTitle>
                    <DialogDescription>
                      This action cannot be undone. This will permanently delete
                      the project <strong>{project.name}</strong> and all
                      associated data including index snapshots and jobs.
                    </DialogDescription>
                  </DialogHeader>
                  <div className="space-y-2">
                    <label htmlFor="delete-confirm-input" className="text-sm">
                      Type <strong>{project.name}</strong> to confirm:
                    </label>
                    <Input
                      id="delete-confirm-input"
                      value={confirmName}
                      onChange={(e) => setConfirmName(e.target.value)}
                      placeholder={project.name}
                    />
                  </div>
                  <DialogFooter>
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setDeleteOpen(false);
                        setConfirmName("");
                      }}
                    >
                      Cancel
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={handleDelete}
                      disabled={!canDelete || deleteProject.isPending}
                    >
                      {deleteProject.isPending
                        ? "Deleting..."
                        : "Delete Project"}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>
          </>
        )}
      </CardContent>

      {/* Leave Project Dialog */}
      <LeaveProjectDialog
        open={leaveDialogOpen}
        onOpenChange={setLeaveDialogOpen}
        projectId={project.id}
      />
    </Card>
  );
}
