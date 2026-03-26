export type SSHKey = {
  id: string;
  name: string;
  fingerprint: string;
  public_key: string;
  key_type: string;
  is_active: boolean;
  created_by: string;
  rotated_at: string | null;
  created_at: string;
};

export type SSHKeyProject = {
  id: string;
  name: string;
  repo_url: string;
  default_branch: string;
  status: "active" | "paused";
  created_by: string;
  created_at: string;
  updated_at: string;
};
