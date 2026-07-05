import { apiRequest } from "../../api/client";
import type { CreateDraftRequest, VersionDiff, VersionPage, VersionView } from "./versions.types";

export const listVersions = (agentId: string) =>
  apiRequest<VersionPage>(`/v1/agents/${encodeURIComponent(agentId)}/versions`);

export const getVersion = (agentId: string, versionId: string) =>
  apiRequest<VersionView>(`/v1/agents/${encodeURIComponent(agentId)}/versions/${encodeURIComponent(versionId)}`);

export const createDraft = (agentId: string, payload: CreateDraftRequest) =>
  apiRequest<{ version_id: string; version_number: number; status: string }>(
    `/v1/agents/${encodeURIComponent(agentId)}/versions`,
    { method: "POST", body: payload },
  );

export const diffVersion = (agentId: string, versionId: string, baseVersionId?: string) =>
  apiRequest<VersionDiff>(`/v1/agents/${encodeURIComponent(agentId)}/versions/${encodeURIComponent(versionId)}/diff`, {
    method: "POST",
    body: baseVersionId ? { base_version_id: baseVersionId } : {},
  });

export const startEvaluation = (agentId: string, versionId: string, benchmarkSetId: string, environmentDigest: string) =>
  apiRequest<{ evaluation_run_id: string }>(
    `/v1/agents/${encodeURIComponent(agentId)}/versions/${encodeURIComponent(versionId)}/evaluations`,
    { method: "POST", body: { benchmark_set_id: benchmarkSetId, environment_digest: environmentDigest } },
  );

export const promoteVersion = (agentId: string, versionId: string) =>
  apiRequest<VersionView>(`/v1/agents/${encodeURIComponent(agentId)}/versions/${encodeURIComponent(versionId)}/promote`, {
    method: "POST",
  });

export const rollbackVersion = (agentId: string, versionId: string) =>
  apiRequest<VersionView>(`/v1/agents/${encodeURIComponent(agentId)}/versions/${encodeURIComponent(versionId)}/rollback`, {
    method: "POST",
  });
