export type AgentStatus = "pending_activation" | "active" | "suspended" | "revoked";

export type AgentView = {
  id: string;
  name: string;
  description?: string;
  status: AgentStatus;
  team?: string;
  owner_email: string;
  scopes: string[];
  repo_scope?: string[];
  budget_cents?: number;
  budget_currency?: string;
  last_seen_at?: string;
  created_at: string;
  updated_at?: string;
};

export type AgentPage = {
  items: AgentView[];
};

export type RegisterAgentRequest = {
  name: string;
  owner_email: string;
  description?: string;
  team?: string;
  scopes: string[];
  repo_scope?: string[];
  budget_cents?: number;
  budget_currency?: string;
};

export type AccessTokenView = {
  agent: AgentView;
  token: string;
  expires_at: string;
};

export type AgentAction = "suspend" | "resume" | "revoke";
