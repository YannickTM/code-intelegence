"use client";

import { useState } from "react";
import { Check, Copy, ExternalLink } from "lucide-react";
import Link from "next/link";
import { toast } from "sonner";
import { api } from "~/trpc/react";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import { Label } from "~/components/ui/label";
import { Skeleton } from "~/components/ui/skeleton";
import type { SSHKey } from "./types";

interface KeyDetailDialogProps {
  sshKey: SSHKey | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRetire: (key: SSHKey) => void;
}

function StatusBadge({ isActive }: { isActive: boolean }) {
  if (isActive) {
    return (
      <Badge
        variant="outline"
        className="border-success/40 text-success"
      >
        Active
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="text-muted-foreground">
      Retired
    </Badge>
  );
}

export function KeyDetailDialog({
  sshKey,
  open,
  onOpenChange,
  onRetire,
}: KeyDetailDialogProps) {
  const [copied, setCopied] = useState(false);

  const {
    data: projectsData,
    isLoading: projectsLoading,
    isError: projectsError,
  } = api.sshKeys.getProjects.useQuery(
    { id: sshKey?.id ?? "" },
    { enabled: open && !!sshKey },
  );

  async function handleCopyPublicKey() {
    if (!sshKey) return;
    try {
      await navigator.clipboard.writeText(sshKey.public_key);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Failed to copy to clipboard");
    }
  }

  if (!sshKey) return null;

  const projects = projectsData?.items ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <DialogTitle>{sshKey.name}</DialogTitle>
            <StatusBadge isActive={sshKey.is_active} />
          </div>
        </DialogHeader>

        <div className="space-y-6">
          {/* Key Info */}
          <div className="space-y-3">
            <div className="space-y-1">
              <Label className="text-muted-foreground text-xs">
                Fingerprint
              </Label>
              <p className="font-mono text-sm break-all">
                {sshKey.fingerprint}
              </p>
            </div>

            <div className="space-y-1">
              <Label className="text-muted-foreground text-xs">
                Public Key
              </Label>
              <code className="bg-muted block rounded-md border p-3 font-mono text-sm break-all">
                {sshKey.public_key}
              </code>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1">
                <Label className="text-muted-foreground text-xs">
                  Key Type
                </Label>
                <p className="text-sm">{sshKey.key_type}</p>
              </div>
              <div className="space-y-1">
                <Label className="text-muted-foreground text-xs">
                  Created
                </Label>
                <p className="text-sm">
                  {new Date(sshKey.created_at).toLocaleString()}
                </p>
              </div>
            </div>

            {sshKey.rotated_at && (
              <div className="space-y-1">
                <Label className="text-muted-foreground text-xs">
                  Rotated
                </Label>
                <p className="text-sm">
                  {new Date(sshKey.rotated_at).toLocaleString()}
                </p>
              </div>
            )}
          </div>

          {/* Assigned Projects */}
          <div className="space-y-2">
            <Label className="text-muted-foreground text-xs">
              Assigned Projects
            </Label>
            {projectsLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
              </div>
            ) : projectsError ? (
              <p className="text-destructive text-sm">
                Failed to load assigned projects.
              </p>
            ) : projects.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                This key is not assigned to any projects.
              </p>
            ) : (
              <ul className="space-y-2">
                {projects.map((project) => (
                  <li
                    key={project.id}
                    className="flex items-center justify-between rounded-md border px-3 py-2"
                  >
                    <div className="min-w-0 flex-1">
                      <Link
                        href={`/project/${project.id}`}
                        className="text-sm font-medium hover:underline"
                        onClick={() => onOpenChange(false)}
                      >
                        {project.name}
                      </Link>
                      <p className="text-muted-foreground truncate text-xs">
                        {project.repo_url}
                      </p>
                    </div>
                    <div className="ml-2 flex items-center gap-2">
                      <Badge variant="outline" className="text-xs">
                        {project.status}
                      </Badge>
                      <Link
                        href={`/project/${project.id}`}
                        onClick={() => onOpenChange(false)}
                      >
                        <ExternalLink className="text-muted-foreground h-3.5 w-3.5" />
                      </Link>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => void handleCopyPublicKey()}
          >
            {copied ? (
              <>
                <Check className="mr-1 h-4 w-4" />
                Copied
              </>
            ) : (
              <>
                <Copy className="mr-1 h-4 w-4" />
                Copy Public Key
              </>
            )}
          </Button>
          {sshKey.is_active && (
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                onOpenChange(false);
                onRetire(sshKey);
              }}
            >
              Retire Key
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
