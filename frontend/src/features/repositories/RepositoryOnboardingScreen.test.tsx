import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as client from "../../api/client";
import type { RepositoryInventoryItem, RepositoryOnboardingSummary } from "../../api/client";
import { RepositoryOnboardingScreen } from "./RepositoryOnboardingScreen";

describe("RepositoryOnboardingScreen", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
  });

  it("renders the two-step onboarding areas and added repositories from demo mode", async () => {
    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByRole("heading", { name: "仓库接入" })).toBeVisible();
    expect(screen.getByText("Step 1 · GitHub App")).toBeVisible();
    expect(screen.getByText("Step 2 · 添加仓库")).toBeVisible();
    expect(await screen.findByText("acme/event-gateway")).toBeVisible();

    const table = await screen.findByRole("table", { name: "已接入仓库列表" });
    expect(within(table).getByText("acme/billing-service")).toBeVisible();
    expect(within(table).getByText("vercel/next.js")).toBeVisible();
    expect(within(table).getByText("GitHub App")).toBeVisible();
    expect(within(table).getByText("公开仓库")).toBeVisible();
  });

  it("adds a GitHub App repository candidate to the added repository list", async () => {
    render(<RepositoryOnboardingScreen />);

    const candidateRow = await screen.findByRole("row", { name: /acme\/event-gateway/ });
    await userEvent.click(within(candidateRow).getByRole("button", { name: "添加" }));

    const table = await screen.findByRole("table", { name: "已接入仓库列表" });
    await waitFor(() => expect(within(table).getByText("acme/event-gateway")).toBeVisible());
  });

  it("adds a public GitHub URL without creating an issue sync rule", async () => {
    const createSyncRuleSpy = vi.spyOn(client, "createSyncRule");

    render(<RepositoryOnboardingScreen />);

    await userEvent.type(await screen.findByLabelText("公共仓库 URL 或 owner/repo"), "https://github.com/rust-lang/rust");
    await userEvent.click(screen.getByRole("button", { name: "添加公开仓库" }));

    const table = await screen.findByRole("table", { name: "已接入仓库列表" });
    await waitFor(() => expect(within(table).getByText("rust-lang/rust")).toBeVisible());
    expect(createSyncRuleSpy).not.toHaveBeenCalled();
  });

  it("shows a recovery state when GitHub App repository listing fails", async () => {
    vi.spyOn(client, "getRepositoryOnboarding").mockResolvedValue({
      data: summaryFixture({
        app_repositories: { items: [] },
        app_repositories_error: "GitHub App installation requires re-authentication",
      }),
      meta: { server_time: "", resource_version: 0 },
    });

    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByRole("alert")).toHaveTextContent("GitHub App installation requires re-authentication");
    expect(screen.getByRole("button", { name: "打开 Git 接入" })).toBeVisible();
  });

  it("shows an empty state when a configured GitHub App has no visible repositories", async () => {
    vi.spyOn(client, "getRepositoryOnboarding").mockResolvedValue({
      data: summaryFixture({ app_repositories: { items: [] } }),
      meta: { server_time: "", resource_version: 0 },
    });

    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByText("暂无 GitHub App 可见仓库")).toBeVisible();
  });

  it("keeps public and GitHub App entries separate for the same full name", async () => {
    const publicRepo: RepositoryInventoryItem = {
      id: "public-acme-shared",
      source_type: "public_github",
      full_name: "acme/shared",
      default_branch: "main",
      visibility: "public",
    };
    const appRepo: RepositoryInventoryItem = {
      id: "app-acme-shared",
      source_type: "github_app",
      full_name: "acme/shared",
      default_branch: "main",
      visibility: "private",
    };
    vi.spyOn(client, "getRepositoryOnboarding").mockResolvedValue({
      data: summaryFixture({
        app_repositories: { items: [appRepo] },
        onboarded_repositories: { items: [publicRepo] },
      }),
      meta: { server_time: "", resource_version: 0 },
    });
    vi.spyOn(client, "addGitHubAppRepository").mockResolvedValue({
      data: appRepo,
      meta: { server_time: "", resource_version: 1 },
    });

    render(<RepositoryOnboardingScreen />);

    const candidateTable = await screen.findByRole("table", { name: "GitHub App 候选仓库列表" });
    const candidateRow = within(candidateTable).getByRole("row", { name: /acme\/shared/ });
    const addButton = within(candidateRow).getByRole("button", { name: "添加" });
    expect(addButton).toBeEnabled();

    await userEvent.click(addButton);

    const onboardedTable = await screen.findByRole("table", { name: "已接入仓库列表" });
    await waitFor(() => expect(within(onboardedTable).getAllByText("acme/shared")).toHaveLength(2));
    expect(within(onboardedTable).getByText("公开仓库")).toBeVisible();
    expect(within(onboardedTable).getByText("GitHub App")).toBeVisible();
  });
});

function summaryFixture(overrides: Partial<RepositoryOnboardingSummary> = {}): RepositoryOnboardingSummary {
  return {
    github_app: {
      app_id: "123",
      app_slug: "agentguild-test",
      configured: true,
    },
    app_repositories: { items: [] },
    onboarded_repositories: { items: [] },
    ...overrides,
  };
}
