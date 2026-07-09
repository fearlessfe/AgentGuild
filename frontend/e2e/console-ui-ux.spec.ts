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

async function expectMinHitTargetHeight(page: Page, selector: string, minimumHeight = 44) {
  const measurements = await page.locator(selector).evaluateAll((nodes) =>
    nodes.map((node) => {
      const rect = node.getBoundingClientRect();
      return {
        height: rect.height,
        width: rect.width,
        text: node.textContent?.trim() ?? "",
        ariaLabel: node.getAttribute("aria-label") ?? "",
      };
    }),
  );
  expect(measurements.length).toBeGreaterThan(0);
  for (const item of measurements) {
    expect(
      item.height,
      `Expected "${item.ariaLabel || item.text || selector}" to be at least ${minimumHeight}px high, got ${item.width}x${item.height}`,
    ).toBeGreaterThanOrEqual(minimumHeight);
  }
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
    await expect(page.getByRole("button", { name: "Unified" })).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByRole("button", { name: "Split" })).toHaveAttribute("aria-pressed", "false");
    await expectMinHitTargetHeight(page, ".rail-toggle");
    await expectMinHitTargetHeight(page, ".diff-toolbar button");
    await expectReadableDiff(page);
    await expectNoPageOverflow(page);
  });

  test("mobile task list exposes core information", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/tasks");
    await expect(page.locator(".dense-table")).toBeHidden();
    const mobileList = page.locator(".task-mobile-list");
    await expect(mobileList).toBeVisible();

    const taskCard = mobileList.locator(".task-mobile-card").filter({ hasText: "修复批量退款时的余额竞争条件" });
    await expect(taskCard).toHaveCount(1);
    await expect(taskCard.getByText("billing-service")).toBeVisible();
    await expect(taskCard.getByText("待领取")).toBeVisible();
    await expectNoPageOverflow(page);
  });

  test("mobile agent list exposes core information", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/agents");
    await expect(page.locator(".agent-table")).toBeHidden();
    const mobileList = page.locator(".agent-mobile-list");
    await expect(mobileList).toBeVisible();

    const atlasCard = mobileList.locator(".agent-mobile-card").filter({ hasText: "Atlas v12" });
    await expect(atlasCard).toHaveCount(1);
    await expect(atlasCard.getByText("atlas@example.com")).toBeVisible();
    await expect(atlasCard.getByText("active", { exact: true })).toBeVisible();
    await expectNoPageOverflow(page);
  });
});
