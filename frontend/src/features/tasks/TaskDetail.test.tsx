import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { envelope, executionViewFixture, taskViewFixture } from "../../api/fixtures";
import { TaskDetail } from "./TaskDetail";

describe("TaskDetail", () => {
  it("renders lease and cost coverage without mutation controls", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify(
            envelope(
              taskViewFixture({
                id: "task-1",
                title: "修复并发退款",
                status: "in_progress",
                active_execution_id: "exec-1",
              }),
            ),
          ),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(envelope(executionViewFixture())), { status: 200 }),
      );

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <TaskDetail taskId="task-1" />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Running")).toBeVisible();
    expect(screen.getByText("10m lease")).toBeVisible();
    expect(screen.getByText("Cost coverage: partial")).toBeVisible();
    expect(screen.getByText("运行测试")).toBeVisible();
    expect(screen.getByText(/Observed cost: \$0\.067/)).toBeVisible();
    expect(screen.getByText(/Provider: langfuse/)).toBeVisible();
    expect(screen.queryByRole("button", { name: /claim|accept|set status/i })).toBeNull();
  });
});
