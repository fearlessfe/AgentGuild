import { expect, test } from "@playwright/test";
import { readFileSync } from "fs";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";

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
  await expect(page.getByLabel("任务列表")).toBeVisible();
  await expect(page.getByRole("dialog", { name: "任务详情" })).toHaveCount(0);
  await expect(page.getByLabel("任务详情")).toBeVisible();
  await expect(page.getByRole("button", { name: /claim|accept|set status/i })).toHaveCount(0);

  const screenshot = await page.screenshot({ fullPage: true });
  comparePng("../docs/assets/agentguild-tasks.png", screenshot);
});

test("mobile observer uses horizontal list and modal detail drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/tasks/AG-192");

  const drawer = page.locator('dialog[role="dialog"][aria-modal="true"]');
  await expect(drawer).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭详情" })).toBeVisible();

  // Focus is trapped inside the modal drawer.
  const focusedInDrawer = await drawer.evaluate((node) => node.contains(document.activeElement));
  expect(focusedInDrawer).toBeTruthy();

  const overflow = await page.locator(".list-pane").evaluate((node) => node.scrollWidth > node.clientWidth);
  expect(overflow).toBeTruthy();

  // Close via the close button.
  await page.getByRole("button", { name: "关闭详情" }).click();
  await expect(drawer).toHaveCount(0);
  await expect(page).toHaveURL("/tasks");

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
  // Click outside the drawer panel on the ::backdrop area. The dialog element itself covers the backdrop.
  await drawer.click({ position: { x: 10, y: 100 } });
  await expect(drawer).toHaveCount(0);
});
