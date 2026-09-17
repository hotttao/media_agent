import { expect, test } from "../fixtures/base";

test.describe("Materials and Workflow shell", () => {
  test("shows the authenticated company scope and both applications", {
    tag: "@C900001",
  }, async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Media Workflow" })).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Applications" })).toContainText("Materials");
    await expect(page.getByRole("navigation", { name: "Applications" })).toContainText("Workflow");
    await expect(page.getByText("Local Organization / Default Team / Standalone Workspace")).toBeVisible();
    await expect(page.getByText("No materials yet.")).toBeVisible();
  });

  test("switches from Materials to Workflow and loads the workflow resource tabs", {
    tag: "@C900002",
  }, async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Workflow", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Templates" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Workflows" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Jobs" })).toBeVisible();
    await page.getByRole("tab", { name: "Workflows" }).click();
    await expect(page.getByText("No workflows yet.")).toBeVisible();
  });
});
