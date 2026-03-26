"use client";

import { useEffect, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, RotateCcw, XCircle } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "~/components/ui/dialog";
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
import type {
  ConnectivityTestResponse,
  ProjectProviderMode,
  ProjectProviderSetting,
  ProviderCapability,
  ProviderConfig,
  ResolvedProjectProvider,
} from "~/lib/provider-settings";
import { isValidProviderEndpointURL } from "~/lib/provider-endpoint-url";
import { cn } from "~/lib/utils";

const PROVIDER_LABELS: Record<string, string> = {
  aws: "AWS",
  azure: "Azure",
  claude: "Claude",
  google: "Google",
  huggingface: "Hugging Face",
  ollama: "Ollama",
  openai: "OpenAI",
  openrouter: "OpenRouter",
};

type CustomDraft = {
  name: string;
  provider: string;
  endpoint_url: string;
  model: string;
  dimensions: string;
  max_tokens: string;
};

type ProviderSavePayload =
  | { mode: "global"; global_config_id: string }
  | {
      mode: "custom";
      name: string;
      provider: string;
      endpoint_url: string;
      model?: string;
      dimensions?: number;
      max_tokens?: number;
    };

type ProviderTestPayload = {
  provider: string;
  endpoint_url: string;
  model?: string;
  dimensions?: number;
};

type ProviderCardData = {
  supportedProviders?: string[];
  availableItems?: ProviderConfig[];
  setting?: ProjectProviderSetting<ProviderConfig>;
  resolved?: ResolvedProjectProvider<ProviderConfig> | null;
  isLoading: boolean;
  isError: boolean;
  retry: () => void;
};

type ProviderCardActions = {
  onSave: (payload: ProviderSavePayload) => Promise<void>;
  onReset: () => Promise<void>;
  onTest: (payload: ProviderTestPayload) => Promise<ConnectivityTestResponse>;
  isSaving: boolean;
  isResetting: boolean;
  isTesting: boolean;
};

type ProviderSettingsCardProps = {
  capability: ProviderCapability;
  title: string;
  role: string;
  data: ProviderCardData;
  actions: ProviderCardActions;
};

function formatProviderLabel(provider: string) {
  return (
    PROVIDER_LABELS[provider] ??
    provider
      .split(/[-_]/)
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(" ")
  );
}

function isAdminRole(role: string) {
  return role === "admin" || role === "owner";
}

function isEmbeddingCapability(capability: ProviderCapability) {
  return capability === "embedding";
}

function getDimensions(config: ProviderConfig) {
  return "dimensions" in config ? config.dimensions : null;
}

function getModel(config: ProviderConfig) {
  return config.model ?? "";
}

function buildDraftFromConfig(
  config: ProviderConfig | null | undefined,
  capability: ProviderCapability,
  fallbackProvider: string,
): CustomDraft {
  return {
    name: config?.name ?? "",
    provider: config?.provider ?? fallbackProvider,
    endpoint_url: config?.endpoint_url ?? "",
    model: config?.model ?? "",
    dimensions:
      capability === "embedding" && config && "dimensions" in config
        ? String(config.dimensions)
        : "",
    max_tokens:
      capability === "embedding" && config && "max_tokens" in config
        ? String(config.max_tokens)
        : "",
  };
}

function sourceDescription(
  resolved: ResolvedProjectProvider<ProviderConfig> | null | undefined,
) {
  switch (resolved?.source) {
    case "default":
      return "Using active global default";
    case "global":
      return "Using selected platform config";
    case "custom":
      return "Using project custom config";
    default:
      return "No provider configuration available";
  }
}

function credentialsStatus(
  source: ProjectProviderMode | undefined,
  hasCredentials: boolean | undefined,
) {
  if (!hasCredentials) {
    return "Not configured";
  }
  if (source === "custom") {
    return "Credentials saved";
  }
  return "Credentials managed by platform";
}

function modeLabel(mode: ProjectProviderMode | undefined) {
  if (mode === "global") return "Global";
  if (mode === "custom") return "Custom";
  if (mode === "default") return "Default";
  return "";
}

