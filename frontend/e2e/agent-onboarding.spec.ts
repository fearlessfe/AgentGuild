import { expect, test } from "@playwright/test";

test("agents list supports status filtering in demo mode", async ({ page }) => {
  await page.goto("/agents");

  const desktopAgentList = page.getByLabel("Agent 列表", { exact: true });
  await expect(desktopAgentList).toBeVisible();
  await expect(desktopAgentList.getByText("Atlas v12")).toBeVisible();
  await expect(desktopAgentList.getByText("Suspended Worker")).toBeVisible();

  await page.getByRole("combobox", { name: "状态" }).selectOption("suspended");

  await expect(desktopAgentList.getByText("Suspended Worker")).toBeVisible();
  await expect(desktopAgentList.getByText("Atlas v12")).toHaveCount(0);
});

test("registration reveals one-time token and agent detail hides unsupported controls", async ({ page }) => {
  await page.goto("/agents/new");

  await page.getByLabel("名称").fill("Code Review Bot");
  await page.getByRole("button", { name: "注册 Agent" }).click();

  await expect(page.getByLabel("Activation Token")).toBeVisible();
  await expect(page.getByText("agtok_agent-5_once")).toBeVisible();

  await page.getByRole("button", { name: "我已保存" }).click();
  await expect(page.getByLabel("Activation Token")).toHaveCount(0);

  await page.goto("/agents/agent-revoked");
  await expect(page.locator(".detail-status")).toHaveText("revoked");
  await expect(page.getByRole("button", { name: "暂停 Agent" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "恢复 Agent" })).toHaveCount(0);

  await page.goto("/agents/agent-pending");
  await expect(page.locator(".detail-status")).toHaveText("pending activation");
  await expect(page.getByRole("button", { name: "暂停 Agent" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "恢复 Agent" })).toHaveCount(0);
});

test("selects an installed app, searches locally, and adds app and public repositories", async ({ page }) => {
  await page.goto("/repositories");

  await page.getByRole("combobox", { name: "GitHub App", exact: true }).selectOption("gha-beta");
  const repositorySelector = page.getByRole("combobox", { name: "授权仓库" });
  await repositorySelector.fill("data");
  await expect(page.getByRole("option", { name: "acme/data-api", exact: true })).toBeVisible();
  await expect(page.getByRole("option", { name: "acme/frontend", exact: true })).toHaveCount(0);
  await repositorySelector.press("ArrowDown");
  await repositorySelector.press("Enter");
  await page.getByRole("button", { name: "添加仓库", exact: true }).click();

  const onboardedTable = page.getByRole("table", { name: "已接入仓库列表" });
  await expect(onboardedTable).toContainText("acme/data-api");

  await page.getByRole("radio", { name: "公开仓库" }).check();
  await page.getByLabel("公共仓库 URL 或 owner/repo").fill("https://github.com/facebook/react");
  await page.getByRole("button", { name: "添加公开仓库" }).click();
  await expect(onboardedTable).toContainText("facebook/react");
});

test("git integration shows both automatic GitHub App labels", async ({ page }) => {
  await page.goto("/git-integration");

  const githubAppList = page.getByRole("list", { name: "GitHub App 列表", exact: true });
  await expect(githubAppList.getByRole("heading", { name: "alpha · acme-corp", exact: true })).toBeVisible();
  await expect(githubAppList.getByRole("heading", { name: "beta · acme-labs", exact: true })).toBeVisible();
});
