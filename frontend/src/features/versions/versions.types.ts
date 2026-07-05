export type VersionStatus = "draft" | "evaluating" | "eligible" | "active" | "retired" | "rejected";

export type VersionView = {
  id: string;
  tenant_id: string;
  agent_id: string;
  version_number: number;
  parent_version_id?: string;
  status: VersionStatus;
  runtime: string;
  model: string;
  capabilities: string[];
  config_fingerprint: string;
  content_hash: string;
  environment_digest: string;
  prompt_ref?: string;
  skill_refs: string[];
  memory_ref?: string;
  tool_refs: string[];
  created_by: string;
  created_at: string;
  promoted_at?: string;
  retired_at?: string;
  rejected_reason?: string;
};

export type VersionPage = { items: VersionView[] };

export type VersionDiff = {
  base_version_id: string;
  target_version_id: string;
  added_capabilities: string[];
  removed_capabilities: string[];
  changed_refs: { field: string; from?: string; to?: string }[];
};

export type CreateDraftRequest = {
  runtime: string;
  model: string;
  capabilities?: string[];
  prompt_ref?: string;
  skill_refs?: string[];
  memory_ref?: string;
  tool_refs?: string[];
  environment_digest: string;
  approved_experience_ids?: string[];
};
