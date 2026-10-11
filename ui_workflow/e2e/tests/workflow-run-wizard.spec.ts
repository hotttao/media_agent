import { expect, openStandaloneApp, test } from "../fixtures/base";
import { runInputGroups } from "../../src/pages/workflow/job/run-input-groups";
import { fromWire } from "../../src/domains/workflows/lib/api";

test("prepares typed inputs on real workflow nodes and cancels without submitting", async ({
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
    const candidate = fromWire(await detail.json());
    if (candidate.status === "published" && candidate.publication_status === "registered" && candidate.allowed_actions?.includes("run")) {
      workflow = candidate;
      break;
    }
  }
  const summary = workflow;
  expect(summary, "requires an authorized published workflow").toBeTruthy();
  if (!workflow) throw new Error("No runnable workflow fixture");
  const plan = runInputGroups(workflow.steps, workflow.input_schema.entries);
  let submits = 0;
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      /\/api\/v1\/jobs(?:\?|$)/.test(request.url())
    )
      submits++;
  });
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
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
  await expect(page.locator("pre")).toHaveCount(0);
  for (const group of plan.groups) {
    await expect(
      page.getByRole("heading", { name: group.stepKey, exact: true }),
    ).toBeVisible();
    for (const entry of group.entries) {
      const field = page.getByRole("region", { name: entry.name, exact: true });
      await expect(field.getByRole("textbox")).toBeVisible();
      if (["PRODUCT", "VIRTUAL_IP"].includes(entry.resource_type)) {
        const preview = field.getByRole("img").first();
        await expect(preview).toBeVisible();
        await expect
          .poll(() =>
            preview.evaluate(
              (element) =>
                (element as HTMLImageElement).complete &&
                (element as HTMLImageElement).naturalWidth > 0,
            ),
          )
          .toBe(true);
      }
      if (entry.required)
        await field.getByTestId(`run-choice-${entry.name}`).first().click();
    }
    await page
      .getByRole("button", { name: /^下一步$|^Next$/ })
      .last()
      .click();
  }
  await expect(
    page.getByRole("heading", { name: /^确认并运行$|^Review and run$/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^提交执行$|^Submit execution$/ }),
  ).toBeDisabled();
  const workspace = page.getByRole("combobox", {
    name: /^(归属工作空间|工作空间|Owning workspace|Workspace)$/,
  });
  const workspaces = await (
    await page.request.get("/api/v1/workspaces")
  ).json();
  await workspace.selectOption(workspaces.workspaces[0].id);
  await expect(
    page.getByRole("button", { name: /^提交执行$|^Submit execution$/ }),
  ).toBeEnabled();
  await page.setViewportSize({ width: 480, height: 900 });
  await page
    .getByRole("button", {
      name: /^收起输入，查看画布$|^Hide inputs to view canvas$/,
    })
    .click();
  await expect(
    page.getByRole("complementary", { name: /^准备运行$|^Prepare run$/ }),
  ).toBeHidden();
  await page.getByRole("button", { name: /^展开输入$|^Show inputs$/ }).click();
  await page.getByRole("button", { name: /^取消$|^Cancel$/ }).click();
  await expect(
    page.getByRole("heading", { name: workflow.name, exact: true }),
  ).toBeVisible();
  expect(submits).toBe(0);
});
