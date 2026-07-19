import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "fs";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";

async function expectNoPageOverflow(page: Page) {
  const metrics = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
  }));
  expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
  expect(metrics.bodyScrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
}

function comparePng(
  baselinePath: string,
  screenshot: Buffer,
  threshold = 0.2,
  maxDiffRatio = 0.35,
) {
  const baseline = PNG.sync.read(readFileSync(baselinePath));
  const captured = PNG.sync.read(screenshot);
  const width = baseline.width;
  const height = baseline.height;
  if (captured.width !== width || captured.height !== height) {
    throw new Error(
      `Screenshot size ${captured.width}x${captured.height} does not match baseline ${width}x${height}`,
    );
  }
  const diff = pixelmatch(baseline.data, captured.data, null, width, height, {
    threshold,
    includeAA: true,
  });
  const ratio = diff / (width * height);
  expect(
    ratio,
    `Visual diff ${(ratio * 100).toFixed(1)}% exceeds allowed ${(maxDiffRatio * 100).toFixed(1)}%`,
  ).toBeLessThanOrEqual(maxDiffRatio);
}

test("desktop observer keeps the dense table and detail pane", async ({ page }) => {
  await page.setViewportSize({ width: 1487, height: 1058 });
  await page.goto("/tasks/AG-192");
  // The mobile card list stays in the DOM but is display:none on desktop, so
  // the visible desktop region is the only "任务列表" in the a11y tree.
  await expect(page.getByRole("region", { name: "任务列表", exact: true })).toBeVisible();
  await expect(page.getByRole("dialog", { name: "任务详情" })).toHaveCount(0);
  await expect(page.getByLabel("任务详情")).toBeVisible();
  await expect(page.getByRole("button", { name: /claim|accept|set status/i })).toHaveCount(0);

  const screenshot = await page.screenshot({ fullPage: true });
  comparePng("../docs/assets/agentguild-tasks.png", screenshot);
});

test("mobile observer uses card list and modal detail drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/tasks/AG-192");

  const drawer = page.locator('dialog[role="dialog"][aria-modal="true"]');
  await expect(drawer).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭详情" })).toBeVisible();

  // Focus is trapped inside the modal drawer.
  const focusedInDrawer = await drawer.evaluate((node) => node.contains(document.activeElement));
  expect(focusedInDrawer).toBeTruthy();

  // Close via the close button.
  await page.getByRole("button", { name: "关闭详情" }).click();
  await expect(drawer).toHaveCount(0);
  await expect(page).toHaveURL("/tasks");

  // The mobile task list is a vertical list of cards; the dense table stays
  // hidden and the page never overflows horizontally.
  await expect(page.locator(".dense-table")).toBeHidden();
  const mobileList = page.locator(".task-mobile-list");
  await expect(mobileList).toBeVisible();
  const layout = await page.evaluate(() => {
    const cards = Array.from(document.querySelectorAll(".task-mobile-card")).map((node) => {
      const rect = node.getBoundingClientRect();
      return { x: rect.x, y: rect.y, right: rect.right };
    });
    return { cards, clientWidth: document.documentElement.clientWidth };
  });
  expect(layout.cards.length).toBeGreaterThan(1);
  for (let i = 0; i < layout.cards.length; i += 1) {
    if (i > 0) expect(layout.cards[i].y).toBeGreaterThan(layout.cards[i - 1].y);
    expect(layout.cards[i].x).toBeCloseTo(layout.cards[0].x, 0);
    expect(layout.cards[i].right).toBeLessThanOrEqual(layout.clientWidth);
  }
  await expectNoPageOverflow(page);

  // Reopen by selecting a task from the list.
  await page.getByRole("link", { name: "查看 AG-192" }).click();
  await expect(drawer).toBeVisible();

  // Escape closes the modal and restores the list route.
  await page.keyboard.press("Escape");
  await expect(drawer).toHaveCount(0);
  await expect(page).toHaveURL("/tasks");

  // Backdrop click closes too.
  await page.getByRole("link", { name: "查看 AG-192" }).click();
  await expect(drawer).toBeVisible();
  // Backdrop click closes too: click the dimmed area outside the centered
  // drawer panel (the panel starts 16px from the left edge at this viewport).
  await page.mouse.click(8, 100);
  await expect(drawer).toHaveCount(0);
});
