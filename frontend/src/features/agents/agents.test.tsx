import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { envelope } from "../../api/fixtures";
import { AgentDetail } from "./AgentDetail";
import { AgentList } from "./AgentList";
import { AgentRegister } from "./AgentRegister";

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

describe("Agents UI", () => {
  it("reveals activation token only through onRegistered after registration", async () => {
    const onRegistered = vi.fn();
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          envelope({
            agent: {
              id: "agent-7",
              name: "Code Review Bot",
              status: "pending_activation",
              owner_email: "review@example.com",
              scopes: ["tasks:read"],
              created_at: "2026-07-02T01:00:00Z",
            },
            token: "agtok_once_only",
            expires_at: "2026-07-09T01:00:00Z",
          }),
        ),
        { status: 200 },
      ),
    );

    renderWithProviders(<AgentRegister onRegistered={onRegistered} />, { initialEntries: ["/agents/new"] });

    await userEvent.type(screen.getByLabelText(/名称/i), "Code Review Bot");
    await userEvent.type(screen.getByLabelText(/Owner Email/i), "review@example.com");
    await userEvent.click(screen.getByRole("button", { name: /注册 agent/i }));

    await waitFor(() =>
      expect(onRegistered).toHaveBeenCalledWith(
        expect.objectContaining({
          token: "agtok_once_only",
        }),
      ),
    );
    expect(screen.queryByText("agtok_once_only")).toBeNull();
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
            scopes: ["tasks:read", "tasks:write"],
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

  it("filters the list by status", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
      const status = url.searchParams.get("status");
      const items =
        status === "suspended"
          ? [
              {
                id: "agent-2",
                name: "Suspended Worker",
                status: "suspended",
                owner_email: "ops@example.com",
                team: "Ops",
                scopes: ["tasks:read"],
                created_at: "2026-07-02T01:00:00Z",
              },
            ]
          : [
              {
                id: "agent-1",
                name: "Active Worker",
                status: "active",
                owner_email: "eng@example.com",
                team: "Eng",
                scopes: ["tasks:read"],
                created_at: "2026-07-02T01:00:00Z",
              },
              {
                id: "agent-2",
                name: "Suspended Worker",
                status: "suspended",
                owner_email: "ops@example.com",
                team: "Ops",
                scopes: ["tasks:read"],
                created_at: "2026-07-02T01:00:00Z",
              },
            ];
      return Promise.resolve(new Response(JSON.stringify(envelope({ items })), { status: 200 }));
    });

    renderWithProviders(
      <Routes>
        <Route path="/agents" element={<AgentList />} />
      </Routes>,
    );

    expect(await screen.findByText("Active Worker")).toBeVisible();
    expect(screen.getByText("Suspended Worker")).toBeVisible();

    await userEvent.selectOptions(screen.getByRole("combobox", { name: /状态/i }), "suspended");

    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => new URL(String(call[0]), "http://localhost"));
      expect(calls.at(-1)?.searchParams.get("status")).toBe("suspended");
    });
    expect(await screen.findByText("Suspended Worker")).toBeVisible();
    expect(screen.queryByText("Active Worker")).toBeNull();
  });
});
