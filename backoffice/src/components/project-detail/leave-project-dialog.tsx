"use client";

import { useRouter } from "next/navigation";
import { AlertTriangle } from "lucide-react";
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
import { api } from "~/trpc/react";

interface LeaveProjectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
}

export function LeaveProjectDialog({
  open,
  onOpenChange,
  projectId,
}: LeaveProjectDialogProps) {
  const router = useRouter();
  const utils = api.useUtils();
  const meQuery = api.auth.me.useQuery(undefined, { enabled: open });

  const leaveMutation = api.projectMembers.remove.useMutation({
    onSuccess: () => {
      toast.success("You have left the project");
      void utils.users.listMyProjects.invalidate();
      onOpenChange(false);
      router.push("/project");
    },
    onError: (err) => {
      if (err.message?.includes("last owner") || err.data?.httpStatus === 409) {
        toast.error(
          "You are the last owner of this project. Transfer ownership before leaving.",
        );
      } else {
        toast.error("Failed to leave project. Please try again.");
      }
    },
  });

  const currentUserId = meQuery.data?.user?.id;

  function handleLeave() {
    if (!currentUserId) return;
    leaveMutation.mutate({ projectId, userId: currentUserId });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="text-destructive size-5" />
            Leave Project
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to leave this project? You will immediately
            lose access to all project resources. You can only rejoin if a
            project admin invites you back.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={leaveMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleLeave}
            disabled={!currentUserId || leaveMutation.isPending}
          >
            {leaveMutation.isPending ? "Leaving..." : "Leave Project"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
