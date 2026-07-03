import { expect, test } from "@playwright/test";

test("observer follows server polling metadata", async ({ page }) => {
  await page.goto("/tasks/AG-188");
  await expect(page.getByText("Running", { exact: true })).toBeVisible();
  await expect(page.getByText("10m lease")).toBeVisible();
  await expect(page.getByRole("button", { name: "Set status" })).toHaveCount(0);
});
