import { expect, test } from "../fixtures/base";

test("renders the workflow as a FlowGram canvas instead of JSON", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /^(Workflow|工作流)$/ }).click();
  await page.getByRole("tab", { name: /^(Workflows|工作流)$/ }).click();

  const workflowCard = page.getByRole("article").first();
  await expect(workflowCard).toBeVisible();
  await workflowCard
    .getByRole("button")
    .filter({ has: page.locator("h2") })
    .click();

  await expect(page.getByText("FlowGram", { exact: true })).toBeVisible();
  await expect(page.locator(".workflow-graph")).toBeVisible();
  await expect(page.locator(".workflow-node-card")).toHaveCount(9);
  await expect(page.locator("pre")).toHaveCount(0);

  await page.getByRole("button", { name: /^(Edit|编辑)$/ }).click();
  await expect(page.getByText(/FlowGram 画布/)).toBeVisible();
  await expect(
    page.getByText(/Steps \(JSON|DAG visual editor TODO/),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: /^(节点配置|Node configuration)$/ }),
  ).toBeVisible();
});
