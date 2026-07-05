import { test, expect } from "@playwright/test";

test.describe("Agent Version and Experience", () => {
  test("Agent detail page shows version and experience sections", async ({ page }) => {
    await page.goto("/agents/agent-active");
    await expect(page.getByText("版本谱系").first()).toBeVisible();
    await expect(page.getByText("经验候选").first()).toBeVisible();
  });
});
