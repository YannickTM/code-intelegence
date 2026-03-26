"use client";

import { useCallback, useState } from "react";
import { toast } from "sonner";
import { AlertCircle, Loader2, Plus, X } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Checkbox } from "~/components/ui/checkbox";
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
import { isValidProviderEndpointURL } from "~/lib/provider-endpoint-url";
import { api } from "~/trpc/react";
import {
  ProviderCard,
  type TestResult,
} from "./provider-card";

type AddFormState = {
  name: string;
  provider: string;
  endpoint_url: string;
  model: string;
  dimensions: string;
  max_tokens: string;
  is_available_to_projects: boolean;
};

const emptyAddForm: AddFormState = {
  name: "",
  provider: "",
  endpoint_url: "",
  model: "",
  dimensions: "",
  max_tokens: "",
  is_available_to_projects: true,
};

function validateAddForm(form: AddFormState): string | null {
  if (!form.name.trim()) return "Name is required";
  if (!form.provider) return "Provider is required";
  if (!form.endpoint_url.trim()) return "Endpoint URL is required";
  if (!isValidProviderEndpointURL(form.endpoint_url.trim()))
    return "Endpoint URL must be a valid http/https URL";
  if (!form.model.trim()) return "Model is required";
  const dims = Number(form.dimensions);
  if (!form.dimensions || !Number.isInteger(dims) || dims < 1 || dims > 65536)
    return "Dimensions must be an integer between 1 and 65536";
  const maxTok = Number(form.max_tokens);
  if (
    !form.max_tokens ||
    !Number.isInteger(maxTok) ||
    maxTok < 1 ||
    maxTok > 131072
  )
    return "Max Tokens must be an integer between 1 and 131072";
  return null;
}

