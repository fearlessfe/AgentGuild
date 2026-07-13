import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as client from "../../api/client";
import { GitIntegrationScreen } from "./GitIntegrationScreen";

const envelope = <T,>(data: T): client.Envelope<T> => ({
  data,
  meta: { server_time: "", resource_version: 0 },
});

const alphaApp: client.GitHubAppView = {
  id: "gha-alpha",
  app_id: 123,
  installation_id: 456,
  app_slug: "alpha",
  installation_account_login: "acme-corp",
  is_default: true,
  configured: true,
};

const betaApp: client.GitHubAppView = {
  id: "gha-beta",
  app_id: 789,
  installation_id: 987,
  app_slug: "beta",
  installation_account_login: "labs",
  is_default: false,
  configured: true,
};

describe("GitIntegrationScreen", () => {
  const originalLocation = window.location;

  beforeEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: { ...originalLocation, href: "" },
    });
    vi.spyOn(client, "listGitHubApps").mockResolvedValue(envelope({ items: [alphaApp, betaApp] }));
    vi.spyOn(client, "testGitHubApp").mockResolvedValue(envelope({ ok: true, repo_count: 2 }));
    vi.spyOn(client, "deleteGitHubApp").mockResolvedValue(envelope({ deleted: true }));
    vi.spyOn(client, "githubManifestUrl").mockReturnValue("/api/oauth/github/app/manifest");
    vi.spyOn(client, "githubInstallUrl").mockImplementation(
      (id) => `/api/oauth/github/app/install?github_app_id=${encodeURIComponent(id ?? "")}`,
    );
    vi.spyOn(window, "confirm").mockReturnValue(true);
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
    vi.restoreAllMocks();
  });

  it("lists every App with its automatic label and one global add action", async () => {
    render(<GitIntegrationScreen />);

    expect(await screen.findByText("alpha · acme-corp")).toBeVisible();
    expect(screen.getByText("beta · labs")).toBeVisible();
    expect(screen.getByRole("button", { name: "新增 GitHub App" })).toBeVisible();
  });

  it("shows only a retryable error state when loading Apps fails", async () => {
    vi.mocked(client.listGitHubApps)
      .mockRejectedValueOnce(new Error("网络不可用"))
      .mockResolvedValueOnce(envelope({ items: [alphaApp] }));
    const user = userEvent.setup();

    render(<GitIntegrationScreen />);

    expect(await screen.findByRole("alert")).toHaveTextContent("网络不可用");
    expect(screen.queryByText(/尚未配置 GitHub App/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试加载 GitHub App" }));
    expect(await screen.findByText("alpha · acme-corp")).toBeVisible();
    expect(client.listGitHubApps).toHaveBeenCalledTimes(2);
  });

  it("tests and deletes the selected GitHub App by id", async () => {
    vi.mocked(client.listGitHubApps)
      .mockResolvedValueOnce(envelope({ items: [alphaApp, betaApp] }))
      .mockResolvedValueOnce(envelope({ items: [alphaApp] }));
    const user = userEvent.setup();
    render(<GitIntegrationScreen />);

    await user.click(await screen.findByRole("button", { name: "检测 beta" }));
    expect(client.testGitHubApp).toHaveBeenCalledWith("gha-beta");
    expect(await screen.findByText("连接成功 · 可访问 2 个仓库")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "删除 beta" }));
    expect(client.deleteGitHubApp).toHaveBeenCalledWith("gha-beta");
    expect(screen.queryByText("beta · labs")).not.toBeInTheDocument();
    expect(screen.getByText("alpha · acme-corp")).toBeVisible();
  });

  it("keeps the current list visible while refreshing the promoted default after deletion", async () => {
    let resolveRefresh!: (value: client.Envelope<{ items: client.GitHubAppView[] }>) => void;
    vi.mocked(client.listGitHubApps)
      .mockResolvedValueOnce(envelope({ items: [alphaApp, betaApp] }))
      .mockReturnValueOnce(new Promise((resolve) => {
        resolveRefresh = resolve;
      }));
    const user = userEvent.setup();
    render(<GitIntegrationScreen />);

    await user.click(await screen.findByRole("button", { name: "删除 alpha" }));

    await waitFor(() => expect(client.listGitHubApps).toHaveBeenCalledTimes(2));
    expect(screen.getByText("alpha · acme-corp")).toBeVisible();
    expect(screen.getByRole("button", { name: "正在删除 alpha" })).toBeDisabled();
    expect(screen.queryByText(/尚未配置 GitHub App/)).not.toBeInTheDocument();

    resolveRefresh(envelope({ items: [{ ...betaApp, is_default: true }] }));

    expect(await screen.findByRole("status", { name: "GitHub App 操作结果" }))
      .toHaveTextContent("已删除 GitHub App alpha");
    expect(screen.queryByText("alpha · acme-corp")).not.toBeInTheDocument();
    const betaCard = screen.getByText("beta · labs").closest(".provider-card");
    expect(betaCard).not.toBeNull();
    expect(within(betaCard as HTMLElement).getByText("默认 GitHub App")).toBeVisible();
  });

  it("exposes an App-scoped busy testing state without disabling other cards", async () => {
    let resolveTest!: (value: client.Envelope<client.ConnectionTestResult>) => void;
    vi.mocked(client.testGitHubApp).mockReturnValueOnce(new Promise((resolve) => {
      resolveTest = resolve;
    }));
    const user = userEvent.setup();
    render(<GitIntegrationScreen />);

    await user.click(await screen.findByRole("button", { name: "检测 beta" }));

    const pending = screen.getByRole("button", { name: "正在检测 beta" });
    expect(pending).toBeDisabled();
    expect(pending).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "检测 alpha" })).toBeEnabled();

    resolveTest(envelope({ ok: true, repo_count: 2 }));
    await waitFor(() => expect(screen.getByRole("button", { name: "检测 beta" })).toBeEnabled());
  });

  it("announces an App-scoped busy deletion and its successful completion", async () => {
    let resolveDelete!: (value: client.Envelope<{ deleted: boolean }>) => void;
    vi.mocked(client.deleteGitHubApp).mockReturnValueOnce(new Promise((resolve) => {
      resolveDelete = resolve;
    }));
    const user = userEvent.setup();
    render(<GitIntegrationScreen />);

    await user.click(await screen.findByRole("button", { name: "删除 beta" }));

    const pending = screen.getByRole("button", { name: "正在删除 beta" });
    expect(pending).toBeDisabled();
    expect(pending).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "删除 alpha" })).toBeEnabled();

    resolveDelete(envelope({ deleted: true }));
    expect(await screen.findByRole("status", { name: "GitHub App 操作结果" }))
      .toHaveTextContent("已删除 GitHub App beta");
  });

  it("installs only the selected pending App", async () => {
    vi.mocked(client.listGitHubApps).mockResolvedValue(
      envelope({ items: [{ ...alphaApp, installation_id: undefined, installation_account_login: undefined }, betaApp] }),
    );
    const user = userEvent.setup();
    render(<GitIntegrationScreen />);

    const pendingCard = (await screen.findByText("alpha · 待选择安装账户")).closest(".provider-card");
    expect(pendingCard).not.toBeNull();
    expect(within(pendingCard as HTMLElement).getByText("已创建，待安装")).toBeVisible();
    await user.click(within(pendingCard as HTMLElement).getByRole("button", { name: "安装 alpha" }));

    expect(client.githubInstallUrl).toHaveBeenCalledWith("gha-alpha");
    expect(window.location.href).toBe("/api/oauth/github/app/install?github_app_id=gha-alpha");
  });

  it("keeps an App visible and announces a repository-binding delete conflict", async () => {
    vi.mocked(client.deleteGitHubApp).mockRejectedValueOnce(
      new Error("该 GitHub App 仍绑定已接入仓库，请先移除仓库"),
    );
    const user = userEvent.setup();
    render(<GitIntegrationScreen />);

    await user.click(await screen.findByRole("button", { name: "删除 alpha" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("该 GitHub App 仍绑定已接入仓库，请先移除仓库");
    expect(screen.getByText("alpha · acme-corp")).toBeVisible();
  });
});
