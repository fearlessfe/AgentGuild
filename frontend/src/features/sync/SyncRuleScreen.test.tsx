import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as client from "../../api/client";
import type { Envelope, SyncRule } from "../../api/client";
import { AppShell } from "../../app/AppShell";
import { OnboardingScreen } from "../onboarding/OnboardingScreen";
import { SyncRuleScreen } from "./SyncRuleScreen";

describe("Navigation and task generation integration", () => {
  beforeEach(() => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
  });

  it("surfaces repository onboarding from the persistent rail", async () => {
    render(
      <MemoryRouter initialEntries={["/generation"]}>
        <AppShell />
      </MemoryRouter>,
    );

    expect(within(screen.getByRole("navigation", { name: "主导航" })).getByRole("link", { name: "仓库接入" })).toHaveAttribute(
      "href",
      "/repositories",
    );
  });

  it("routes /repositories to the repository onboarding screen", async () => {
    render(
      <MemoryRouter initialEntries={["/repositories"]}>
        <AppShell />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "仓库接入" })).toBeVisible();
    expect(screen.getByRole("group", { name: "仓库来源" })).toBeVisible();
  });

  it("keeps GitHub App setup reachable while sending repository setup to /repositories", () => {
    render(
      <MemoryRouter>
        <OnboardingScreen />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "连接 GitHub" })).toHaveAttribute("href", "/git-integration");
    expect(screen.getByRole("link", { name: "设置仓库" })).toHaveAttribute("href", "/repositories");
  });

  it("keeps task generation focused on onboarded repositories and strategy management", async () => {
    render(
      <MemoryRouter>
        <SyncRuleScreen />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("table", { name: "任务生成策略列表" })).toBeVisible();
    expect(screen.getByRole("region", { name: "任务生成策略列表" })).toHaveAttribute("tabindex", "0");
    expect(screen.queryByText("公共仓库同步")).toBeNull();
    expect(screen.getByRole("button", { name: "创建策略" })).toBeEnabled();
    expect(screen.getByRole("link", { name: "仓库接入" })).toHaveAttribute("href", "/repositories");
  });

  it("redirects the legacy /sync route to task generation", async () => {
    render(
      <MemoryRouter initialEntries={["/sync"]}>
        <AppShell />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "任务生成" })).toBeVisible();
  });
});

describe("SyncRuleScreen repository inventory", () => {
  const onboardedRepository = {
    id: "repo-platform-api",
    source_type: "github_app" as const,
    github_app_id: "gha-platform",
    full_name: "company/platform-api",
    default_branch: "main",
    visibility: "private",
  };

  beforeEach(() => {
    delete (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE;
    vi.spyOn(client, "getRepositoryOnboarding").mockResolvedValue(envelope({
      github_app: { id: "gha-platform", app_id: 101, configured: true, is_default: true },
      app_repositories: {
        items: [
          onboardedRepository,
          { full_name: "company/not-onboarded", default_branch: "main", visibility: "private" },
        ],
      },
      onboarded_repositories: { items: [onboardedRepository] },
    }));
    vi.spyOn(client, "listSyncRules").mockResolvedValue(envelope({ items: [] }));
    vi.spyOn(client, "createSyncRule").mockImplementation(async (input) => envelope(syncRule(input.repo)));
    vi.spyOn(client, "runSyncRule").mockResolvedValue(envelope({
      created: 1,
      updated: 0,
      skipped: 0,
      cancelled: 0,
      flagged: 0,
      failed: 0,
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    delete (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE;
  });

  it("creates a rule only from the tenant's onboarded repositories", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <SyncRuleScreen />
      </MemoryRouter>,
    );

    const repo = await screen.findByLabelText("仓库");
    expect(repo).toHaveValue("company/platform-api");
    expect(within(repo).getByRole("option", { name: "company/platform-api" })).toBeVisible();
    expect(within(repo).queryByRole("option", { name: "company/not-onboarded" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("仓库来源")).toHaveValue("GitHub App");
    expect(screen.getByLabelText("Issue 访问")).toHaveValue("app");

    await user.click(screen.getByRole("button", { name: "创建策略" }));
    expect(client.createSyncRule).toHaveBeenCalledWith(
      expect.objectContaining({ repo: "company/platform-api", source_auth: "app" }),
      { idempotencyKey: expect.any(String) },
    );
    expect(await screen.findByRole("heading", { name: "运行结果" })).toBeVisible();
  });
});

function envelope<T>(data: T): Envelope<T> {
  return { data, meta: { server_time: "2026-07-16T00:00:00Z", resource_version: 1 } };
}

function syncRule(repo: string): SyncRule {
  return {
    id: "rule-platform-api",
    repo,
    include_labels: ["agent-task"],
    exclude_labels: ["blocked", "wontfix"],
    issue_state: "open",
    task_type: "coding",
    default_priority: "normal",
    dedupe_strategy: "update",
    source_auth: "app",
    enabled: true,
    created_at: "2026-07-16T00:00:00Z",
    updated_at: "2026-07-16T00:00:00Z",
  };
}
