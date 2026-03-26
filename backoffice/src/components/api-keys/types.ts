export type APIKeyBase = {
  id: string;
  key_prefix: string;
  name: string;
  role: "read" | "write";
  is_active: boolean;
  expires_at: string | null;
  last_used_at: string | null;
  created_at: string;
};

export type CreateAPIKeyResponseBase = {
  id: string;
  key_prefix: string;
  plaintext_key: string;
  name: string;
  role: "read" | "write";
  expires_at: string | null;
  created_at: string;
};
