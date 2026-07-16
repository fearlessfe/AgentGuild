import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { envelope, executionViewFixture, taskViewFixture } from "../../api/fixtures";
import { OutcomeScreen } from "./OutcomeScreen";

describe("OutcomeScreen", () => {
  it("renders the accepted result from the retained execution relationship", async () => {
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
        <MemoryRouter initialEntries={["/outcome?task_id=task-1"]}>
          <OutcomeScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("已接受")).toBeVisible();
    expect(screen.getByText("accepted")).toBeVisible();
    expect(screen.getByText("Atlas v12")).toBeVisible();
  });
});
