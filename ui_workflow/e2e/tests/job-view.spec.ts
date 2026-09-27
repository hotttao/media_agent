import { expect, test } from "../fixtures/base";

test("opens the migrated sample execution without creating another job", async ({ page }) => {
  const sampleJobId = process.env.E2E_SAMPLE_JOB_ID;
  test.skip(!sampleJobId, "requires an existing sample job ID");

  await page.goto("/");
  await page.getByRole("button", { name: /Workflow|工作流/ }).click();
  await page.getByRole("tab", { name: /Jobs|任务/ }).click();
  await page.locator("tbody tr").first().getByRole("button").first().click();

  await expect(page.getByText("Job ID", { exact: true })).toBeVisible();
  await expect(page.getByText(sampleJobId!, { exact: true })).toBeVisible();
  await expect(page.getByText("Step Runs (4)")).toBeVisible();
});
