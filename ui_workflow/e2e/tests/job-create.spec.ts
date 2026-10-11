import { expect, openStandaloneApp, test } from "../fixtures/base";

test("generic job creation enters the same canvas wizard after selecting a workflow", async ({
  page,
}) => {
  test.setTimeout(120_000);
  await openStandaloneApp(page);
  const response = await page.request.get("/api/v1/workflows?page_size=200");
  expect(response.ok()).toBeTruthy();
  const { items } = await response.json();
  let workflow;
  for (const item of items) {
    const detail = await page.request.get(`/api/v1/workflows/${item.id}`);
    if (!detail.ok()) continue;
    const candidate = await detail.json();
    if (candidate.allowed_actions?.includes("run")) {
      workflow = candidate;
      break;
    }
  }
  if (!workflow) throw new Error("Requires an existing runnable workflow");
  let submissions = 0;
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      /\/api\/v1\/jobs(?:\?|$)/.test(request.url())
    )
      submissions++;
  });
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page
    .getByRole("button", { name: /^运行工作流$|^Run workflow$/ })
    .click();
  await page
    .getByRole("combobox", { name: /^工作流$|^Workflows$/ })
    .selectOption(workflow.id);
  await expect(
    page.getByRole("heading", { name: workflow.name, exact: true }),
  ).toBeVisible();
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
  await expect(page.locator("pre")).toHaveCount(0);
  await page.getByRole("button", { name: /^取消$|^Cancel$/ }).click();
  await expect(
    page.getByRole("button", { name: /^运行工作流$|^Run workflow$/ }),
  ).toBeVisible();
  expect(submissions).toBe(0);
});
