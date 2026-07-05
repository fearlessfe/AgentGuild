import { test, expect } from "@playwright/test";

test.describe("Agent Version and Experience", () => {
  test("Agent detail page shows version tree placeholder", async ({ page }) => {
    await page.goto("/agents/agent-active");
    await expect(page.getByText("版本谱系")).toBeVisible();
    await expect(page.getByText("经验候选")).toBeVisible();
    await expect(page.getByText("评测运行")).toBeVisible();
  });
});
