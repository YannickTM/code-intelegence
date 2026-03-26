"use client";

import { useCallback, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Loader2,
  MoreHorizontal,
  Pencil,
  Star,
  Trash2,
  XCircle,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import { Checkbox } from "~/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "~/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "~/components/ui/tooltip";
import { formatRelativeTime } from "~/lib/format";
import { isValidProviderEndpointURL } from "~/lib/provider-endpoint-url";
import type {
  EmbeddingProviderConfig,
  ProviderCapability,
  ProviderConfig,
} from "~/lib/provider-settings";
import { isEmbeddingProviderConfig } from "~/lib/provider-settings";

type EditFormState = {
  name: string;
  provider: string;
  endpoint_url: string;
  model: string;
  dimensions: string;
  max_tokens: string;
  is_available_to_projects: boolean;
};

function configToEditForm(
  config: ProviderConfig,
  capability: ProviderCapability,
): EditFormState {
  const base: EditFormState = {
    name: config.name,
    provider: config.provider,
    endpoint_url: config.endpoint_url,
    model: isEmbeddingProviderConfig(config) ? config.model : (config.model ?? ""),
    dimensions: "",
    max_tokens: "",
    is_available_to_projects: config.is_available_to_projects,
  };
  if (capability === "embedding" && isEmbeddingProviderConfig(config)) {
    base.dimensions = String(config.dimensions);
    base.max_tokens = String(config.max_tokens);
  }
  return base;
}

export type TestResult = {
  ok: boolean;
  message: string;
};

export type ProviderCardProps = {
  config: ProviderConfig;
  capability: ProviderCapability;
  supportedProviders: string[];
  onTest: () => void;
  onUpdate: (values: Record<string, unknown>) => Promise<void>;
  onDelete: () => void;
  onPromote: () => void;
  isTesting: boolean;
  isUpdating: boolean;
  isDeleting: boolean;
  isPromoting: boolean;
  testResult?: TestResult | null;
  onClearTestResult?: () => void;
};

export function ProviderCard({
  config,
  capability,
  supportedProviders,
  onTest,
  onUpdate,
  onDelete,
  onPromote,
  isTesting,
  isUpdating,
  isDeleting,
  isPromoting,
  testResult,
  onClearTestResult,
}: ProviderCardProps) {
  const [editing, setEditing] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [form, setForm] = useState<EditFormState>(() =>
    configToEditForm(config, capability),
  );

  const isDefault = config.is_default;
  const isEmbedding = capability === "embedding";
  const embeddingConfig = isEmbeddingProviderConfig(config)
    ? (config as EmbeddingProviderConfig)
    : null;

  const updateField = useCallback(
    <K extends keyof EditFormState>(key: K, value: EditFormState[K]) => {
      setForm((prev) => ({ ...prev, [key]: value }));
    },
    [],
  );

  const validate = (): string | null => {
    if (!form.name.trim()) return "Name is required";
    if (!form.provider) return "Provider is required";
    if (!form.endpoint_url.trim()) return "Endpoint URL is required";
    if (!isValidProviderEndpointURL(form.endpoint_url.trim()))
      return "Endpoint URL must be a valid http/https URL";
    if (!form.model.trim()) return "Model is required";
    if (isEmbedding) {
      const dims = Number(form.dimensions);
      if (
        !form.dimensions ||
        !Number.isInteger(dims) ||
        dims < 1 ||
        dims > 65536
      )
        return "Dimensions must be an integer between 1 and 65536";
      const maxTok = Number(form.max_tokens);
      if (
        !form.max_tokens ||
        !Number.isInteger(maxTok) ||
        maxTok < 1 ||
        maxTok > 131072
      )
        return "Max Tokens must be an integer between 1 and 131072";
    }
    return null;
  };

  const handleSave = async () => {
    const err = validate();
    if (err) return;
    const values: Record<string, unknown> = {
      name: form.name.trim(),
      provider: form.provider,
      endpoint_url: form.endpoint_url.trim(),
      model: form.model.trim(),
      is_available_to_projects: form.is_available_to_projects,
    };
    if (isEmbedding) {
      values.dimensions = Number(form.dimensions);
      values.max_tokens = Number(form.max_tokens);
    }
    try {
      await onUpdate(values);
      setEditing(false);
    } catch {
      // error toast handled by mutation onError; keep form open
    }
  };

  const handleStartEdit = () => {
    setForm(configToEditForm(config, capability));
    setEditing(true);
  };

  const handleCancelEdit = () => {
    setEditing(false);
  };

  return (
    <>
      <Card>
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <CardTitle className="text-base">{config.name}</CardTitle>
              {isDefault && (
                <Badge variant="default" className="bg-green-600 text-white">
                  Default
                </Badge>
              )}
              <Badge variant="outline">Active</Badge>
            </div>
            <div className="flex items-center gap-1">
              <Button
                variant="outline"
                size="sm"
                onClick={onTest}
                disabled={isTesting}
              >
                {isTesting && (
                  <Loader2 className="mr-1 size-3 animate-spin" />
                )}
                Test
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleStartEdit}
                disabled={editing}
              >
                <Pencil className="mr-1 size-3" />
                Edit
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="size-8 p-0">
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {!isDefault && (
                    <DropdownMenuItem
                      onClick={onPromote}
                      disabled={isPromoting}
                    >
                      <Star className="mr-2 size-4" />
                      Promote to Default
                    </DropdownMenuItem>
                  )}
                  {isDefault ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <div>
                          <DropdownMenuItem disabled>
                            <Trash2 className="mr-2 size-4" />
                            Delete
                          </DropdownMenuItem>
                        </div>
                      </TooltipTrigger>
                      <TooltipContent>
                        Promote another provider first
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <DropdownMenuItem
                      onClick={() => setDeleteDialogOpen(true)}
                      className="text-destructive focus:text-destructive"
                    >
                      <Trash2 className="mr-2 size-4" />
                      Delete
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Details */}
          <div className="text-muted-foreground space-y-1 text-sm">
            <p>
              <Badge variant="outline" className="mr-1">
                {config.provider}
              </Badge>
              {isEmbedding && embeddingConfig
                ? `${embeddingConfig.model} \u00b7 ${embeddingConfig.dimensions.toLocaleString()} dims \u00b7 ${embeddingConfig.max_tokens.toLocaleString()} max tokens`
                : config.model ?? ""}
            </p>
            <p className="truncate">{config.endpoint_url}</p>
            <p>
              Available to projects:{" "}
              {config.is_available_to_projects ? "Yes" : "No"}
              {" \u00b7 "}
              Credentials:{" "}
              {config.has_credentials ? "Configured" : "Not configured"}
            </p>
            <p title={new Date(config.updated_at).toLocaleString()}>
              Updated {formatRelativeTime(config.updated_at)}
            </p>
          </div>

          {/* Test Result */}
          {testResult && (
            <Alert variant={testResult.ok ? "default" : "destructive"}>
              {testResult.ok ? (
                <CheckCircle2 className="size-4 text-green-600" />
              ) : (
                <XCircle className="size-4" />
              )}
              <AlertTitle>
                {testResult.ok ? "Connection Successful" : "Connection Failed"}
              </AlertTitle>
              <AlertDescription>{testResult.message}</AlertDescription>
            </Alert>
          )}

          {/* Inline Edit Form */}
          {editing && (
            <div className="border-t pt-4 space-y-3">
              <div className="space-y-2">
                <Label>Name</Label>
                <Input
                  value={form.name}
                  onChange={(e) => updateField("name", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label>Provider</Label>
                <Select
                  value={form.provider}
                  onValueChange={(v) => updateField("provider", v)}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Select a provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {supportedProviders.map((p) => (
                      <SelectItem key={p} value={p}>
                        {p}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Endpoint URL</Label>
                <Input
                  type="url"
                  value={form.endpoint_url}
                  onChange={(e) => updateField("endpoint_url", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label>Model</Label>
                <Input
                  value={form.model}
                  onChange={(e) => updateField("model", e.target.value)}
                />
              </div>
              {isEmbedding && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>Dimensions</Label>
                    <Input
                      type="number"
                      min={1}
                      max={65536}
                      value={form.dimensions}
                      onChange={(e) =>
                        updateField("dimensions", e.target.value)
                      }
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Max Tokens</Label>
                    <Input
                      type="number"
                      min={1}
                      max={131072}
                      value={form.max_tokens}
                      onChange={(e) =>
                        updateField("max_tokens", e.target.value)
                      }
                    />
                  </div>
                </div>
              )}
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`avail-${config.id}`}
                  checked={form.is_available_to_projects}
                  onCheckedChange={(checked) =>
                    updateField("is_available_to_projects", checked === true)
                  }
                />
                <Label
                  htmlFor={`avail-${config.id}`}
                  className="cursor-pointer"
                >
                  Available to projects
                </Label>
              </div>
              <div className="flex gap-2 pt-1">
                <Button
                  size="sm"
                  onClick={handleSave}
                  disabled={isUpdating}
                >
                  {isUpdating && (
                    <Loader2 className="mr-1 size-3 animate-spin" />
                  )}
                  {isUpdating ? "Saving..." : "Save"}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={handleCancelEdit}
                  disabled={isUpdating}
                >
                  Cancel
                </Button>
              </div>
              {validate() && (
                <p className="text-destructive text-sm">
                  <AlertCircle className="mr-1 inline size-3" />
                  {validate()}
                </p>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onOpenChange={(open) => {
          if (!isDeleting) setDeleteDialogOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Provider</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{config.name}&quot;? This
              action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              disabled={isDeleting}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={onDelete}
              disabled={isDeleting}
            >
              {isDeleting && (
                <Loader2 className="mr-1 size-3 animate-spin" />
              )}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
