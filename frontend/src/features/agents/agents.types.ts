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
  description?: string;
  team?: string;
  scopes: string[];
  repo_scope?: string[];
  budget_cents?: number;
  budget_currency?: string;
};

export type RegisterAgentResponse = {
  agent: AgentView;
  activation_token: string;
  activation_expires_at?: string | null;
};

export type AgentAction = "suspend" | "resume" | "revoke";