function sameDraft(
  left: CustomDraft,
  right: CustomDraft,
  capability: ProviderCapability,
) {
  return (
    left.name === right.name &&
    left.provider === right.provider &&
    left.endpoint_url === right.endpoint_url &&
    left.model === right.model &&
    (capability === "llm" || left.dimensions === right.dimensions) &&
    (capability === "llm" || left.max_tokens === right.max_tokens)
  );
}

function availableOptionLabel(config: ProviderConfig) {
  const parts = [
    config.name,
    formatProviderLabel(config.provider),
    getModel(config) || "No model",
  ];
  if (config.is_default) {
    parts.push("Default");
  }
  return parts.join(" · ");
}

function buildTestPayload(
  config: ProviderConfig,
  capability: ProviderCapability,
): ProviderTestPayload {
  return {
    provider: config.provider,
    endpoint_url: config.endpoint_url,
    ...(config.model ? { model: config.model } : {}),
    ...(capability === "embedding" &&
    "dimensions" in config &&
    typeof config.dimensions === "number"
      ? { dimensions: config.dimensions }
      : {}),
  };
}

function buildCurrentTestPayload(
  mode: ProjectProviderMode,
  capability: ProviderCapability,
  customValid: boolean,
  customDraft: CustomDraft,
  dimensionsValue: number,
  selectedGlobalConfig: ProviderConfig | undefined,
  defaultConfig: ProviderConfig | undefined,
): ProviderTestPayload | undefined {
  if (mode === "custom") {
    if (!customValid) return undefined;
    return {
      provider: customDraft.provider.trim(),
      endpoint_url: customDraft.endpoint_url.trim(),
      ...(customDraft.model.trim() ? { model: customDraft.model.trim() } : {}),
      ...(capability === "embedding" ? { dimensions: dimensionsValue } : {}),
    };
  }
  if (mode === "global") {
    return selectedGlobalConfig
      ? buildTestPayload(selectedGlobalConfig, capability)
      : undefined;
  }
  return defaultConfig
    ? buildTestPayload(defaultConfig, capability)
    : undefined;
}

function SummaryRow({
  label,
  value,
}: {
  label: string;
  value: string | number;
}) {
  return (
    <div className="grid gap-1 sm:grid-cols-[140px_1fr] sm:gap-3">
      <dt className="text-muted-foreground text-sm">{label}</dt>
      <dd className="text-sm font-medium break-all">{value}</dd>
    </div>
  );
}

function ModeOption({
  active,
  disabled,
  title,
  description,
  onClick,
}: {
  active: boolean;
  disabled?: boolean;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "rounded-lg border px-3 py-3 text-left transition-colors",
        active
          ? "border-primary bg-primary/5"
          : "hover:bg-muted/50 border-border bg-card",
        disabled && "cursor-not-allowed opacity-50",
      )}
    >
      <div className="text-sm font-medium">{title}</div>
      <div className="text-muted-foreground mt-1 text-xs leading-relaxed">
        {description}
      </div>
    </button>
  );
}

