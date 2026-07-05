import { apiRequest } from "../../api/client";
import type { BenchmarkSetPage, BenchmarkSetView, EvaluationRunPage, EvaluationRunView } from "./evaluations.types";

export const listBenchmarkSets = () => apiRequest<BenchmarkSetPage>("/v1/benchmarks");

export const getBenchmarkSet = (id: string) => apiRequest<BenchmarkSetView>(`/v1/benchmarks/${encodeURIComponent(id)}`);

export const createBenchmarkSet = (payload: { name: string; description?: string; task_refs: string[] }) =>
  apiRequest<BenchmarkSetView>("/v1/benchmarks", { method: "POST", body: payload });

export const listEvaluationRuns = (agentVersionId?: string) => {
  const query = agentVersionId ? `?agent_version_id=${encodeURIComponent(agentVersionId)}` : "";
  return apiRequest<EvaluationRunPage>(`/v1/evaluations${query}`);
};

export const getEvaluationRun = (id: string) =>
  apiRequest<EvaluationRunView>(`/v1/evaluations/${encodeURIComponent(id)}`);
