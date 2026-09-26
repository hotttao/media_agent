import { expect, test } from "../fixtures/base";

test("creates a typed job form from the published workflow contract", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Workflow|工作流/ }).click();
  await page.getByRole("tab", { name: /Jobs|任务/ }).click();
  await page.getByRole("button", { name: /New|新建/ }).click();

  await expect(page.getByRole("heading", { name: "创建生成任务" })).toBeVisible();
  await expect(page.getByText("Input (JSON)")).toHaveCount(0);

  await page.getByText("选择已发布的工作流").click();
  await page
    .getByRole("option", { name: /media_ai · 完整带货视频生成/ })
    .click();

  await expect(page.getByText("movement_a · movement").first()).toBeVisible();
  await expect(page.getByText("movement_b · movement").first()).toBeVisible();
  await expect(page.getByText("pose · pose").first()).toBeVisible();
  await expect(page.getByText("scene · scene").first()).toBeVisible();
  await expect(page.getByText("product · 商品").first()).toBeVisible();
  await expect(page.getByText("virtual_ip · 虚拟 IP").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "提交任务" })).toBeDisabled();
});
