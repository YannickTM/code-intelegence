"use client";

import { toast } from "sonner";
import { ProviderSettingsCard } from "./settings-provider-card";
import { api } from "~/trpc/react";

export function LLMSettingsSection({
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
  const settingQuery = api.projectLLM.get.useQuery(
    { projectId },
    {
      retry: false,
    },
  );
  const availableQuery = api.projectLLM.getAvailable.useQuery(
    { projectId },
    { retry: false },
  );
  const resolvedQuery = api.projectLLM.getResolved.useQuery(
    { projectId },
    { retry: false },
  );
  const resolvedNotFound = resolvedQuery.error?.data?.code === "NOT_FOUND";

  const saveMutation = api.projectLLM.put.useMutation({
    onSuccess: () => {
      toast.success("LLM provider settings saved");
      void Promise.all([
        utils.projectLLM.get.invalidate({ projectId }),
        utils.projectLLM.getAvailable.invalidate({ projectId }),
        utils.projectLLM.getResolved.invalidate({ projectId }),
      ]);
    },
    onError: (error) => {
      toast.error(
        error.message ??
          "Failed to save LLM provider settings. Please try again.",
      );
    },
  });

  const resetMutation = api.projectLLM.delete.useMutation({
    onSuccess: () => {
      toast.success("LLM provider settings reset to active global default");
      void Promise.all([
        utils.projectLLM.get.invalidate({ projectId }),
        utils.projectLLM.getAvailable.invalidate({ projectId }),
        utils.projectLLM.getResolved.invalidate({ projectId }),
      ]);
    },
    onError: (error) => {
      toast.error(
        error.message ??
          "Failed to reset LLM provider settings. Please try again.",
      );
    },
  });

  const testMutation = api.projectLLM.test.useMutation();

  return (
    <ProviderSettingsCard
      capability="llm"
      title="LLM Provider"
      role={role}
      data={{
        supportedProviders: supportedQuery.data?.llm,
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

          await saveMutation.mutateAsync({
            projectId,
            mode: "custom",
            name: payload.name,
            provider: payload.provider,
            endpoint_url: payload.endpoint_url,
            ...(payload.model ? { model: payload.model } : {}),
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
            ...(payload.model ? { model: payload.model } : {}),
          }),
        isSaving: saveMutation.isPending,
        isResetting: resetMutation.isPending,
        isTesting: testMutation.isPending,
      }}
    />
  );
}
