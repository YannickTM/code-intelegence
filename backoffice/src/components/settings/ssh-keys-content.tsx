"use client";

import { useState } from "react";
import { AlertCircle, KeyRound, Plus } from "lucide-react";
import { toast } from "sonner";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import {
  SSHKeyTable,
  CreateKeyDialog,
  KeyDetailDialog,
  RetireKeyDialog,
} from "~/components/ssh-keys";
import type { SSHKey } from "~/components/ssh-keys";

export function SSHKeysContent() {
  const utils = api.useUtils();
  const { data, isLoading, isError, error } = api.sshKeys.list.useQuery();

  // Dialog state
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [createdKey, setCreatedKey] = useState<SSHKey | null>(null);
  const [detailKey, setDetailKey] = useState<SSHKey | null>(null);
  const [keyToRetire, setKeyToRetire] = useState<SSHKey | null>(null);

  const createMutation = api.sshKeys.create.useMutation({
    onSuccess: (data) => {
      setCreatedKey(data);
      void utils.sshKeys.list.invalidate();
    },
    onError: (err) => {
      toast.error(err.message ?? "Failed to create SSH key.");
    },
  });

  const retireMutation = api.sshKeys.retire.useMutation({
    onSuccess: () => {
      toast.success("SSH key retired");
      setKeyToRetire(null);
      void utils.sshKeys.list.invalidate();
    },
    onError: (err) => {
      toast.error(err.message ?? "Failed to retire SSH key.");
    },
  });

  function handleCreateDialogChange(open: boolean) {
    setCreateDialogOpen(open);
    if (!open) {
      setCreatedKey(null);
      createMutation.reset();
    }
  }

  function handleKeyDone() {
    setCreateDialogOpen(false);
    setCreatedKey(null);
    createMutation.reset();
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold">SSH Keys</h2>
            <p className="text-muted-foreground text-sm">
              Manage deploy keys for connecting repositories.
            </p>
          </div>
          <Skeleton className="h-9 w-[100px]" />
        </div>
        <div className="rounded-md border">
          <div className="space-y-3 p-4">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="flex flex-col gap-4">
        <h2 className="text-lg font-semibold">SSH Keys</h2>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            {error?.message ?? "Failed to load SSH keys."}
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  const keys = data?.items ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">SSH Keys</h2>
          <p className="text-muted-foreground text-sm">
            Manage deploy keys for connecting repositories.
          </p>
        </div>
        <Button onClick={() => setCreateDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Add Key
        </Button>
      </div>

      {keys.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
          <KeyRound className="text-muted-foreground mb-4 h-10 w-10" />
          <h3 className="text-lg font-semibold">No SSH keys yet</h3>
          <p className="text-muted-foreground mb-4 text-sm">
            Create a deploy key to connect your repositories.
          </p>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Key
          </Button>
        </div>
      ) : (
        <SSHKeyTable
          keys={keys}
          onViewDetails={setDetailKey}
          onRetire={setKeyToRetire}
        />
      )}

      <CreateKeyDialog
        open={createDialogOpen}
        onOpenChange={handleCreateDialogChange}
        onSubmit={(values) => createMutation.mutate(values)}
        createdKey={createdKey}
        isPending={createMutation.isPending}
        onDone={handleKeyDone}
      />

      <KeyDetailDialog
        sshKey={detailKey}
        open={detailKey !== null}
        onOpenChange={(open) => {
          if (!open) setDetailKey(null);
        }}
        onRetire={(key) => {
          setDetailKey(null);
          setKeyToRetire(key);
        }}
      />

      <RetireKeyDialog
        sshKey={keyToRetire}
        onClose={() => setKeyToRetire(null)}
        onConfirm={(id) => retireMutation.mutate({ id })}
        isPending={retireMutation.isPending}
      />
    </div>
  );
}