export function EmbeddingConfigForm() {
  const utils = api.useUtils();

  // Queries
  const configsQuery = api.platformEmbedding.list.useQuery(undefined, {
    retry: false,
  });
  const supportedQuery = api.providers.listSupported.useQuery(undefined, {
    retry: false,
  });

  // Add form state
  const [showAddForm, setShowAddForm] = useState(false);
  const [addForm, setAddForm] = useState<AddFormState>(emptyAddForm);

  // Per-config test results
  const [testResults, setTestResults] = useState<
    Record<string, TestResult | null>
  >({});

  const updateAddField = useCallback(
    <K extends keyof AddFormState>(key: K, value: AddFormState[K]) => {
      setAddForm((prev) => ({ ...prev, [key]: value }));
    },
    [],
  );

  // Mutations
  const createMutation = api.platformEmbedding.create.useMutation({
    onSuccess: () => {
      toast.success("Provider created");
      setShowAddForm(false);
      setAddForm(emptyAddForm);
      void utils.platformEmbedding.list.invalidate();
    },
    onError: (error) => {
      toast.error(error.message ?? "Failed to create provider");
    },
  });

  const updateByIdMutation = api.platformEmbedding.updateById.useMutation({
    onSuccess: () => {
      toast.success("Provider updated");
      void utils.platformEmbedding.list.invalidate();
    },
    onError: (error) => {
      toast.error(error.message ?? "Failed to update provider");
    },
  });

  const deleteByIdMutation = api.platformEmbedding.deleteById.useMutation({
    onSuccess: () => {
      toast.success("Provider deleted");
      void utils.platformEmbedding.list.invalidate();
    },
    onError: (error) => {
      toast.error(error.message ?? "Failed to delete provider");
    },
  });

  const promoteMutation = api.platformEmbedding.promote.useMutation({
    onSuccess: () => {
      toast.success("Provider promoted to default");
      void utils.platformEmbedding.list.invalidate();
    },
    onError: (error) => {
      toast.error(error.message ?? "Failed to promote provider");
    },
  });

  const testByIdMutation = api.platformEmbedding.testById.useMutation();

  // Handlers
  const handleCreate = () => {
    const err = validateAddForm(addForm);
    if (err) {
      toast.error(err);
      return;
    }
    createMutation.mutate({
      name: addForm.name.trim(),
      provider: addForm.provider,
      endpoint_url: addForm.endpoint_url.trim(),
      model: addForm.model.trim(),
      dimensions: Number(addForm.dimensions),
      max_tokens: Number(addForm.max_tokens),
      is_available_to_projects: addForm.is_available_to_projects,
    });
  };

  const handleTest = (configId: string) => {
    setTestResults((prev) => ({ ...prev, [configId]: null }));
    testByIdMutation.mutate(
      { configId },
      {
        onSuccess: (data) => {
          setTestResults((prev) => ({
            ...prev,
            [configId]: { ok: data.ok, message: data.message },
          }));
        },
        onError: (error) => {
          setTestResults((prev) => ({
            ...prev,
            [configId]: {
              ok: false,
              message: error.message ?? "Connectivity test failed",
            },
          }));
        },
      },
    );
  };

  const handleUpdate = async (
    configId: string,
    values: Record<string, unknown>,
  ) => {
    await updateByIdMutation.mutateAsync({
      configId,
      ...values,
    } as Parameters<typeof updateByIdMutation.mutateAsync>[0]);
  };

  const handleDelete = (configId: string) => {
    deleteByIdMutation.mutate({ configId });
  };

  const handlePromote = (configId: string) => {
    promoteMutation.mutate({ configId });
  };

  // Loading / Error
  const isLoading = configsQuery.isLoading || supportedQuery.isLoading;
  const isError = configsQuery.isError || supportedQuery.isError;

  if (isLoading) {
    return (
      <div className="flex flex-col gap-6">
        <div className="flex items-center justify-between">
          <div className="flex flex-col gap-1">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-96" />
          </div>
          <Skeleton className="h-9 w-32" />
        </div>
        {Array.from({ length: 2 }).map((_, i) => (
          <Card key={i}>
            <CardHeader className="pb-3">
              <div className="flex items-center gap-2">
                <Skeleton className="h-5 w-40" />
                <Skeleton className="h-5 w-16" />
              </div>
            </CardHeader>
            <CardContent className="space-y-2">
              <Skeleton className="h-4 w-72" />
              <Skeleton className="h-4 w-96" />
              <Skeleton className="h-4 w-64" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <div className="flex flex-col gap-6">
        <div className="flex flex-col gap-1">
          <h2 className="text-lg font-semibold">Embedding Provider</h2>
          <p className="text-muted-foreground text-sm">
            Manage platform-wide embedding provider configurations.
          </p>
        </div>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription className="flex items-center gap-2">
            Failed to load embedding provider settings.
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void configsQuery.refetch();
                void supportedQuery.refetch();
              }}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  const providers = supportedQuery.data?.embedding ?? [];
  const activeConfigs = (configsQuery.data?.items ?? [])
    .filter((c) => c.is_active)
    .sort((a, b) => {
      if (a.is_default && !b.is_default) return -1;
      if (!a.is_default && b.is_default) return 1;
      return (
        new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
      );
    });

  return (
    <div className="flex flex-col gap-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex flex-col gap-1">
          <h2 className="text-lg font-semibold">Embedding Provider</h2>
          <p className="text-muted-foreground text-sm">
            Manage platform-wide embedding provider configurations.
          </p>
        </div>
        <Button
          onClick={() => setShowAddForm(true)}
          disabled={showAddForm}
        >
          <Plus className="mr-1 size-4" />
          Add Provider
        </Button>
      </div>

      {/* Add Provider Form */}
      {showAddForm && (
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-base">Add Provider</CardTitle>
              <Button
                variant="ghost"
                size="sm"
                className="size-8 p-0"
                onClick={() => {
                  setShowAddForm(false);
                  setAddForm(emptyAddForm);
                }}
              >
                <X className="size-4" />
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="add-name">Name</Label>
              <Input
                id="add-name"
                value={addForm.name}
                onChange={(e) => updateAddField("name", e.target.value)}
                placeholder="e.g. Production Embedding"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="add-provider">Provider</Label>
              <Select
                value={addForm.provider}
                onValueChange={(v) => updateAddField("provider", v)}
              >
                <SelectTrigger id="add-provider" className="w-full">
                  <SelectValue placeholder="Select a provider" />
                </SelectTrigger>
                <SelectContent>
                  {providers.map((p) => (
                    <SelectItem key={p} value={p}>
                      {p}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="add-endpoint">Endpoint URL</Label>
              <Input
                id="add-endpoint"
                type="url"
                value={addForm.endpoint_url}
                onChange={(e) => updateAddField("endpoint_url", e.target.value)}
                placeholder="https://api.example.com/v1/embeddings"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="add-model">Model</Label>
              <Input
                id="add-model"
                value={addForm.model}
                onChange={(e) => updateAddField("model", e.target.value)}
                placeholder="e.g. text-embedding-3-small"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="add-dimensions">Dimensions</Label>
                <Input
                  id="add-dimensions"
                  type="number"
                  min={1}
                  max={65536}
                  value={addForm.dimensions}
                  onChange={(e) =>
                    updateAddField("dimensions", e.target.value)
                  }
                  placeholder="e.g. 1536"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="add-max-tokens">Max Tokens</Label>
                <Input
                  id="add-max-tokens"
                  type="number"
                  min={1}
                  max={131072}
                  value={addForm.max_tokens}
                  onChange={(e) =>
                    updateAddField("max_tokens", e.target.value)
                  }
                  placeholder="e.g. 8192"
                />
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="add-available"
                checked={addForm.is_available_to_projects}
                onCheckedChange={(checked) =>
                  updateAddField("is_available_to_projects", checked === true)
                }
              />
              <Label htmlFor="add-available" className="cursor-pointer">
                Available to projects
              </Label>
            </div>
            <div className="flex gap-2 pt-1">
              <Button
                onClick={handleCreate}
                disabled={createMutation.isPending}
              >
                {createMutation.isPending && (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                )}
                {createMutation.isPending ? "Creating..." : "Create Provider"}
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setShowAddForm(false);
                  setAddForm(emptyAddForm);
                }}
                disabled={createMutation.isPending}
              >
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Provider Cards */}
      {activeConfigs.length === 0 && !showAddForm && (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-muted-foreground text-sm">
              No embedding providers configured yet. Add one to get started.
            </p>
          </CardContent>
        </Card>
      )}

      {activeConfigs.map((config) => (
        <ProviderCard
          key={config.id}
          config={config}
          capability="embedding"
          supportedProviders={providers}
          onTest={() => handleTest(config.id)}
          onUpdate={(values) => handleUpdate(config.id, values)}
          onDelete={() => handleDelete(config.id)}
          onPromote={() => handlePromote(config.id)}
          isTesting={
            testByIdMutation.isPending &&
            testByIdMutation.variables?.configId === config.id
          }
          isUpdating={
            updateByIdMutation.isPending &&
            updateByIdMutation.variables?.configId === config.id
          }
          isDeleting={
            deleteByIdMutation.isPending &&
            deleteByIdMutation.variables?.configId === config.id
          }
          isPromoting={
            promoteMutation.isPending &&
            promoteMutation.variables?.configId === config.id
          }
          testResult={testResults[config.id]}
        />
      ))}
    </div>
  );
}
