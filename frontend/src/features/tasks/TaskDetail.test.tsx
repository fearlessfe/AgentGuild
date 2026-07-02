import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TaskDetail } from "./TaskDetail";

describe("TaskDetail", () => {
  it("renders lease and cost coverage without mutation controls", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { id: "task-1", title: "修复并发退款", type: "code", status: "in_progress", deadline: "2026-07-03T10:00:00Z", created_at: "2026-07-02T01:00:00Z", updated_at: "2026-07-02T01:00:00Z", publisher_agent_version_id: "billing", tenant_id: "tenant", problem: "避免余额重复扣减", constraints: ["幂等"], requirements: ["并发测试"], state_version: 2, active_execution_id: "exec-1" }, meta: { server_time: "2026-07-02T02:00:00Z", resource_version: 2, poll_after_seconds: 17 } }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { id: "exec-1", task_id: "task-1", tenant_id: "tenant", agent_version_id: "Atlas v12", status: "running", stage: "运行测试", progress: 65, lease_generation: 3, lease_soft_expires_at: "2026-07-02T02:10:00Z", lease_hard_expires_at: "2026-07-02T02:10:30Z", cost: { observed: "0.067", coverage: "partial" }, audit_summary: "agent heartbeat · running" }, meta: { server_time: "2026-07-02T02:00:00Z", resource_version: 3, poll_after_seconds: 19 } }), { status: 200 }));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><TaskDetail taskId="task-1" /></QueryClientProvider>);

    expect(await screen.findByText("Lease: active")).toBeVisible();
    expect(screen.getByText("Cost coverage: partial")).toBeVisible();
    expect(screen.getByText("运行测试")).toBeVisible();
    expect(screen.queryByRole("button", { name: /claim|accept|set status/i })).toBeNull();
  });
});
