import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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
        <MemoryRouter><TaskDetail taskId="task-1" /></MemoryRouter>
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

  it("renders synced issue tasks when requirements are null", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          envelope({
            ...taskViewFixture({
              id: "issue-task-1",
              type: "github_issue",
              title: "Update instructions for Monitoring Workshop section",
              requirements: null,
              constraints: null,
              source: {
                kind: "issue",
                repo: "langfuse/langfuse",
                issue_number: 13862,
                issue_url: "https://github.com/langfuse/langfuse/issues/13862",
              },
            }),
          }),
        ),
        { status: 200 },
      ),
    );

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter><TaskDetail taskId="issue-task-1" /></MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Update instructions for Monitoring Workshop section")).toBeVisible();
    expect(screen.getByText("Issue #13862 @ langfuse/langfuse")).toBeVisible();
    expect(screen.getByText("暂无验收标准")).toBeVisible();
  });

  it("links a completed task to its retained execution and outcome", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify(envelope(taskViewFixture({ id: "task-1", status: "completed", active_execution_id: "exec-1" }))),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(envelope(executionViewFixture({ id: "exec-1", status: "accepted" }))), { status: 200 }),
      );

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter><TaskDetail taskId="task-1" /></MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Accepted")).toBeVisible();
    expect(screen.getByRole("link", { name: "查看结果" })).toHaveAttribute("href", "/outcome?task_id=task-1");
  });
});
