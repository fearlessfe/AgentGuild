import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as client from "../../api/client";
import type { GitHubAppView, Repository, RepositoryInventoryItem, RepositoryOnboardingSummary } from "../../api/client";
import { RepositoryOnboardingScreen } from "./RepositoryOnboardingScreen";

const alphaApp: GitHubAppView = {
  id: "gha-alpha",
  app_id: 101,
  app_slug: "agentguild-alpha",
  installation_id: 1001,
  installation_account_login: "acme",
  is_default: true,
  configured: true,
};
const betaApp: GitHubAppView = {
  id: "gha-beta",
  app_id: 202,
  app_slug: "agentguild-beta",
  installation_id: 2002,
  installation_account_login: "platform",
  is_default: false,
  configured: true,
};
const pendingApp: GitHubAppView = {
  id: "gha-pending",
  app_id: 303,
  app_slug: "agentguild-pending",
  is_default: false,
  configured: true,
};
const apiRepo: Repository = { full_name: "acme/api", default_branch: "main", visibility: "private" };
const webRepo: Repository = { full_name: "acme/web", default_branch: "develop", visibility: "private" };

describe("RepositoryOnboardingScreen", () => {
  beforeEach(() => {
    vi.spyOn(client, "listGitHubApps").mockResolvedValue(envelope({ items: [alphaApp, betaApp, pendingApp] }));
    vi.spyOn(client, "getRepositoryOnboarding").mockResolvedValue(envelope(summaryFixture()));
    vi.spyOn(client, "listGitHubAppRepositories").mockResolvedValue(envelope({ items: [apiRepo, webRepo] }));
    vi.spyOn(client, "addGitHubAppRepository").mockImplementation(async (appID, fullName) =>
      envelope({
        id: `repo-${fullName}`,
        source_type: "github_app",
        github_app_id: appID,
        ...(fullName === webRepo.full_name ? webRepo : apiRepo),
      }),
    );
    vi.spyOn(client, "addPublicRepository").mockResolvedValue(
      envelope({
        id: "repo-rust",
        source_type: "public_github",
        full_name: "rust-lang/rust",
        default_branch: "master",
        visibility: "public",
      }),
    );
    vi.spyOn(client, "removeRepository").mockResolvedValue(envelope({ deleted: true }));
  });

  it("loads plural Apps and only offers installed Apps with slug and account labels", async () => {
    render(<RepositoryOnboardingScreen />);

    const appSelect = await screen.findByLabelText("GitHub App");
    expect(within(appSelect).getByRole("option", { name: "agentguild-alpha · acme" })).toBeVisible();
    expect(within(appSelect).getByRole("option", { name: "agentguild-beta · platform" })).toBeVisible();
    expect(within(appSelect).queryByRole("option", { name: /pending/ })).not.toBeInTheDocument();
    expect(client.listGitHubApps).toHaveBeenCalledOnce();
    expect(client.getRepositoryOnboarding).toHaveBeenCalledOnce();
    expect(screen.queryByRole("table", { name: "GitHub App 候选仓库列表" })).not.toBeInTheDocument();
  });

  it("loads independent resources under React StrictMode", async () => {
    render(<StrictMode><RepositoryOnboardingScreen /></StrictMode>);

    expect(await screen.findByText("0 个仓库")).toBeVisible();
    expect(screen.getByLabelText("GitHub App")).toBeVisible();
  });

  it("filters repositories locally without another request", async () => {
    const user = userEvent.setup();
    render(<RepositoryOnboardingScreen />);
    await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-beta");
    const input = await screen.findByRole("combobox", { name: "授权仓库" });
    await user.type(input, "WEB");

    expect(screen.getByRole("option", { name: "acme/web" })).toBeVisible();
    expect(screen.queryByRole("option", { name: "acme/api" })).not.toBeInTheDocument();
    expect(client.listGitHubAppRepositories).toHaveBeenCalledTimes(1);
  });

  it("lets the repository list escape only the add card and selects a later option", async () => {
    const user = userEvent.setup();
    render(<RepositoryOnboardingScreen />);
    await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-alpha");
    const input = await screen.findByRole("combobox", { name: "授权仓库" });
    const addCard = screen.getByText("添加仓库", { selector: ".card-title" }).closest("section");

    expect(addCard).toHaveClass("repository-add-card");
    expect(screen.getByRole("table", { name: "已接入仓库列表" }).closest("section")).not.toHaveClass("repository-add-card");
    await user.click(input);
    await user.click(screen.getByRole("option", { name: "acme/web" }));
    expect(input).toHaveValue("acme/web");
  });

  it("clears repository query and selection when switching Apps", async () => {
    const user = userEvent.setup();
    render(<RepositoryOnboardingScreen />);
    const appSelect = await screen.findByLabelText("GitHub App");
    await user.selectOptions(appSelect, "gha-alpha");
    const input = await screen.findByRole("combobox", { name: "授权仓库" });
    await user.click(input);
    await user.click(screen.getByRole("option", { name: "acme/web" }));
    expect(screen.getByText(/默认分支：develop/)).toBeVisible();

    await user.selectOptions(appSelect, "gha-beta");

    expect(await screen.findByRole("combobox", { name: "授权仓库" })).toHaveValue("");
    expect(screen.queryByText(/默认分支：develop/)).not.toBeInTheDocument();
  });

  it("reuses successful per-App repository results", async () => {
    const user = userEvent.setup();
    render(<RepositoryOnboardingScreen />);
    const appSelect = await screen.findByLabelText("GitHub App");

    await user.selectOptions(appSelect, "gha-alpha");
    await screen.findByRole("combobox", { name: "授权仓库" });
    await user.selectOptions(appSelect, "gha-beta");
    await waitFor(() => expect(client.listGitHubAppRepositories).toHaveBeenCalledTimes(2));
    await user.selectOptions(appSelect, "gha-alpha");
    await user.click(screen.getByRole("combobox", { name: "授权仓库" }));

    expect(screen.getByRole("option", { name: "acme/api" })).toBeVisible();
    expect(client.listGitHubAppRepositories).toHaveBeenCalledTimes(2);
  });

  it("does not cache failures and retries the selected App locally", async () => {
    const user = userEvent.setup();
    vi.mocked(client.listGitHubAppRepositories)
      .mockRejectedValueOnce(new Error("installation token expired"))
      .mockResolvedValueOnce(envelope({ items: [webRepo] }));
    render(<RepositoryOnboardingScreen />);
    await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-beta");

    expect(await screen.findByRole("alert")).toHaveTextContent("installation token expired");
    await user.click(screen.getByRole("button", { name: "重试加载仓库" }));

    await user.click(await screen.findByRole("combobox", { name: "授权仓库" }));
    expect(screen.getByRole("option", { name: "acme/web" })).toBeVisible();
    expect(client.listGitHubAppRepositories).toHaveBeenCalledTimes(2);
  });

  it("ignores a stale slower App response after switching Apps", async () => {
    const user = userEvent.setup();
    let resolveAlpha!: (value: ReturnType<typeof envelope<{ items: Repository[] }>>) => void;
    vi.mocked(client.listGitHubAppRepositories).mockImplementation((appID) => {
      if (appID === "gha-alpha") return new Promise((resolve) => (resolveAlpha = resolve));
      return Promise.resolve(envelope({ items: [webRepo] }));
    });
    render(<RepositoryOnboardingScreen />);
    const appSelect = await screen.findByLabelText("GitHub App");
    await user.selectOptions(appSelect, "gha-alpha");
    await user.selectOptions(appSelect, "gha-beta");
    await screen.findByRole("combobox", { name: "授权仓库" });
    resolveAlpha(envelope({ items: [apiRepo] }));
    await waitFor(() => expect(client.listGitHubAppRepositories).toHaveBeenCalledTimes(2));

    await user.click(screen.getByRole("combobox", { name: "授权仓库" }));
    expect(screen.getByRole("option", { name: "acme/web" })).toBeVisible();
    expect(screen.queryByRole("option", { name: "acme/api" })).not.toBeInTheDocument();
  });

  it("excludes already-onboarded full names regardless of source", async () => {
    const user = userEvent.setup();
    vi.mocked(client.getRepositoryOnboarding).mockResolvedValue(
      envelope(
        summaryFixture({
          onboarded_repositories: {
            items: [{ id: "public-api", source_type: "public_github", ...apiRepo }],
          },
        }),
      ),
    );
    render(<RepositoryOnboardingScreen />);
    await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-alpha");
    await user.click(await screen.findByRole("combobox", { name: "授权仓库" }));

    expect(screen.queryByRole("option", { name: "acme/api" })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: "acme/web" })).toBeVisible();
  });

  it("distinguishes no authorized repositories from no search matches", async () => {
    const user = userEvent.setup();
    vi.mocked(client.listGitHubAppRepositories).mockResolvedValueOnce(envelope({ items: [] }));
    render(<RepositoryOnboardingScreen />);
    const appSelect = await screen.findByLabelText("GitHub App");
    await user.selectOptions(appSelect, "gha-alpha");
    expect(await screen.findByText("此 GitHub App 没有授权仓库")).toBeVisible();

    await user.selectOptions(appSelect, "gha-beta");
    const input = await screen.findByRole("combobox", { name: "授权仓库" });
    await user.type(input, "missing");
    expect(screen.getByText("没有匹配的仓库")).toBeVisible();
  });

  it("adds the selected repository with its App id and removes it from candidates", async () => {
    const user = userEvent.setup();
    render(<RepositoryOnboardingScreen />);
    await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-beta");
    const input = await screen.findByRole("combobox", { name: "授权仓库" });
    await user.click(input);
    await user.click(screen.getByRole("option", { name: "acme/web" }));
    expect(screen.getByText("默认分支：develop · 可见性：private")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "添加仓库" }));

    expect(client.addGitHubAppRepository).toHaveBeenCalledWith("gha-beta", "acme/web");
    expect(input).toHaveValue("");
    await user.click(input);
    expect(screen.queryByRole("option", { name: "acme/web" })).not.toBeInTheDocument();
    expect(within(screen.getByRole("table", { name: "已接入仓库列表" })).getByText("acme/web")).toBeVisible();
  });

  it("keeps the public URL flow and clears App selection when switching sources", async () => {
    const user = userEvent.setup();
    const createSyncRuleSpy = vi.spyOn(client, "createSyncRule");
    render(<RepositoryOnboardingScreen />);
    await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-beta");
    await user.click(screen.getByRole("radio", { name: "公开仓库" }));

    await user.type(screen.getByLabelText("公共仓库 URL 或 owner/repo"), "https://github.com/rust-lang/rust");
    await user.click(screen.getByRole("button", { name: "添加公开仓库" }));

    expect(client.addPublicRepository).toHaveBeenCalledWith("https://github.com/rust-lang/rust");
    expect(screen.getByLabelText("公共仓库 URL 或 owner/repo")).toHaveValue("");
    expect(within(screen.getByRole("table", { name: "已接入仓库列表" })).getByText("rust-lang/rust")).toBeVisible();
    expect(createSyncRuleSpy).not.toHaveBeenCalled();
    await user.click(screen.getByRole("radio", { name: "GitHub App 授权仓库" }));
    expect(screen.getByLabelText("GitHub App")).toHaveValue("");
  });

  it("directs users to Git integration when no installed Apps exist", async () => {
    vi.mocked(client.listGitHubApps).mockResolvedValue(envelope({ items: [pendingApp] }));
    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByText("暂无已安装的 GitHub App")).toBeVisible();
    expect(screen.getByRole("button", { name: "打开 Git 接入" })).toBeVisible();
  });

  it("keeps the loaded inventory when Apps fail and retries only Apps", async () => {
    const user = userEvent.setup();
    vi.mocked(client.listGitHubApps)
      .mockRejectedValueOnce(new Error("apps unavailable"))
      .mockResolvedValueOnce(envelope({ items: [alphaApp] }));
    vi.mocked(client.getRepositoryOnboarding).mockResolvedValue(
      envelope(summaryFixture({ onboarded_repositories: { items: [{ id: "repo-api", source_type: "github_app", ...apiRepo }] } })),
    );
    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByRole("alert")).toHaveTextContent("apps unavailable");
    expect(screen.queryByText("暂无已安装的 GitHub App")).not.toBeInTheDocument();
    expect(within(screen.getByRole("table", { name: "已接入仓库列表" })).getByText("acme/api")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "重试加载 GitHub Apps" }));

    expect(await screen.findByLabelText("GitHub App")).toBeVisible();
    expect(client.listGitHubApps).toHaveBeenCalledTimes(2);
    expect(client.getRepositoryOnboarding).toHaveBeenCalledOnce();
  });

  it("does not present a confirmed zero inventory when summary fails and retries only inventory", async () => {
    const user = userEvent.setup();
    vi.mocked(client.getRepositoryOnboarding)
      .mockRejectedValueOnce(new Error("inventory unavailable"))
      .mockResolvedValueOnce(envelope(summaryFixture()));
    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByRole("alert")).toHaveTextContent("inventory unavailable");
    expect(screen.queryByText("0 个仓库")).not.toBeInTheDocument();
    expect(screen.getByText("仓库清单未确认")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "重试加载仓库清单" }));

    expect(await screen.findByText("0 个仓库")).toBeVisible();
    expect(client.getRepositoryOnboarding).toHaveBeenCalledTimes(2);
    expect(client.listGitHubApps).toHaveBeenCalledOnce();
  });

  it("locks all repository mutations and selectors until the active add finishes", async () => {
    const user = userEvent.setup();
    let resolveAdd!: (value: ReturnType<typeof envelope<RepositoryInventoryItem>>) => void;
    vi.mocked(client.getRepositoryOnboarding).mockResolvedValue(
      envelope(summaryFixture({ onboarded_repositories: { items: [{ id: "repo-existing", source_type: "github_app", ...apiRepo }] } })),
    );
    vi.mocked(client.addGitHubAppRepository).mockImplementation(
      (_appID, fullName) => new Promise((resolve) => {
        resolveAdd = resolve;
      }),
    );
    render(<RepositoryOnboardingScreen />);
    const appSelect = await screen.findByLabelText("GitHub App");
    await user.selectOptions(appSelect, "gha-beta");
    const combobox = await screen.findByRole("combobox", { name: "授权仓库" });
    await user.click(combobox);
    await user.click(screen.getByRole("option", { name: "acme/web" }));
    const addButton = screen.getByRole("button", { name: "添加仓库" });
    await user.click(addButton);

    expect(appSelect).toBeDisabled();
    expect(combobox).toBeDisabled();
    expect(screen.getByRole("radio", { name: "公开仓库" })).toBeDisabled();
    expect(addButton).toBeDisabled();
    expect(screen.getByRole("button", { name: "移除" })).toBeDisabled();
    await user.selectOptions(appSelect, "gha-alpha");
    await user.click(screen.getByRole("radio", { name: "公开仓库" }));
    await user.click(addButton);
    await user.click(screen.getByRole("button", { name: "移除" }));
    expect(client.addGitHubAppRepository).toHaveBeenCalledOnce();
    expect(client.removeRepository).not.toHaveBeenCalled();
    expect(appSelect).toHaveValue("gha-beta");
    expect(combobox).toHaveValue("acme/web");

    resolveAdd(envelope({ id: "repo-web", source_type: "github_app", github_app_id: "gha-beta", ...webRepo }));
    await waitFor(() => expect(appSelect).toBeEnabled());
    expect(appSelect).toHaveValue("gha-beta");
    expect(combobox).toHaveValue("");
  });

  it("shows textual loading and resource errors", async () => {
    let rejectApps!: (reason: Error) => void;
    vi.mocked(client.listGitHubApps).mockImplementation(() => new Promise((_, reject) => (rejectApps = reject)));
    render(<RepositoryOnboardingScreen />);
    expect(screen.getByRole("status")).toHaveTextContent("正在加载仓库接入信息");
    rejectApps(new Error("apps unavailable"));

    expect(await screen.findByRole("alert")).toHaveTextContent("apps unavailable");
  });
});

function envelope<T>(data: T) {
  return { data, meta: { server_time: "", resource_version: 0 } };
}

function summaryFixture(overrides: Partial<RepositoryOnboardingSummary> = {}): RepositoryOnboardingSummary {
  return {
    github_app: alphaApp,
    app_repositories: { items: [] },
    onboarded_repositories: { items: [] },
    ...overrides,
  };
}
