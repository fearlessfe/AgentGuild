import { expect, test, type Page } from "@playwright/test";

async function expectNoPageOverflow(page: Page) {
  const metrics = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
  }));
  expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
  expect(metrics.bodyScrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
}

async function expectReadableDiff(page: Page) {
  const measurements = await page.locator(".diff-table .line-content pre").evaluateAll((nodes) =>
    nodes
      .map((node) => {
        const rect = node.getBoundingClientRect();
        return {
          text: node.textContent ?? "",
          width: rect.width,
          height: rect.height,
        };
      })
      .filter((item) => item.text.trim().length > 0),
  );
  expect(measurements.length).toBeGreaterThan(0);
  for (const item of measurements.slice(0, 6)) {
    expect(item.width).toBeGreaterThan(80);
    expect(item.height).toBeLessThan(80);
  }
}

test.describe("console UI/UX baseline", () => {
  test("review diff remains readable on desktop", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/reviews/review-1");
    await expect(page.getByRole("heading", { name: "审核工作台" })).toBeVisible();
    await expectReadableDiff(page);
    await expectNoPageOverflow(page);
  });

  test("review diff remains readable on mobile", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/reviews/review-1");
    await expect(page.getByRole("heading", { name: "审核工作台" })).toBeVisible();
    await expectReadableDiff(page);
    await expectNoPageOverflow(page);
  });

  test("mobile task list exposes core information", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/tasks");
    await expect(page.getByText("修复批量退款时的余额竞争条件")).toBeVisible();
    await expect(page.getByText("billing-service")).toBeVisible();
    await expect(page.getByText("待领取").first()).toBeVisible();
    await expectNoPageOverflow(page);
  });

  test("mobile agent list exposes core information", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/agents");
    const atlasRow = page.locator(".agent-row").filter({ hasText: "Atlas v12" });
    await expect(page.getByText("Atlas v12")).toBeVisible();
    await expect(page.getByText("atlas@example.com")).toBeVisible();
    await expect(atlasRow.getByText("active", { exact: true })).toBeVisible();
    await expectNoPageOverflow(page);
  });
});
