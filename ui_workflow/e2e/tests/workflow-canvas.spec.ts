import { expect, test } from "../fixtures/base";

test("renders the workflow as a FlowGram canvas instead of JSON", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /^(Workflow|工作流)$/ }).click();
  await page.getByRole("tab", { name: /^(Workflows|工作流)$/ }).click();

  const workflowCard = page
    .getByRole("article")
    .filter({ hasText: "media_ai · 图片与动作视频生成（联调样本）" });
  await expect(workflowCard).toBeVisible();
  await workflowCard
    .getByRole("button")
    .filter({ has: page.locator("h2") })
    .click();

  await expect(page.getByText("FlowGram", { exact: true })).toBeVisible();
  await expect(page.locator(".workflow-graph")).toBeVisible();
  await expect(page.locator(".workflow-node-card")).toHaveCount(4);
  await expect(page.locator("pre")).toHaveCount(0);
  await expect(page.getByText("选择一个节点")).toBeVisible();

  const modelNode = page.getByRole("button", { name: "选择节点 model_image" });
  await modelNode.click();
  await expect(
    page.getByRole("heading", { name: "model_image" }),
  ).toBeVisible();

  const nodeBox = await modelNode.boundingBox();
  if (!nodeBox) throw new Error("workflow node is not visible");
  await page.mouse.move(
    nodeBox.x + nodeBox.width / 2,
    nodeBox.y + nodeBox.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    nodeBox.x + nodeBox.width / 2 + 100,
    nodeBox.y + nodeBox.height / 2 + 60,
    { steps: 10 },
  );
  await page.mouse.up();
  const movedBox = await modelNode.boundingBox();
  expect(movedBox?.x).toBeGreaterThan(nodeBox.x + 20);

  await page.getByRole("button", { name: /^(Edit|编辑)$/ }).click();
  await expect(page.getByText(/FlowGram 画布/)).toBeVisible();
  await expect(
    page.getByText(/Steps \(JSON|DAG visual editor TODO/),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: /^(节点配置|Node configuration)$/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: "选择节点 style_image" }).click();
  await expect(page.getByLabel("节点标识")).toHaveValue("style_image");
  await page.getByLabel("当前节点", { exact: true }).selectOption({ label: "model_image" });
  await expect(page.getByLabel("节点标识")).toHaveValue("model_image");
});
