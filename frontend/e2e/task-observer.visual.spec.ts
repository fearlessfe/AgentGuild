import { expect, test } from "@playwright/test";

test("desktop observer keeps the dense table and detail pane", async ({ page }) => {
  await page.setViewportSize({ width: 1512, height: 1064 });
  await page.goto("/tasks");
  await expect(page.getByLabel("任务列表")).toBeVisible();
  await expect(page.getByLabel("任务详情")).toBeVisible();
  await expect(page.getByRole("button", { name: /claim|accept|set status/i })).toHaveCount(0);
  await page.screenshot({ path: "/private/tmp/agentguild-task8/desktop-1512x1064.png", fullPage: true });
});

test("mobile observer uses horizontal list and detail drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/tasks/AG-192");
  await expect(page.getByLabel("任务详情")).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭详情" })).toBeVisible();
  const overflow = await page.getByLabel("任务列表").evaluate((node) => node.scrollWidth > node.clientWidth);
  expect(overflow).toBeTruthy();
  await page.screenshot({ path: "/private/tmp/agentguild-task8/mobile-390x844.png", fullPage: true });
});
