import { expect, test } from "@playwright/test";

test("agents list supports status filtering in demo mode", async ({ page }) => {
  await page.goto("/agents");

  await expect(page.getByLabel("Agent 列表")).toBeVisible();
  await expect(page.getByText("Atlas v12")).toBeVisible();
  await expect(page.getByText("Suspended Worker")).toBeVisible();

  await page.getByRole("combobox", { name: "状态" }).selectOption("suspended");

  await expect(page.getByText("Suspended Worker")).toBeVisible();
  await expect(page.getByText("Atlas v12")).toHaveCount(0);
});

test("registration reveals one-time token and revoked detail hides controls", async ({ page }) => {
  await page.goto("/agents/new");

  await page.getByLabel("名称").fill("Code Review Bot");
  await page.getByLabel("Owner Email").fill("review@example.com");
  await page.getByRole("button", { name: "注册 Agent" }).click();

  await expect(page.getByLabel("Activation Token")).toBeVisible();
  await expect(page.getByText("agtok_agent-5_once")).toBeVisible();

  await page.getByRole("button", { name: "我已保存" }).click();
  await expect(page.getByLabel("Activation Token")).toHaveCount(0);

  await page.goto("/agents/agent-revoked");
  await expect(page.getByText("revoked")).toBeVisible();
  await expect(page.getByRole("button", { name: "暂停 Agent" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "恢复 Agent" })).toHaveCount(0);
});
