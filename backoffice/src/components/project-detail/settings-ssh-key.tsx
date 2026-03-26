"use client";

import { useState } from "react";
import { AlertCircle, Copy, KeyRound, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import { Skeleton } from "~/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "~/components/ui/dialog";
import { api } from "~/trpc/react";
import { useProjectDetailMutations } from "~/hooks/use-project-detail-mutations";

export function SSHKeySettingsSection({ projectId }: { projectId: string }) {
  const sshKeyQuery = api.projects.getSSHKey.useQuery(
    { id: projectId },
    { retry: false },
  );
  const keysQuery = api.sshKeys.list.useQuery(undefined, {
    retry: false,
  });
  const { putSSHKey, deleteSSHKey } = useProjectDetailMutations(projectId);

  const [selectedKeyId, setSelectedKeyId] = useState("");
  const [generateName, setGenerateName] = useState("");
  const [removeOpen, setRemoveOpen] = useState(false);

  function handleReassign() {
    if (!selectedKeyId) return;
    putSSHKey.mutate(
      { id: projectId, ssh_key_id: selectedKeyId },
      {
        onSuccess: () => setSelectedKeyId(""),
      },
    );
  }

  function handleGenerate() {
    if (!generateName.trim()) return;
    putSSHKey.mutate(
      { id: projectId, generate: true, name: generateName.trim() },
      {
        onSuccess: () => setGenerateName(""),
      },
    );
  }

  function handleRemove() {
    deleteSSHKey.mutate(
      { id: projectId },
      {
        onSuccess: () => setRemoveOpen(false),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>SSH Key</CardTitle>
        <CardDescription>
          Manage the deploy key used to clone the repository.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {sshKeyQuery.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
        ) : sshKeyQuery.isError ? (
          <div className="text-destructive flex items-center gap-2 text-sm">
            <AlertCircle className="size-4" />
            <span>Failed to load SSH key</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void sshKeyQuery.refetch()}
            >
              Retry
            </Button>
          </div>
        ) : sshKeyQuery.data ? (
          <>
            {/* Current key */}
            <div className="space-y-2 rounded-lg border p-3 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Name</span>
                <span className="font-medium">{sshKeyQuery.data.name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Type</span>
                <span>{sshKeyQuery.data.key_type}</span>
              </div>
              <div className="flex items-center justify-between gap-2">
                <span className="text-muted-foreground">Fingerprint</span>
                <code className="max-w-[240px] truncate font-mono text-xs">
                  {sshKeyQuery.data.fingerprint}
                </code>
              </div>
              <div>
                <label className="text-muted-foreground mb-1 block">
                  Public Key
                </label>
                <div className="flex items-start gap-2">
                  <code className="bg-muted flex-1 rounded-md p-2 font-mono text-xs break-all">
                    {sshKeyQuery.data.public_key}
                  </code>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8 shrink-0"
                    aria-label="Copy public key"
                    onClick={() => {
                      void navigator.clipboard.writeText(
                        sshKeyQuery.data!.public_key,
                      );
                      toast.success("Public key copied");
                    }}
                  >
                    <Copy className="size-3.5" />
                  </Button>
                </div>
              </div>
            </div>

            {/* Remove */}
            <Dialog open={removeOpen} onOpenChange={setRemoveOpen}>
              <DialogTrigger asChild>
                <Button variant="outline" size="sm">
                  <Trash2 className="size-4" />
                  Remove Key
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Remove SSH Key</DialogTitle>
                  <DialogDescription>
                    This will unassign the deploy key from the project. The
                    project will not be able to clone the repository until a new
                    key is assigned.
                  </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                  <Button variant="ghost" onClick={() => setRemoveOpen(false)}>
                    Cancel
                  </Button>
                  <Button
                    variant="destructive"
                    onClick={handleRemove}
                    disabled={deleteSSHKey.isPending}
                  >
                    {deleteSSHKey.isPending ? "Removing..." : "Remove"}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </>
        ) : (
          <div className="text-muted-foreground flex items-center gap-2 text-sm">
            <KeyRound className="size-4" />
            No SSH key assigned
          </div>
        )}

        {/* Reassign from library */}
        <div className="space-y-2 border-t pt-4">
          <Label>Reassign from key library</Label>
          <div className="flex gap-2">
            <Select value={selectedKeyId} onValueChange={setSelectedKeyId}>
              <SelectTrigger className="flex-1">
                <SelectValue placeholder="Select a key..." />
              </SelectTrigger>
              <SelectContent>
                {keysQuery.data?.items?.map((key) => (
                  <SelectItem key={key.id} value={key.id}>
                    {key.name} ({key.fingerprint.slice(0, 16)}...)
                  </SelectItem>
                ))}
                {(!keysQuery.data?.items ||
                  keysQuery.data.items.length === 0) && (
                  <SelectItem value="_none" disabled>
                    No keys available
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              onClick={handleReassign}
              disabled={!selectedKeyId || putSSHKey.isPending}
            >
              Assign
            </Button>
          </div>
        </div>

        {/* Generate new key */}
        <div className="space-y-2 border-t pt-4">
          <Label>Generate and assign a new key</Label>
          <div className="flex gap-2">
            <Input
              placeholder="Key name"
              value={generateName}
              onChange={(e) => setGenerateName(e.target.value)}
              className="flex-1"
            />
            <Button
              size="sm"
              onClick={handleGenerate}
              disabled={!generateName.trim() || putSSHKey.isPending}
            >
              {putSSHKey.isPending ? "Generating..." : "Generate"}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
