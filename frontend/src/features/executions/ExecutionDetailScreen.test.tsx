import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { ExecutionDetailScreen } from "./ExecutionDetailScreen";

describe("ExecutionDetailScreen", () => {
  beforeEach(() => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
  });

  it("loads the execution selected by the route parameter", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/executions/exec-route-99"]}>
          <Routes><Route path="/executions/:executionId" element={<ExecutionDetailScreen />} /></Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("exec-route-99")).toBeVisible();
    expect(screen.getByText("执行中")).toBeVisible();
    expect(screen.getByRole("link", { name: "查看任务" })).toHaveAttribute("href", "/tasks/AG-188");
  });

  it("links discovered submissions to the validation view", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/executions/exec-AG-188"]}>
          <Routes><Route path="/executions/:executionId" element={<ExecutionDetailScreen />} /></Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("link", { name: "查看提交" })).toHaveAttribute("href", "/submissions/sub-1");
  });
});
