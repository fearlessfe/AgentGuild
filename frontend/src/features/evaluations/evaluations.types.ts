export type BenchmarkSetView = {
  id: string;
  tenant_id: string;
  version_number: number;
  name: string;
  description: string;
  is_active: boolean;
  created_by: string;
  created_at: string;
};

export type CreateBenchmarkSetRequest = {
  name: string;
  description?: string;
  task_refs: string[];
  is_active?: boolean;
};

export type CreateBenchmarkSetResponse = {
  benchmark_set_id: string;
  version_number: number;
};

export type ThresholdResultView = {
  name: string;
  passed: boolean;
  evidence?: Record<string, unknown>;
};

export type EvaluationSummaryView = {
  pass_rate: number;
  avg_latency_ms: number;
  cost_cents: number;
  security_passed: boolean;
  executor?: string;
  extra?: Record<string, unknown>;
};

export type EvaluationRunStatus = "running" | "passed" | "failed";

export type EvaluationRunView = {
  id: string;
  tenant_id: string;
  agent_version_id: string;
  benchmark_set_id: string;
  status: EvaluationRunStatus;
  environment_digest: string;
  scoring_rule_version: string;
  threshold_results?: ThresholdResultView[];
  summary?: EvaluationSummaryView;
  started_at: string;
  completed_at?: string;
};

export type EvaluationRunPage = { items: EvaluationRunView[] };
export type BenchmarkSetPage = { items: BenchmarkSetView[] };
