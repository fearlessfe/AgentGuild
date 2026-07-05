import { apiRequest } from "../../api/client";
import type { Envelope } from "../../api/client";
import type {
  BenchmarkSetPage,
  BenchmarkSetView,
  CreateBenchmarkSetRequest,
  CreateBenchmarkSetResponse,
  EvaluationRunPage,
  EvaluationRunView,
} from "./evaluations.types";

function unwrapEvaluationResponse<T>(json: unknown): T {
  if (json && typeof json === "object" && "data" in json) {
    return (json as Envelope<T>).data;
  }
  return json as T;
}

export const listBenchmarkSets = (): Promise<BenchmarkSetPage> =>
  apiRequest<BenchmarkSetPage>("/v1/benchmarks").then((json) => unwrapEvaluationResponse<BenchmarkSetPage>(json));

export const getBenchmarkSet = (id: string): Promise<BenchmarkSetView> =>
  apiRequest<BenchmarkSetView>(`/v1/benchmarks/${encodeURIComponent(id)}`).then((json) =>
    unwrapEvaluationResponse<BenchmarkSetView>(json),
  );

export const createBenchmarkSet = (payload: CreateBenchmarkSetRequest): Promise<CreateBenchmarkSetResponse> =>
  apiRequest<CreateBenchmarkSetResponse>("/v1/benchmarks", { method: "POST", body: payload }).then((json) =>
    unwrapEvaluationResponse<CreateBenchmarkSetResponse>(json),
  );

export const listEvaluationRuns = (agentVersionId?: string): Promise<EvaluationRunPage> => {
  const query = agentVersionId ? `?agent_version_id=${encodeURIComponent(agentVersionId)}` : "";
  return apiRequest<EvaluationRunPage>(`/v1/evaluations${query}`).then((json) =>
    unwrapEvaluationResponse<EvaluationRunPage>(json),
  );
};

export const getEvaluationRun = (id: string): Promise<EvaluationRunView> =>
  apiRequest<EvaluationRunView>(`/v1/evaluations/${encodeURIComponent(id)}`).then((json) =>
    unwrapEvaluationResponse<EvaluationRunView>(json),
  );
