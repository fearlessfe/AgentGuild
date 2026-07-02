import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TaskList } from "./TaskList";

function renderList() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<MemoryRouter><QueryClientProvider client={queryClient}><TaskList /></QueryClientProvider></MemoryRouter>);
}

describe("TaskList", () => {
  it("renders grouped read-only tasks and forwards the opaque cursor unchanged", async () => {
    const cursor = "eyJ0IjoidGVuYW50In0.signature/opaque+part";
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({
        data: { items: [
          { id: "AG-192", title: "修复批量退款时的余额竞争条件", type: "code", status: "open", deadline: "2026-07-03T10:00:00Z", created_at: "2026-07-02T01:00:00Z", updated_at: "2026-07-02T01:00:00Z", publisher_agent_version_id: "billing", tenant_id: "tenant", problem: "并发退款", constraints: [], requirements: [], state_version: 1 },
          { id: "AG-188", title: "修复定时任务重复触发问题", type: "code", status: "in_progress", deadline: "2026-07-03T10:00:00Z", created_at: "2026-07-02T01:00:00Z", updated_at: "2026-07-02T01:00:00Z", publisher_agent_version_id: "billing", tenant_id: "tenant", problem: "重复触发", constraints: [], requirements: [], state_version: 1 },
        ] },
        meta: { server_time: "2026-07-02T02:00:00Z", resource_version: 1, poll_after_seconds: 13, next_cursor: cursor },
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { items: [] }, meta: { server_time: "2026-07-02T02:00:13Z", resource_version: 1 } }), { status: 200 }));

    renderList();
    expect(await screen.findByText("待领取 · 1")).toBeVisible();
    expect(screen.getByText("进行中 · 1")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const requested = new URL(String(fetchMock.mock.calls[1][0]), "http://localhost");
    expect(requested.searchParams.get("cursor")).toBe(cursor);
    expect(screen.queryByRole("button", { name: /claim|accept|set status/i })).toBeNull();
  });
});
