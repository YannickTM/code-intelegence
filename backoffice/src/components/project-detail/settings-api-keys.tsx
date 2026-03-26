"use client";

import { useState } from "react";
import { AlertCircle, Key, Plus } from "lucide-react";
import { toast } from "sonner";
import { api } from "~/trpc/react";
import { Alert, AlertDescription } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
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
import type { ProjectRole } from "~/lib/dashboard-types";

interface ProjectApiKeysSectionProps {
  projectId: string;
  role: ProjectRole;
}

export function ProjectApiKeysSection({
  projectId,
  role,
}: ProjectApiKeysSectionProps) {
  const isReadOnly = role === "member";
  const utils = api.useUtils();
  const { data, isLoading, isError, error } =
    api.projectKeys.list.useQuery({ projectId });

  // Create dialog state
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResponseBase | null>(
    null,
  );

  // Delete dialog state
  const [keyToDelete, setKeyToDelete] = useState<APIKeyBase | null>(null);

  const createMutation = api.projectKeys.create.useMutation({
    onSuccess: (data) => {
      setCreatedKey(data);
      void utils.projectKeys.list.invalidate({ projectId });
    },
    onError: (err) => {
      const message =
        err.data?.code === "FORBIDDEN" || err.data?.code === "NOT_FOUND"
          ? "You need admin access to create API keys."
          : (err.message ?? "Failed to create API key. Please try again.");
      toast.error(message);
    },
  });

  const deleteMutation = api.projectKeys.delete.useMutation({
    onSuccess: () => {
      toast.success("API key revoked");
      setKeyToDelete(null);
      void utils.projectKeys.list.invalidate({ projectId });
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

  const keys = data?.items ?? [];

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle>API Keys</CardTitle>
            <CardDescription>
              {isReadOnly
                ? "View project API keys."
                : "Project-scoped keys for CI/CD and programmatic access."}
            </CardDescription>
          </div>
          {!isReadOnly && !isLoading && !isError && keys.length > 0 && (
            <Button onClick={() => setCreateDialogOpen(true)} size="sm">
              <Plus className="mr-2 h-4 w-4" />
              Create Key
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : isError ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              {error?.message ?? "Failed to load API keys."}
            </AlertDescription>
          </Alert>
        ) : keys.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-8">
            <Key className="text-muted-foreground mb-3 h-8 w-8" />
            <p className="text-muted-foreground mb-3 text-sm">
              No API keys for this project yet.
            </p>
            {!isReadOnly && (
            <Button
              onClick={() => setCreateDialogOpen(true)}
              size="sm"
              variant="outline"
            >
              <Plus className="mr-2 h-4 w-4" />
              Create Key
            </Button>
            )}
          </div>
        ) : (
          <ApiKeyTable keys={keys} onDelete={isReadOnly ? undefined : setKeyToDelete} />
        )}
      </CardContent>

      <CreateKeyDialog
        open={createDialogOpen}
        onOpenChange={handleCreateDialogChange}
        onSubmit={(values) =>
          createMutation.mutate({ projectId, ...values })
        }
        createdKey={createdKey}
        isPending={createMutation.isPending}
        dialogTitle="Create Project API Key"
        onDone={handleKeyDone}
      />

      <DeleteKeyDialog
        keyToDelete={keyToDelete}
        onClose={() => setKeyToDelete(null)}
        onConfirm={(keyId) => deleteMutation.mutate({ projectId, keyId })}
        isPending={deleteMutation.isPending}
        dialogDescription="Any integrations using this key will immediately lose access to this project."
      />
    </Card>
  );
}
