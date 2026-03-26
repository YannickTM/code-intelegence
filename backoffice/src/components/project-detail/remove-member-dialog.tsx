"use client";

import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import type { ProjectMember } from "~/server/api/routers/project-members";

interface RemoveMemberDialogProps {
  member: ProjectMember | null;
  onClose: () => void;
  onConfirm: (member: ProjectMember) => void;
  isPending: boolean;
}

export function RemoveMemberDialog({
  member,
  onClose,
  onConfirm,
  isPending,
}: RemoveMemberDialogProps) {
  return (
    <Dialog
      open={member !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Remove Member</DialogTitle>
          <DialogDescription>
            Are you sure you want to remove{" "}
            <span className="font-semibold text-foreground">
              {member?.username}
            </span>{" "}
            from this project? They will immediately lose access to all project
            resources.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={isPending}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => member && onConfirm(member)}
            disabled={isPending}
          >
            {isPending ? "Removing..." : "Remove"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
