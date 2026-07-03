import { apiRequest } from "../../api/client";
import type { AgentAction, AgentPage, AgentView, RegisterAgentRequest, RegisterAgentResponse } from "./agents.types";

export const listAgents = () => apiRequest<AgentPage>("/v1/agents");

export const getAgent = (id: string) => apiRequest<AgentView>(`/v1/agents/${encodeURIComponent(id)}`);

export const registerAgent = (payload: RegisterAgentRequest) =>
  apiRequest<RegisterAgentResponse>("/v1/agents", { method: "POST", body: payload });

export function updateAgentStatus(id: string, action: AgentAction) {
  return apiRequest<AgentView>(`/v1/agents/${encodeURIComponent(id)}:${action}`, { method: "POST" });
}