export function ProviderSettingsCard({
  capability,
  title,
  role,
  data,
  actions,
}: ProviderSettingsCardProps) {
  const isAdmin = isAdminRole(role);
  const supportedProviders = data.supportedProviders ?? [];
  const availableItems = data.availableItems ?? [];
  const savedSetting = data.setting;
  const resolved = data.resolved;
  const savedConfig = savedSetting?.config ?? resolved?.config;
  const defaultProvider = supportedProviders[0] ?? "";
  const savedDraft = buildDraftFromConfig(
    savedConfig,
    capability,
    defaultProvider,
  );
  const savedMode = savedSetting?.mode ?? "default";
  const savedGlobalConfigId = savedSetting?.global_config_id ?? "";

  const [mode, setMode] = useState<ProjectProviderMode>("default");
  const [selectedGlobalConfigId, setSelectedGlobalConfigId] = useState("");
  const [customDraft, setCustomDraft] = useState<CustomDraft>(savedDraft);
  const [testResult, setTestResult] = useState<{
    ok: boolean;
    message: string;
  } | null>(null);
  const [resetOpen, setResetOpen] = useState(false);
  // Tracks whether the form has been hydrated from server data at least once.
  const hasHydratedRef = useRef(false);
  // Set to true when the user makes any form edit; prevents server data from
  // overwriting in-progress edits on background refetches.
  const userEditedRef = useRef(false);
  // Set to true after a successful save; forces the next server data change
  // to resync the form even if userEditedRef is true.
  const pendingResyncRef = useRef(false);
  const dirty =
    mode !== savedMode ||
    (mode === "global" && selectedGlobalConfigId !== savedGlobalConfigId) ||
    (mode === "custom" &&
      (savedMode !== "custom" ||
        !sameDraft(customDraft, savedDraft, capability)));

  // Sync form state from server data on first render, on server data changes
  // (e.g. after a save), or when a pending resync is flagged. Skips if the user
  // has made edits and no resync is pending, to avoid overwriting their work.
  useEffect(() => {
    if (
      hasHydratedRef.current &&
      userEditedRef.current &&
      !pendingResyncRef.current
    ) {
      return;
    }
    /* eslint-disable react-hooks/set-state-in-effect */
    setMode(savedMode);
    setSelectedGlobalConfigId(savedGlobalConfigId);
    setCustomDraft(
      buildDraftFromConfig(savedConfig, capability, defaultProvider),
    );
    setTestResult(null);
    hasHydratedRef.current = true;
    userEditedRef.current = false;
    pendingResyncRef.current = false;
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [
    capability,
    defaultProvider,
    savedConfig,
    savedGlobalConfigId,
    savedMode,
  ]);

  const providerOptions =
    customDraft.provider && !supportedProviders.includes(customDraft.provider)
      ? [...supportedProviders, customDraft.provider]
      : supportedProviders;

  const globalOptions =
    savedSetting?.mode === "global" &&
    savedSetting.config &&
    !availableItems.some((item) => item.id === savedSetting.config.id)
      ? [savedSetting.config, ...availableItems]
      : availableItems;

  const endpointValid = isValidProviderEndpointURL(
    customDraft.endpoint_url.trim(),
  );
  const dimensionsValue = Number(customDraft.dimensions.trim());
  const dimensionsValid =
    capability === "llm" ||
    (/^\d+$/.test(customDraft.dimensions.trim()) &&
      dimensionsValue >= 1 &&
      dimensionsValue <= 65536);
  const maxTokensValue = Number(customDraft.max_tokens.trim());
  const maxTokensValid =
    capability === "llm" ||
    (/^\d+$/.test(customDraft.max_tokens.trim()) &&
      maxTokensValue >= 1 &&
      maxTokensValue <= 131072);
  const modelValid =
    capability === "llm" || customDraft.model.trim().length > 0;
  const customValid =
    customDraft.name.trim().length > 0 &&
    customDraft.provider.trim().length > 0 &&
    endpointValid &&
    modelValid &&
    dimensionsValid &&
    maxTokensValid;

  const supportsCustomMode =
    providerOptions.length > 0 || savedMode === "custom";
  const supportsGlobalMode = globalOptions.length > 0 || savedMode === "global";
  const canSave =
    !actions.isSaving &&
    !actions.isResetting &&
    dirty &&
    (mode === "default" ||
      (mode === "global" && selectedGlobalConfigId.length > 0) ||
      (mode === "custom" && customValid));
  const selectedGlobalConfig = globalOptions.find(
    (item) => item.id === selectedGlobalConfigId,
  );
  const defaultConfig =
    globalOptions.find((item) => item.is_default) ??
    (resolved?.source === "default" ? resolved.config : undefined);
  const currentTestPayload = buildCurrentTestPayload(
    mode,
    capability,
    customValid,
    customDraft,
    dimensionsValue,
    selectedGlobalConfig,
    defaultConfig,
  );
  const canTest =
    !actions.isTesting &&
    !actions.isSaving &&
    !actions.isResetting &&
    !!currentTestPayload;
  const showResetAction = isAdmin && savedMode !== "default";
  const showLoading =
    data.isLoading &&
    (data.supportedProviders === undefined ||
      data.availableItems === undefined ||
      data.setting === undefined ||
      data.resolved === undefined);
  const activeSource = resolved?.source ?? savedSetting?.mode;
  const activeModeLabel = modeLabel(activeSource);

  function clearTestResult() {
    setTestResult(null);
  }

  function openResetConfirmation() {
    setResetOpen(true);
  }

  function updateDraft<K extends keyof CustomDraft>(
    key: K,
    value: CustomDraft[K],
  ) {
    userEditedRef.current = true;
    setCustomDraft((current) => ({ ...current, [key]: value }));
    clearTestResult();
  }

  function selectMode(nextMode: ProjectProviderMode) {
    userEditedRef.current = true;
    setMode(nextMode);
    clearTestResult();

    if (nextMode === "global" && !selectedGlobalConfigId && globalOptions[0]) {
      setSelectedGlobalConfigId(globalOptions[0].id);
    }

    if (
      nextMode === "custom" &&
      !customDraft.provider &&
      providerOptions.length > 0
    ) {
      setCustomDraft((current) => ({
        ...current,
        provider: providerOptions[0] ?? current.provider,
      }));
    }
  }

  async function handleSave() {
    if (!canSave) {
      return;
    }

    if (mode === "default") {
      openResetConfirmation();
      return;
    }

    if (mode === "global") {
      try {
        await actions.onSave({
          mode: "global",
          global_config_id: selectedGlobalConfigId,
        });
        pendingResyncRef.current = true;
      } catch {
        return;
      }
      return;
    }

    const payload: ProviderSavePayload = {
      mode: "custom",
      name: customDraft.name.trim(),
      provider: customDraft.provider.trim(),
      endpoint_url: customDraft.endpoint_url.trim(),
      ...(customDraft.model.trim() ? { model: customDraft.model.trim() } : {}),
      ...(capability === "embedding"
        ? { dimensions: dimensionsValue, max_tokens: maxTokensValue }
        : {}),
    };

    try {
      await actions.onSave(payload);
      pendingResyncRef.current = true;
    } catch {
      return;
    }
  }

  async function handleTest() {
    if (!canTest) {
      return;
    }

    setTestResult(null);

    try {
      if (!currentTestPayload) {
        return;
      }
      const result = await actions.onTest(currentTestPayload);
      setTestResult({ ok: result.ok, message: result.message });
    } catch (error) {
      setTestResult({
        ok: false,
        message:
          error instanceof Error
            ? error.message
            : "Failed to test provider connectivity. Please try again.",
      });
    }
  }

  async function handleReset() {
    try {
      await actions.onReset();
      pendingResyncRef.current = true;
      clearTestResult();
      setResetOpen(false);
    } catch {
      return;
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-1">
            <CardTitle>{title}</CardTitle>
            <CardDescription>{sourceDescription(resolved)}</CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            {activeModeLabel ? (
              <Badge variant="secondary">{activeModeLabel}</Badge>
            ) : null}
            {(activeSource === "default" || activeSource === "global") && (
              <Badge variant="outline">Platform managed</Badge>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        {showLoading ? (
          <div className="space-y-6">
            <div className="space-y-3">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-20 w-full" />
            </div>
            <div className="space-y-3">
              <Skeleton className="h-4 w-28" />
              <div className="grid gap-2 sm:grid-cols-3">
                <Skeleton className="h-20 w-full" />
                <Skeleton className="h-20 w-full" />
                <Skeleton className="h-20 w-full" />
              </div>
            </div>
            <div className="space-y-3">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          </div>
        ) : data.isError ? (
          <Alert variant="destructive">
            <AlertCircle />
            <AlertTitle>
              Failed to load {title.toLowerCase()} settings.
            </AlertTitle>
            <AlertDescription className="gap-3">
              <p>Retry after the provider queries have been refreshed.</p>
              <Button
                variant="outline"
                size="sm"
                className="text-foreground"
                onClick={data.retry}
              >
                Retry
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <>
            {resolved ? (
              <div className="rounded-lg border p-4">
                <div className="mb-3 text-sm font-medium">
                  Effective configuration
                </div>
                <dl className="space-y-3">
                  <SummaryRow label="Name" value={resolved.config.name} />
                  <SummaryRow
                    label="Provider"
                    value={formatProviderLabel(resolved.config.provider)}
                  />
                  {isAdmin ? (
                    <SummaryRow
                      label="Endpoint URL"
                      value={resolved.config.endpoint_url}
                    />
                  ) : null}
                  <SummaryRow
                    label="Model"
                    value={getModel(resolved.config) || "—"}
                  />
                  {isEmbeddingCapability(capability) && (
                    <SummaryRow
                      label="Dimensions"
                      value={getDimensions(resolved.config) ?? "—"}
                    />
                  )}
                  {isEmbeddingCapability(capability) && (
                    <SummaryRow
                      label="Max Tokens"
                      value={
                        "max_tokens" in resolved.config
                          ? resolved.config.max_tokens
                          : "—"
                      }
                    />
                  )}
                  <SummaryRow
                    label="Credentials"
                    value={credentialsStatus(
                      resolved.source,
                      resolved.config.has_credentials,
                    )}
                  />
                </dl>
              </div>
            ) : (
              <Alert>
                <AlertCircle />
                <AlertTitle>No effective provider config found</AlertTitle>
                <AlertDescription>
                  {isAdmin
                    ? "Configure a custom project provider or wait for a platform default to be added."
                    : "Ask a project admin to configure a custom provider or wait for a platform default to be added."}
                </AlertDescription>
              </Alert>
            )}

            {!isAdmin ? null : (
              <>
                <div className="space-y-3">
                  <div className="space-y-1">
                    <Label>Selection mode</Label>
                    <p className="text-muted-foreground text-sm">
                      Choose whether this project follows the platform default,
                      a platform config, or its own custom provider config.
                    </p>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-3">
                    <ModeOption
                      active={mode === "default"}
                      title="Use active global default"
                      description="Follow the currently active platform default configuration."
                      onClick={() => selectMode("default")}
                    />
                    <ModeOption
                      active={mode === "global"}
                      disabled={!supportsGlobalMode && mode !== "global"}
                      title="Use platform config"
                      description="Select one of the platform-managed shared provider configs."
                      onClick={() => selectMode("global")}
                    />
                    <ModeOption
                      active={mode === "custom"}
                      disabled={!supportsCustomMode && mode !== "custom"}
                      title="Use custom project config"
                      description="Store a project-owned provider configuration without exposing stored secrets."
                      onClick={() => selectMode("custom")}
                    />
                  </div>
                  {!supportsGlobalMode && (
                    <p className="text-muted-foreground text-sm">
                      No platform-provided configs are currently available for
                      selection.
                    </p>
                  )}
                  {!supportsCustomMode && (
                    <p className="text-muted-foreground text-sm">
                      No providers are currently supported for this capability.
                    </p>
                  )}
                </div>

                {mode === "global" && (
                  <div className="space-y-2">
                    <Label htmlFor={`${capability}-global-config`}>
                      Platform config
                    </Label>
                    <Select
                      value={selectedGlobalConfigId || undefined}
                      onValueChange={(value) => {
                        userEditedRef.current = true;
                        setSelectedGlobalConfigId(value);
                        clearTestResult();
                      }}
                    >
                      <SelectTrigger
                        id={`${capability}-global-config`}
                        className="w-full"
                      >
                        <SelectValue placeholder="Select a platform config..." />
                      </SelectTrigger>
                      <SelectContent>
                        {globalOptions.map((config) => (
                          <SelectItem key={config.id} value={config.id}>
                            {availableOptionLabel(config)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <p className="text-muted-foreground text-sm">
                      This uses a platform-managed configuration. Credentials,
                      if any, are not visible to project users.
                    </p>
                  </div>
                )}

                {mode === "custom" && (
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor={`${capability}-name`}>Name</Label>
                      <Input
                        id={`${capability}-name`}
                        placeholder={
                          capability === "embedding"
                            ? "e.g. Project Ollama Embeddings"
                            : "e.g. Project Ollama Chat"
                        }
                        value={customDraft.name}
                        onChange={(event) =>
                          updateDraft("name", event.target.value)
                        }
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={`${capability}-provider`}>Provider</Label>
                      <Select
                        value={customDraft.provider || undefined}
                        onValueChange={(value) =>
                          updateDraft("provider", value)
                        }
                      >
                        <SelectTrigger
                          id={`${capability}-provider`}
                          className="w-full"
                        >
                          <SelectValue placeholder="Select a provider..." />
                        </SelectTrigger>
                        <SelectContent>
                          {providerOptions.map((provider) => (
                            <SelectItem key={provider} value={provider}>
                              {formatProviderLabel(provider)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={`${capability}-endpoint-url`}>
                        Endpoint URL
                      </Label>
                      <Input
                        id={`${capability}-endpoint-url`}
                        placeholder="e.g. http://localhost:11434"
                        value={customDraft.endpoint_url}
                        onChange={(event) =>
                          updateDraft("endpoint_url", event.target.value)
                        }
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={`${capability}-model`}>Model</Label>
                      <Input
                        id={`${capability}-model`}
                        placeholder={
                          capability === "embedding"
                            ? "e.g. jina/jina-embeddings-v2-base-en"
                            : "e.g. llama3.1"
                        }
                        value={customDraft.model}
                        onChange={(event) =>
                          updateDraft("model", event.target.value)
                        }
                      />
                      {capability === "llm" && (
                        <p className="text-muted-foreground text-sm">
                          Optional in phase 1 for LLM providers.
                        </p>
                      )}
                    </div>
                    {isEmbeddingCapability(capability) && (
                      <div className="space-y-2">
                        <Label htmlFor={`${capability}-dimensions`}>
                          Dimensions
                        </Label>
                        <Input
                          id={`${capability}-dimensions`}
                          type="number"
                          min={1}
                          max={65536}
                          placeholder="e.g. 768"
                          value={customDraft.dimensions}
                          onChange={(event) =>
                            updateDraft("dimensions", event.target.value)
                          }
                        />
                      </div>
                    )}
                    {isEmbeddingCapability(capability) && (
                      <div className="space-y-2">
                        <Label htmlFor={`${capability}-max-tokens`}>
                          Max Tokens
                        </Label>
                        <Input
                          id={`${capability}-max-tokens`}
                          type="number"
                          min={1}
                          max={131072}
                          placeholder="e.g. 8000"
                          value={customDraft.max_tokens}
                          onChange={(event) =>
                            updateDraft("max_tokens", event.target.value)
                          }
                        />
                      </div>
                    )}
                    <p className="text-muted-foreground text-sm">
                      Stored credentials are never shown back in plaintext. This
                      phase only exposes whether credentials are configured.
                    </p>
                  </div>
                )}

                <div className="flex flex-wrap gap-2 pt-2">
                  <Button onClick={handleSave} disabled={!canSave}>
                    {actions.isSaving || actions.isResetting
                      ? "Saving..."
                      : "Save"}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={handleTest}
                    disabled={!canTest}
                  >
                    {actions.isTesting ? "Testing..." : "Test Connection"}
                  </Button>
                  {showResetAction && (
                    <Dialog open={resetOpen} onOpenChange={setResetOpen}>
                      <DialogTrigger asChild>
                        <Button variant="ghost">
                          <RotateCcw className="size-4" />
                          Reset to Default
                        </Button>
                      </DialogTrigger>
                      <DialogContent>
                        <DialogHeader>
                          <DialogTitle>Reset to Default</DialogTitle>
                          <DialogDescription>
                            Reset this project to the active global default
                            configuration? Any project-specific selection will
                            be removed.
                          </DialogDescription>
                        </DialogHeader>
                        <DialogFooter>
                          <Button
                            variant="outline"
                            onClick={() => setResetOpen(false)}
                          >
                            Cancel
                          </Button>
                          <Button
                            variant="destructive"
                            onClick={handleReset}
                          >
                            Reset to Default
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                  )}
                </div>

                {testResult && (
                  <Alert variant={testResult.ok ? "default" : "destructive"}>
                    {testResult.ok ? <CheckCircle2 /> : <XCircle />}
                    <AlertTitle>
                      {testResult.ok
                        ? "Connection successful"
                        : "Connection failed"}
                    </AlertTitle>
                    <AlertDescription>{testResult.message}</AlertDescription>
                  </Alert>
                )}
              </>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
