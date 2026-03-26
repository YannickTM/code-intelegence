"use client";

import { AlertTriangle, Loader2 } from "lucide-react";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import { Skeleton } from "~/components/ui/skeleton";
import type { SSHKey } from "./types";

interface RetireKeyDialogProps {
  sshKey: SSHKey | null;
  onClose: () => void;
  onConfirm: (id: string) => void;
  isPending: boolean;
}

export function RetireKeyDialog({
  sshKey,
  onClose,
  onConfirm,
  isPending,
}: RetireKeyDialogProps) {
  const open = sshKey !== null;

  const {
    data: projectsData,
    isLoading: projectsLoading,
    isError: projectsError,
  } = api.sshKeys.getProjects.useQuery(
    { id: sshKey?.id ?? "" },
    { enabled: open && !!sshKey },
  );

  const projects = projectsData?.items ?? [];
  const hasProjects = !projectsLoading && projects.length > 0;
  const canRetire = !projectsLoading && !projectsError && !hasProjects;

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Retire SSH Key</DialogTitle>
          {canRetire && (
            <DialogDescription>
              Are you sure you want to retire{" "}
              <span className="font-semibold">{sshKey?.name}</span>? Retired
              keys can no longer be assigned to new projects.
            </DialogDescription>
          )}
        </DialogHeader>

        {projectsLoading ? (
          <div className="space-y-2 py-2">
            <Skeleton className="h-5 w-full" />
            <Skeleton className="h-5 w-3/4" />
          </div>
        ) : projectsError ? (
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              Failed to check project assignments. Cannot retire key safely.
            </AlertDescription>
          </Alert>
        ) : hasProjects ? (
          <div className="space-y-3">
            <Alert className="border-amber-500/40 bg-amber-50 text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
              <AlertTriangle className="h-4 w-4" />
              <AlertDescription>
                This key is currently assigned to {projects.length} project(s).
                Reassign or remove the key from these projects before retiring.
              </AlertDescription>
            </Alert>
            <ul className="space-y-1 text-sm">
              {projects.map((p) => (
                <li key={p.id} className="text-muted-foreground">
                  &bull; {p.name}
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        <DialogFooter>
          {canRetire ? (
            <>
              <Button variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => sshKey && onConfirm(sshKey.id)}
                disabled={isPending || projectsLoading}
              >
                {isPending && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                )}
                Retire
              </Button>
            </>
          ) : (
            <Button variant="outline" onClick={onClose}>
              Close
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
