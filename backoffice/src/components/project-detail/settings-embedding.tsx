"use client";

import { toast } from "sonner";
import { ProviderSettingsCard } from "./settings-provider-card";
import { api } from "~/trpc/react";

export function EmbeddingSettingsSection({
  projectId,
  role,
}: {
  projectId: string;
  role: string;
}) {
  const utils = api.useUtils();
  const supportedQuery = api.providers.listSupported.useQuery(undefined, {
    retry: false,
  });
  const settingQuery = api.projectEmbedding.get.useQuery(
    { projectId },
    {
      retry: false,
    },
  );
  const availableQuery = api.projectEmbedding.getAvailable.useQuery(
    { projectId },
    { retry: false },
  );
  const resolvedQuery = api.projectEmbedding.getResolved.useQuery(
    { projectId },
    { retry: false },
  );
  const resolvedNotFound = resolvedQuery.error?.data?.code === "NOT_FOUND";

  const saveMutation = api.projectEmbedding.put.useMutation({
    onSuccess: () => {
      toast.success("Embedding provider settings saved");
      void Promise.all([
        utils.projectEmbedding.get.invalidate({ projectId }),
        utils.projectEmbedding.getAvailable.invalidate({ projectId }),
        utils.projectEmbedding.getResolved.invalidate({ projectId }),
      ]);
    },
    onError: (error) => {
      toast.error(
        error.message ??
          "Failed to save embedding provider settings. Please try again.",
      );
    },
  });

  const resetMutation = api.projectEmbedding.delete.useMutation({
    onSuccess: () => {
      toast.success(
        "Embedding provider settings reset to active global default",
      );
      void Promise.all([
        utils.projectEmbedding.get.invalidate({ projectId }),
        utils.projectEmbedding.getAvailable.invalidate({ projectId }),
        utils.projectEmbedding.getResolved.invalidate({ projectId }),
      ]);
    },
    onError: (error) => {
      toast.error(
        error.message ??
          "Failed to reset embedding provider settings. Please try again.",
      );
    },
  });

  const testMutation = api.projectEmbedding.test.useMutation();

  return (
    <ProviderSettingsCard
      capability="embedding"
      title="Embedding Provider"
      role={role}
      data={{
        supportedProviders: supportedQuery.data?.embedding,
        availableItems: availableQuery.data?.items,
        setting: settingQuery.data,
        resolved: resolvedNotFound ? undefined : resolvedQuery.data,
        isLoading:
          supportedQuery.isLoading ||
          availableQuery.isLoading ||
          settingQuery.isLoading ||
          resolvedQuery.isLoading,
        isError:
          supportedQuery.isError ||
          availableQuery.isError ||
          settingQuery.isError ||
          (resolvedQuery.isError && !resolvedNotFound),
        retry: () => {
          void Promise.all([
            supportedQuery.refetch(),
            availableQuery.refetch(),
            settingQuery.refetch(),
            resolvedQuery.refetch(),
          ]);
        },
      }}
      actions={{
        onSave: async (payload) => {
          if (payload.mode === "global") {
            await saveMutation.mutateAsync({
              projectId,
              mode: "global",
              global_config_id: payload.global_config_id,
            });
            return;
          }

          if (
            !payload.model ||
            typeof payload.dimensions !== "number" ||
            typeof payload.max_tokens !== "number"
          ) {
            toast.error(
              "Missing model, dimensions, or max tokens for custom embedding",
            );
            throw new Error(
              "Missing model, dimensions, or max tokens for custom embedding",
            );
          }

          await saveMutation.mutateAsync({
            projectId,
            mode: "custom",
            name: payload.name,
            provider: payload.provider,
            endpoint_url: payload.endpoint_url,
            model: payload.model,
            dimensions: payload.dimensions,
            max_tokens: payload.max_tokens,
          });
        },
        onReset: async () => {
          await resetMutation.mutateAsync({ projectId });
        },
        onTest: async (payload) =>
          testMutation.mutateAsync({
            projectId,
            provider: payload.provider,
            endpoint_url: payload.endpoint_url,
            model: payload.model,
            dimensions: payload.dimensions,
          }),
        isSaving: saveMutation.isPending,
        isResetting: resetMutation.isPending,
        isTesting: testMutation.isPending,
      }}
    />
  );
}
