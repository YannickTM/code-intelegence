export type ProviderCapability = "embedding" | "llm";

export type ProjectProviderMode = "default" | "global" | "custom";

export type SupportedProvidersResponse = {
  embedding: string[];
  llm: string[];
};

type ProviderConfigBase = {
  id: string;
  name: string;
  provider: string;
  endpoint_url: string;
  settings: Record<string, unknown>;
  has_credentials: boolean;
  is_active: boolean;
  is_default: boolean;
  is_available_to_projects: boolean;
  project_id?: string;
  created_at: string;
  updated_at: string;
};

export type EmbeddingProviderConfig = ProviderConfigBase & {
  model: string;
  dimensions: number;
  max_tokens: number;
};

export type LLMProviderConfig = ProviderConfigBase & {
  model?: string | null;
};

export type ProviderConfig = EmbeddingProviderConfig | LLMProviderConfig;

export type AvailableProviderConfigsResponse<TConfig extends ProviderConfig> = {
  items: TConfig[];
};

export type ProjectProviderSetting<TConfig extends ProviderConfig> = {
  mode: ProjectProviderMode;
  global_config_id?: string;
  config: TConfig;
};

export type ResolvedProjectProvider<TConfig extends ProviderConfig> = {
  source: ProjectProviderMode;
  config: TConfig;
};

export type ConnectivityTestResponse = {
  ok: boolean;
  provider: string;
  message: string;
};

export function isEmbeddingProviderConfig(
  config: ProviderConfig,
): config is EmbeddingProviderConfig {
  return "dimensions" in config;
}
