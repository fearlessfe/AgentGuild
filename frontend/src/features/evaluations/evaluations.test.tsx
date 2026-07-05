import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BenchmarkSetForm, EvaluationList } from "./EvaluationList";
import type { EvaluationRunView } from "./evaluations.types";

const mockRun: EvaluationRunView = {
  id: "run1",
  tenant_id: "t1",
  agent_version_id: "v1",
  benchmark_set_id: "bs1",
  status: "passed",
  environment_digest: "env",
  scoring_rule_version: "v1",
  threshold_results: [{ name: "security", passed: true }],
  summary: { pass_rate: 1.0, avg_latency_ms: 100, cost_cents: 0, security_passed: true },
  started_at: "2026-07-04T00:00:00Z",
};

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  return {
    ...actual,
    useQuery: () => ({ isPending: false, isError: false, data: { data: { items: [mockRun] } } }),
  };
});

vi.mock("./evaluations.api", () => ({
  listEvaluationRuns: vi.fn(),
  getEvaluationRun: vi.fn(),
}));

describe("EvaluationList", () => {
  it("renders passed run", () => {
    render(<EvaluationList agentVersionId="v1" />);
    expect(screen.getByText("评测运行")).toBeInTheDocument();
    expect(screen.getByText("passed")).toBeInTheDocument();
  });
});

describe("BenchmarkSetForm", () => {
  it("renders placeholder form", () => {
    render(<BenchmarkSetForm />);
    expect(screen.getByText("基准集")).toBeInTheDocument();
  });
});
