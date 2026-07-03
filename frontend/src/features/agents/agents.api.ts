import { apiRequest } from "../../api/client";
import type { AccessTokenView, AgentAction, AgentPage, AgentStatus, AgentView, RegisterAgentRequest } from "./agents.types";

export async function listAgents(filters: { status?: AgentStatus } = {}) {
  const params = new URLSearchParams();
  if (filters.status) {
    params.set("status", filters.status);
  }
  const query = params.toString();
  return apiRequest<AgentPage>(`/v1/agents${query ? `?${query}` : ""}`);
}

export const getAgent = (id: string) => apiRequest<AgentView>(`/v1/agents/${encodeURIComponent(id)}`);

export const registerAgent = (payload: RegisterAgentRequest) =>
  apiRequest<AccessTokenView>("/v1/agents", { method: "POST", body: payload });

export function updateAgentStatus(id: string, action: AgentAction) {
  return apiRequest<AgentView>(`/v1/agents/${encodeURIComponent(id)}/${action}`, { method: "POST" });
}
