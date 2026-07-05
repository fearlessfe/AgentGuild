import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { envelope } from "../../api/fixtures";
import { ReputationPage } from "./ReputationPage";
import type { ProjectionView } from "./reputation.types";

function renderWithProviders(ui: ReactNode, { initialEntries = ["/reputation"] }: { initialEntries?: string[] } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    client,
    ...render(
      <MemoryRouter initialEntries={initialEntries}>
        <QueryClientProvider client={client}>{ui}</QueryClientProvider>
      </MemoryRouter>,
    ),
  };
}

function projectionFixture(overrides?: Partial<ProjectionView>): ProjectionView {
  return {
    agent_version_id: "agent-v12",
    capability: "code-review",
    task_type: "typescript",
    total_reviews: 12,
    pass_rate: 0.75,
    rework_rate: 0.17,
    avg_review_cost_cents: 120,
    avg_review_latency_ms: 3450,
    sample_size_hint: "medium",
    ...overrides,
  };
}

function LocationDisplay() {
  const location = useLocation();
  return <span data-testid="location-search">{location.search}</span>;
}

function mockReputationResponse(projection: ProjectionView | ((url: URL) => ProjectionView)) {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
    const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
    const method = (init as RequestInit | undefined)?.method ?? "GET";
    if (url.pathname === "/api/v1/reputation" && method === "GET") {
      const body = typeof projection === "function" ? projection(url) : projection;
      return Promise.resolve(new Response(JSON.stringify(envelope(body)), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify({ error: { message: "not found" } }), { status: 404 }));
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ReputationPage", () => {
  it("renders projection metrics from query params on load", async () => {
    mockReputationResponse(projectionFixture());

    renderWithProviders(
      <Routes>
        <Route path="/reputation" element={<ReputationPage />} />
      </Routes>,
      { initialEntries: ["/reputation?agent_version_id=agent-v12&capability=code-review&task_type=typescript"] },
    );

    expect(await screen.findByText("12")).toBeVisible();
    expect(screen.getByText("75.0%")).toBeVisible();
    expect(screen.getByText("17.0%")).toBeVisible();
    expect(screen.getByText("$1.20")).toBeVisible();
    expect(screen.getByText("3.45 s")).toBeVisible();
    expect(screen.getByText("中")).toBeVisible();
  });

  it("submits user-entered filters and updates the URL", async () => {
    const fetchMock = mockReputationResponse(projectionFixture({ agent_version_id: "agent-v2" }));

    renderWithProviders(
      <Routes>
        <Route path="/reputation" element={<>
          <ReputationPage />
          <LocationDisplay />
        </>} />
      </Routes>,
    );

    await userEvent.type(screen.getByLabelText(/Agent Version ID/i), "agent-v2");
    await userEvent.type(screen.getByLabelText(/Capability/i), "code-review");
    await userEvent.type(screen.getByLabelText(/Task Type/i), "go");
    await userEvent.click(screen.getByRole("button", { name: /查询/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const requestUrl = new URL(String(fetchMock.mock.calls[0][0]), "http://localhost");
    expect(requestUrl.pathname).toBe("/api/v1/reputation");
    expect(requestUrl.searchParams.get("agent_version_id")).toBe("agent-v2");
    expect(requestUrl.searchParams.get("capability")).toBe("code-review");
    expect(requestUrl.searchParams.get("task_type")).toBe("go");

    expect(screen.getByTestId("location-search").textContent).toBe(
      "?agent_version_id=agent-v2&capability=code-review&task_type=go",
    );
  });

  it("reflects URL changes when navigating back or externally", async () => {
    function Navigation() {
      const navigate = useNavigate();
      return (
        <button onClick={() => navigate("/reputation?agent_version_id=agent-v2&capability=code-review&task_type=go")}>
          切换 URL
        </button>
      );
    }

    const fetchMock = mockReputationResponse((url) => {
      const agentVersionId = url.searchParams.get("agent_version_id");
      return projectionFixture({
        agent_version_id: agentVersionId ?? "agent-v12",
        total_reviews: agentVersionId === "agent-v2" ? 99 : 12,
      });
    });

    renderWithProviders(
      <Routes>
        <Route path="/reputation" element={<>
          <ReputationPage />
          <Navigation />
          <LocationDisplay />
        </>} />
      </Routes>,
      { initialEntries: ["/reputation?agent_version_id=agent-v12&capability=code-review&task_type=typescript"] },
    );

    expect(await screen.findByText("12")).toBeVisible();
    expect(screen.getByTestId("location-search").textContent).toBe(
      "?agent_version_id=agent-v12&capability=code-review&task_type=typescript",
    );

    await userEvent.click(screen.getByRole("button", { name: /切换 URL/i }));

    await waitFor(() => expect(screen.getByText("99")).toBeVisible());
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const requestUrl = new URL(String(fetchMock.mock.calls[1][0]), "http://localhost");
    expect(requestUrl.searchParams.get("agent_version_id")).toBe("agent-v2");
    expect(screen.getByTestId("location-search").textContent).toBe(
      "?agent_version_id=agent-v2&capability=code-review&task_type=go",
    );
  });
  it("shows a placeholder when no filters are committed", async () => {
    renderWithProviders(
      <Routes>
        <Route path="/reputation" element={<ReputationPage />} />
      </Routes>,
    );

    expect(screen.getByText(/输入 Agent Version ID、Capability 与 Task Type/i)).toBeVisible();
    expect(screen.queryByText(/正在加载/)).toBeNull();
  });

  it("renders an error message when the API fails", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ error: { message: "权限不足" } }), { status: 403 })),
    );

    renderWithProviders(
      <Routes>
        <Route path="/reputation" element={<ReputationPage />} />
      </Routes>,
      { initialEntries: ["/reputation?agent_version_id=agent-v12&capability=code-review&task_type=typescript"] },
    );

    expect(await screen.findByText(/加载失败：权限不足/i)).toBeVisible();
  });

  it("formats missing optional fields as em-dash", async () => {
    mockReputationResponse(
      projectionFixture({ pass_rate: undefined, rework_rate: undefined, avg_review_cost_cents: undefined, avg_review_latency_ms: undefined }),
    );

    renderWithProviders(
      <Routes>
        <Route path="/reputation" element={<ReputationPage />} />
      </Routes>,
      { initialEntries: ["/reputation?agent_version_id=agent-v12&capability=code-review&task_type=typescript"] },
    );

    await waitFor(() => expect(screen.getByText("12")).toBeVisible());
    const missing = screen.getAllByText("—");
    expect(missing.length).toBeGreaterThanOrEqual(4);
  });
});
