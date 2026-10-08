import { expect, test } from "../fixtures/base";

test("workflow editor keeps draft dependencies and confirms deletion without saving borrowed data", async ({
  page,
}) => {
  await page.goto("/");
  await page.setViewportSize({ width: 1440, height: 900 });
  const response = await page.request.get("/api/v1/workflows?page_size=100");
  expect(response.ok()).toBeTruthy();
  const { items } = await response.json();
  let workflow;
  for (const item of items) {
    const detail = await page.request.get(
      `/api/v1/workflows/${encodeURIComponent(item.id)}`,
    );
    if (!detail.ok()) continue;
    const candidate = await detail.json();
    if ((candidate.steps?.length ?? 0) >= 2) {
      workflow = candidate;
      break;
    }
  }
  expect(workflow, "requires an existing multi-node workflow").toBeTruthy();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^工作流$|^Workflows$/ }).click();
  await page
    .getByRole("textbox", { name: /^搜索名称$|^Search by name$/ })
    .fill(workflow.name);
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: workflow.name, exact: true }),
    })
    .click();
  await expect(
    page.getByRole("button", { name: /^适应画布$|^Fit to canvas$/ }),
  ).toBeVisible();
  await expect
    .poll(
      async () =>
        (await page.getByTestId("workflow-canvas").boundingBox())?.height ?? 0,
    )
    .toBeGreaterThan(500);
  await page.screenshot({ path: "/tmp/media-agent-v12-definition-live.png" });
  await page.getByRole("button", { name: /^编辑$|^Edit$/ }).click();
  await expect(page.getByLabel("节点标识")).toHaveCount(0);
  await expect(
    page.getByRole("complementary", { name: /^节点工具箱$|^Node toolbox$/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: /^添加节点$|^Add node$/ }).click();
  await expect(
    page.getByRole("dialog", {
      name: /^选择生成模板$|^Choose a generation template$/,
    }),
  ).toBeVisible();
  await page.screenshot({
    path: "/tmp/media-agent-v12-template-picker-live.png",
  });
  await page.keyboard.press("Escape");
  const key = workflow.steps[0].key;
  await page
    .getByRole("button", { name: `选择节点 ${key}`, exact: true })
    .click();
  await page.getByLabel("节点标识").fill(`${key}_edited`);
  await expect
    .poll(
      async () =>
        (await page.getByTestId("workflow-canvas").boundingBox())?.height ?? 0,
    )
    .toBeGreaterThan(500);
  await page.screenshot({ path: "/tmp/media-agent-v12-editor-live.png" });
  await page.setViewportSize({ width: 480, height: 900 });
  await expect(
    page.getByRole("complementary", { name: /^节点工具箱$|^Node toolbox$/ }),
  ).toBeHidden();
  await expect(page.getByLabel("节点标识")).toBeVisible();
  await page.screenshot({
    path: "/tmp/media-agent-v12-editor-narrow-live.png",
  });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: "删除当前节点" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText(/保留失效绑定|Broken bindings/);
  await dialog.getByRole("button", { name: /^取消$|^Cancel$/ }).click();
  await expect(
    page.getByRole("button", { name: `选择节点 ${key}_edited`, exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await expect(
    page.getByRole("button", { name: `选择节点 ${key}_edited`, exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("节点标识")).toHaveCount(0);
  await page.getByRole("button", { name: "退出", exact: true }).click();
});
