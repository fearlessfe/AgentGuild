import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { BenchmarkSetForm, EvaluationDetail, EvaluationList } from "./EvaluationList";
import * as evaluationsApi from "./evaluations.api";
import type { EvaluationRunView } from "./evaluations.types";

function renderWithProviders(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

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

vi.mock("./evaluations.api", async () => {
  const actual = await vi.importActual<typeof import("./evaluations.api")>("./evaluations.api");
  return {
    ...actual,
    listEvaluationRuns: vi.fn(),
    getEvaluationRun: vi.fn(),
    createBenchmarkSet: vi.fn(),
  };
});

describe("EvaluationList", () => {
  it("renders passed run", async () => {
    vi.mocked(evaluationsApi.listEvaluationRuns).mockResolvedValue({ items: [mockRun] });
    renderWithProviders(<EvaluationList agentVersionId="v1" />);
    expect(await screen.findByText("评测运行")).toBeInTheDocument();
    expect(screen.getByText("passed")).toBeInTheDocument();
  });
});

describe("BenchmarkSetForm", () => {
  it("renders form fields", () => {
    renderWithProviders(<BenchmarkSetForm />);
    expect(screen.getByLabelText(/名称/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/描述/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/任务引用/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/设为默认/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /创建基准集/i })).toBeInTheDocument();
  });

  it("disables submit when required fields are empty", () => {
    renderWithProviders(<BenchmarkSetForm />);
    expect(screen.getByRole("button", { name: /创建基准集/i })).toBeDisabled();
  });

  it("calls createBenchmarkSet with parsed payload on submit", async () => {
    vi.mocked(evaluationsApi.createBenchmarkSet).mockResolvedValue({
      benchmark_set_id: "bs-new",
      version_number: 2,
    });
    const onCreated = vi.fn();

    renderWithProviders(<BenchmarkSetForm onCreated={onCreated} />);

    await userEvent.type(screen.getByLabelText(/名称/i), "回归基准");
    await userEvent.type(screen.getByLabelText(/描述/i), "核心回归任务");
    await userEvent.type(screen.getByLabelText(/任务引用/i), "task-1, task-2\ntask-3");

    await userEvent.click(screen.getByRole("button", { name: /创建基准集/i }));

    await waitFor(() => expect(evaluationsApi.createBenchmarkSet).toHaveBeenCalledTimes(1));
    expect(evaluationsApi.createBenchmarkSet).toHaveBeenCalledWith({
      name: "回归基准",
      description: "核心回归任务",
      task_refs: ["task-1", "task-2", "task-3"],
      is_active: true,
    });
    expect(onCreated).toHaveBeenCalledTimes(1);
  });
});

describe("EvaluationDetail", () => {
  it("renders run with full summary and threshold results", async () => {
    vi.mocked(evaluationsApi.getEvaluationRun).mockResolvedValue(mockRun);
    renderWithProviders(<EvaluationDetail runId="run1" />);
    expect(await screen.findByText(/评测 run1/)).toBeInTheDocument();
    expect(screen.getByText(/security/)).toBeInTheDocument();
  });

  it("renders run with missing summary and threshold results without crashing", async () => {
    const incompleteRun = {
      ...mockRun,
      summary: undefined as unknown as EvaluationRunView["summary"],
      threshold_results: undefined as unknown as EvaluationRunView["threshold_results"],
    };
    vi.mocked(evaluationsApi.getEvaluationRun).mockResolvedValue(incompleteRun);
    renderWithProviders(<EvaluationDetail runId="run1" />);
    expect(await screen.findByText(/评测 run1/)).toBeInTheDocument();
    expect(screen.queryByText("security")).not.toBeInTheDocument();
  });
});
