export type SampleSizeHint = "low" | "medium" | "high";

export type ProjectionView = {
  agent_version_id: string;
  capability: string;
  task_type: string;
  total_reviews: number;
  pass_rate?: number;
  rework_rate?: number;
  avg_review_cost_cents?: number;
  avg_review_latency_ms?: number;
  sample_size_hint: SampleSizeHint;
};
