import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { agentPageFixture, agentViewFixture, envelope } from "../../api/fixtures";
import { AppShell } from "../../app/AppShell";
import { AgentDetail } from "./AgentDetail";
import { AgentList } from "./AgentList";

function renderWithProviders(
  ui: ReactNode,
  { initialEntries = ["/agents"] }: { initialEntries?: string[] } = {},
) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Agents UI", () => {
  it("reveals activation token from the backend register response shape", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          envelope({
            agent: agentViewFixture({
              id: "agent-7",
              name: "Code Review Bot",
              status: "pending_activation",
              owner_email: "review@example.com",
            }),
            activation_token: "agtok_once_only",
            activation_expires_at: "2026-07-09T01:00:00Z",
          }),
        ),
        { status: 200 },
      ),
    );

    renderWithProviders(<AppShell />, { initialEntries: ["/agents/new"] });

    expect(screen.queryByLabelText(/owner email/i)).toBeNull();
    await userEvent.type(screen.getByLabelText(/名称/i), "Code Review Bot");
    await userEvent.click(screen.getByRole("button", { name: /注册 agent/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const payload = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    expect(payload).not.toHaveProperty("owner_email");
    expect(payload.scopes).toEqual(["tasks:publish", "tasks:claim", "tasks:execute", "tasks:read"]);
    expect(await screen.findByText("agtok_once_only")).toBeVisible();
    expect(screen.getByText(/过期时间：2026-07-09T01:00:00Z/i)).toBeVisible();
  });

  it("hides suspend and resume controls for revoked agents", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          envelope({
            id: "agent-1",
            name: "Atlas v12",
            status: "revoked",
            owner_email: "owner@example.com",
            scopes: ["tasks:read", "tasks:execute"],
            created_at: "2026-07-02T01:00:00Z",
          }),
        ),
        { status: 200 },
      ),
    );

    renderWithProviders(<AgentDetail agentId="agent-1" />);

    await waitFor(() => expect(screen.getAllByText(/revoked/i).length).toBeGreaterThan(0));
    expect(screen.queryByRole("button", { name: /暂停 agent/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /恢复 agent/i })).toBeNull();
  });

  it("does not send a status query param and filters rows client-side", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            envelope(
              agentPageFixture([
                agentViewFixture({
                  id: "agent-1",
                  name: "Active Worker",
                  status: "active",
                  owner_email: "eng@example.com",
                  team: "Eng",
                }),
                agentViewFixture({
                  id: "agent-2",
                  name: "Suspended Worker",
                  status: "suspended",
                  owner_email: "ops@example.com",
                  team: "Ops",
                }),
              ]),
            ),
          ),
          { status: 200 },
        ),
      ),
    );

    renderWithProviders(
      <Routes>
        <Route path="/agents" element={<AgentList />} />
      </Routes>,
    );

    const desktopAgentList = await screen.findByLabelText("Agent 列表");

    expect(await within(desktopAgentList).findByText("Active Worker")).toBeVisible();
    expect(within(desktopAgentList).getByText("Suspended Worker")).toBeVisible();

    await userEvent.selectOptions(screen.getByRole("combobox", { name: /状态/i }), "suspended");

    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => new URL(String(call[0]), "http://localhost"));
      expect(calls.every((url) => !url.searchParams.has("status"))).toBe(true);
    });
    expect(within(desktopAgentList).getByText("Suspended Worker")).toBeVisible();
    expect(within(desktopAgentList).queryByText("Active Worker")).toBeNull();
  });

  it("uses colon action routes for agent status mutations", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch");
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify(envelope(agentViewFixture())), { status: 200 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify(envelope(agentViewFixture({ status: "suspended", updated_at: "2026-07-02T03:00:00Z" }))), {
          status: 200,
        }),
      );

    renderWithProviders(<AgentDetail agentId="agent-1" />);

    await screen.findByText("Atlas v12");
    await userEvent.click(screen.getByRole("button", { name: /暂停 agent/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(new URL(String(fetchMock.mock.calls[1]?.[0]), "http://localhost").pathname).toBe("/api/v1/agents/agent-1:suspend");
  });

  it("keeps pending activation agents read-only for suspend and resume controls", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          envelope(
            agentViewFixture({
              id: "agent-pending",
              name: "Review Draft",
              status: "pending_activation",
              owner_email: "review@example.com",
            }),
          ),
        ),
        { status: 200 },
      ),
    );

    renderWithProviders(<AgentDetail agentId="agent-pending" />);

    await waitFor(() => expect(screen.getAllByText(/pending activation/i).length).toBeGreaterThan(0));
    expect(screen.queryByRole("button", { name: /暂停 agent/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /恢复 agent/i })).toBeNull();
  });
});
