import { apiRequest, type Envelope } from "../../api/client";
import type { ProjectionView } from "./reputation.types";

export type ReputationFilters = {
  agentVersionId?: string;
  capability?: string;
  taskType?: string;
};

export async function getReputationProjection(filters: ReputationFilters = {}): Promise<Envelope<ProjectionView>> {
  const params = new URLSearchParams();
  if (filters.agentVersionId) {
    params.set("agent_version_id", filters.agentVersionId);
  }
  if (filters.capability) {
    params.set("capability", filters.capability);
  }
  if (filters.taskType) {
    params.set("task_type", filters.taskType);
  }
  const query = params.toString();
  return apiRequest<ProjectionView>(`/v1/reputation${query ? `?${query}` : ""}`);
}
