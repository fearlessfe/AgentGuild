import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PublicNetworkRouter } from "./PublicNetwork";

function renderNetwork(initialEntries: string[] = ["/network"]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<MemoryRouter initialEntries={initialEntries}><QueryClientProvider client={queryClient}><PublicNetworkRouter /></QueryClientProvider></MemoryRouter>);
}

function response(data: unknown) {
  return new Response(JSON.stringify({ data, meta: { server_time: "2026-09-01T10:00:00Z", resource_version: 1 } }), { status: 200 });
}

afterEach(() => vi.restoreAllMocks());

describe("PublicNetworkRouter", () => {
  it("routes visitors from the network hub to public tasks and agents", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const path = new URL(String(input), "http://localhost").pathname;
      if (path === "/api/v1/public/tasks") return response({ items: [{ id: "AG-1", task_specification_version_id: "spec-1", canonical_repository: "acme/api", source_issue_url: "", title: "Repair retries", summary: "Make retries safe", quality_level: "standard", published_at: "2026-09-01T09:00:00Z", can_claim: true }] });
      return response({ items: [{ agent_id: "agent-1", agent_version_id: "version-1", handle: "atlas", display_name: "Atlas", status: "active", organization_id: "public", capabilities: ["code"], created_at: "2026-09-01T08:00:00Z" }] });
    });

    renderNetwork();

    expect(await screen.findByRole("heading", { name: /真实工作/ })).toBeVisible();
    expect(await screen.findByRole("link", { name: /Repair retries/ })).toHaveAttribute("href", "/network/tasks/AG-1");
    expect(screen.getAllByRole("link", { name: /查看全部/ }).find((link) => link.getAttribute("href") === "/network/tasks")).toBeTruthy();
    expect(screen.getAllByRole("link", { name: /查看全部/ }).find((link) => link.getAttribute("href") === "/network/agents")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /tasks$/ })).toBeNull();
  });

  it("loads public task detail without entering the internal workbench", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(response({
      id: "AG-1", task_specification_version_id: "spec-1", canonical_repository: "acme/api", source_issue_url: "",
      title: "Repair retries", summary: "Make retries safe", quality_level: "standard", published_at: "2026-09-01T09:00:00Z", can_claim: true,
      issue_revision: "main", base_commit: "abc", problem_diagnosis: "Retries duplicate writes", impact: "Safer writes", proposed_solution: "Add idempotency", implementation_steps: ["Test", "Fix"], constraints: [], non_goals: [], risks: [], acceptance_criteria: [], evidence_refs: [],
    }));

    renderNetwork(["/network/tasks/AG-1"]);

    expect(await screen.findByRole("heading", { name: "Repair retries" })).toBeVisible();
    expect(screen.getByText("Retries duplicate writes")).toBeVisible();
    expect(screen.getByRole("button", { name: /复制任务链接/ })).toBeVisible();
  });
});
