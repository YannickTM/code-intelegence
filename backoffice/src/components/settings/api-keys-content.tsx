"use client";

import { useState } from "react";
import { AlertCircle, Key, Plus } from "lucide-react";
import { toast } from "sonner";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import { Skeleton } from "~/components/ui/skeleton";
import {
  ApiKeyTable,
  CreateKeyDialog,
  DeleteKeyDialog,
} from "~/components/api-keys";
import type {
  APIKeyBase,
  CreateAPIKeyResponseBase,
} from "~/components/api-keys";

export function APIKeysContent() {
  const utils = api.useUtils();
  const { data, isLoading, isError, error } = api.users.listMyKeys.useQuery();

  // Create dialog state
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResponseBase | null>(
    null,
  );

  // Delete dialog state
  const [keyToDelete, setKeyToDelete] = useState<APIKeyBase | null>(null);

  const createMutation = api.users.createMyKey.useMutation({
    onSuccess: (data) => {
      setCreatedKey(data);
      void utils.users.listMyKeys.invalidate();
    },
    onError: (err) => {
      toast.error(err.message ?? "Failed to create API key. Please try again.");
    },
  });

  const deleteMutation = api.users.deleteMyKey.useMutation({
    onSuccess: () => {
      toast.success("API key revoked");
      setKeyToDelete(null);
      void utils.users.listMyKeys.invalidate();
    },
    onError: (err) => {
      toast.error(
        err.message ?? "Failed to revoke API key. Please try again.",
      );
    },
  });

  function handleCreateDialogChange(open: boolean) {
    setCreateDialogOpen(open);
    if (!open && !createdKey) {
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
            <h2 className="text-lg font-semibold">Personal API Keys</h2>
            <p className="text-muted-foreground text-sm">
              API keys for programmatic access. A personal key inherits your
              project memberships.
            </p>
          </div>
          <Skeleton className="h-9 w-[110px]" />
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
        <h2 className="text-lg font-semibold">Personal API Keys</h2>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            {error?.message ?? "Failed to load API keys."}
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
          <h2 className="text-lg font-semibold">Personal API Keys</h2>
          <p className="text-muted-foreground text-sm">
            API keys for programmatic access. A personal key inherits your
            project memberships.
          </p>
        </div>
        <Button onClick={() => setCreateDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Create Key
        </Button>
      </div>

      {keys.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center rounded-xl border border-dashed p-12">
          <Key className="text-muted-foreground mb-4 h-10 w-10" />
          <h3 className="text-lg font-semibold">No API keys yet</h3>
          <p className="text-muted-foreground mb-4 text-sm">
            Create a personal API key for programmatic access to the MYJUNGLE
            API.
          </p>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Create Key
          </Button>
        </div>
      ) : (
        <ApiKeyTable keys={keys} onDelete={setKeyToDelete} />
      )}

      <CreateKeyDialog
        open={createDialogOpen}
        onOpenChange={handleCreateDialogChange}
        onSubmit={(values) => createMutation.mutate(values)}
        createdKey={createdKey}
        isPending={createMutation.isPending}
        onDone={handleKeyDone}
      />

      <DeleteKeyDialog
        keyToDelete={keyToDelete}
        onClose={() => setKeyToDelete(null)}
        onConfirm={(keyId) => deleteMutation.mutate({ keyId })}
        isPending={deleteMutation.isPending}
      />
    </div>
  );
}
