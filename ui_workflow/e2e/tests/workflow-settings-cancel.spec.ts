import { expect, openStandaloneApp, test } from "../fixtures/base";

// Read-only: no saved definitions, executions or worker calls.
test("settings confirm returns to canvas and cancelling run returns to its workflow", async ({
  page,
}) => {
  test.setTimeout(120_000);
  const businessWrites: string[] = [];
  page.on("request", (request) => {
    if (
      request.method() !== "GET" &&
      /\/api\/v1\/(jobs|workflows|templates)(\/|\?|$)/.test(
        new URL(request.url()).pathname,
      )
    )
      businessWrites.push(request.method());
  });
  await openStandaloneApp(page);
  const response = await page.request.get("/api/v1/workflows?page_size=100");
  expect(response.ok()).toBeTruthy();
  const { items } = await response.json();
  let workflow;
  for (const item of items) {
    const detail = await page.request.get(`/api/v1/workflows/${item.id}`);
    if (!detail.ok()) continue;
    const candidate = await detail.json();
    if (candidate.status === 2 && candidate.publication_status === "registered" && candidate.allowed_actions?.includes("run")) {
      workflow = candidate;
      break;
    }
  }
  expect(workflow).toBeTruthy();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^工作流$|^Workflows$/ }).click();
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: workflow.name, exact: true }),
    })
    .click();
  await page
    .getByRole("button", { name: /^运行工作流$|^Run workflow$/ })
    .click();
  await page.getByRole("button", { name: /^取消$|^Cancel$/ }).click();
  await expect(
    page.getByRole("heading", { name: workflow.name, exact: true }),
  ).toBeVisible();
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
  await page.getByRole("button", { name: /^编辑$|^Edit$/ }).click();
  await page
    .getByRole("button", { name: /^工作流设置$|^Workflow settings$/ })
    .click();
  const settings = page.getByRole("dialog", {
    name: /^工作流设置$|^Workflow settings$/,
  });
  const output = settings.getByRole("combobox", {
    name: /^最终输出节点$|^Final output step$/,
  });
  await output.click();
  const key = workflow.steps[0].key;
  await page.getByRole("option", { name: key, exact: true }).click();
  await expect(output).toContainText(key);
  await settings
    .getByRole("button", {
      name: /^确定，返回画布$|^Confirm and return to canvas$/,
    })
    .click();
  await expect(settings).toBeHidden();
  await page.getByRole("button", { name: /^退出$|^Exit$/ }).click();
  expect(businessWrites).toEqual([]);
});
