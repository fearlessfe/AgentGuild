import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { envelope, taskPageFixture, taskViewFixture } from "../../api/fixtures";
import { TaskList } from "./TaskList";

function renderList(initialEntries?: string[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <QueryClientProvider client={queryClient}>
        <TaskList />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

describe("TaskList", () => {
  it("renders grouped read-only tasks and forwards the opaque cursor unchanged", async () => {
    const cursor = "eyJ0IjoidGVuYW50In0.signature/opaque+part";
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify(
            envelope(
              taskPageFixture([
                taskViewFixture({ id: "AG-192", title: "修复批量退款时的余额竞争条件", status: "open" }),
                taskViewFixture({ id: "AG-188", title: "修复定时任务重复触发问题", status: "in_progress" }),
              ]),
              { next_cursor: cursor },
            ),
          ),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(envelope(taskPageFixture([]))), { status: 200 }),
      );

    renderList();
    expect(await screen.findByText("待领取 · 1")).toBeVisible();
    expect(screen.getByText("进行中 · 1")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const requested = new URL(String(fetchMock.mock.calls[1][0]), "http://localhost");
    expect(requested.searchParams.get("cursor")).toBe(cursor);
    expect(screen.queryByRole("button", { name: /claim|accept|set status/i })).toBeNull();
  });

  it("wires status and type filters to REST query params", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify(envelope(taskPageFixture([]))), { status: 200 })),
    );

    renderList();
    await screen.findByRole("tab", { name: /进行中/ });

    // Switch to "进行中" tab filters by in_progress.
    await userEvent.click(screen.getByRole("tab", { name: /进行中/ }));
    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => new URL(String(call[0]), "http://localhost"));
      expect(calls.at(-1)?.searchParams.get("status")).toBe("in_progress");
    });

    // Select a type from the filter dropdown.
    await userEvent.selectOptions(screen.getByRole("combobox", { name: /类型/ }), "Go");
    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => new URL(String(call[0]), "http://localhost"));
      const last = calls.at(-1)!;
      expect(last.searchParams.get("type")).toBe("Go");
      expect(last.searchParams.get("status")).toBe("in_progress");
    });

    expect(screen.queryByRole("button", { name: /claim|accept|set status/i })).toBeNull();
  });

  it("renders server facts instead of synthesised progress or cost", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          envelope(
            taskPageFixture([
              taskViewFixture({
                id: "AG-100",
                title: "服务器事实标题",
                status: "open",
                publisher_agent_version_id: "real-repo",
                type: "Rust",
                deadline: "2026-07-04T18:30:00Z",
              }),
            ]),
          ),
        ),
        { status: 200 },
      ),
    );

    renderList();
    expect(await screen.findByText("服务器事实标题")).toBeVisible();
    expect(screen.getByText("real-repo")).toBeVisible();
    expect(screen.getByText("Rust", { selector: ".language" })).toBeVisible();
    expect(screen.getByText("2026-07-04 18:30")).toBeVisible();
    expect(screen.queryByText(/\$[0-9]/)).toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });
});
