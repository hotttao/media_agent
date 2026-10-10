import { expect, openStandaloneApp, test } from "../fixtures/base";

test("workflow graph labels follow language changes without changing the definition", async ({
  page,
}) => {
  await openStandaloneApp(page);
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^工作流$|^Workflows$/ }).click();
  const response = await page.request.get(
    "/api/v1/workflows/wf_2c1231e5-7379-4207-a0de-a9bb5d93b278",
  );
  expect(response.ok()).toBe(true);
  const workflow = await response.json();
  await page
    .getByRole("textbox", { name: /^搜索名称$|^Search by name$/ })
    .fill(workflow.name);
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: workflow.name, exact: true }),
    })
    .click();
  await expect(page.locator(".workflow-node-card")).toHaveCount(
    workflow.steps.length,
  );
  await page
    .getByRole("combobox", { name: /^语言$|^Language$/ })
    .selectOption("en-US");
  await expect(
    page.getByRole("button", {
      name: `Select node ${workflow.steps[0].key}`,
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.locator(".workflow-node-card").filter({ hasText: "Output" }),
  ).toHaveCount(1);
  await expect(page.locator(".workflow-node-card").first()).toContainText(
    "bindings",
  );
  await page
    .getByRole("combobox", { name: "Language", exact: true })
    .selectOption("zh-CN");
  await expect(
    page.getByRole("button", {
      name: `选择节点 ${workflow.steps[0].key}`,
      exact: true,
    }),
  ).toBeVisible();
  // Borrowed definition is read-only. Language is presentation, not a DSL update.
});
