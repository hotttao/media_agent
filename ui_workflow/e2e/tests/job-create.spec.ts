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
    .getByRole("option", { name: /media_ai · 图片与动作视频生成/ })
    .click();

  await expect(page.getByText("movement · movement").first()).toBeVisible();
  await expect(page.getByText("pose · pose").first()).toBeVisible();
  await expect(page.getByText("scene · scene").first()).toBeVisible();
  await expect(page.getByText("product · 商品").first()).toBeVisible();
  await expect(page.getByText("virtual_ip · 虚拟 IP").first()).toBeVisible();
  const submit = page.getByRole("button", { name: "提交任务" });
  await expect(submit).toBeDisabled();

  for (const label of [
    "movement · movement",
    "pose · pose",
    "product · 商品",
    "scene · scene",
    "virtual_ip · 虚拟 IP",
  ]) {
    await page.getByText(`选择${label}`, { exact: true }).click();
    await expect(page.getByRole("option").first()).toBeVisible();
    await page.getByRole("option").filter({ hasNotText: "孤儿" }).first().click();
  }

  await expect(submit).toBeEnabled();
  // Explicit one-shot integration mode; ordinary E2E runs never create jobs.
  if (process.env.E2E_EXECUTE_SAMPLE === "true") {
    await submit.click();
    const jobId = page.locator("dd").first();
    await expect(page.getByText("Job ID", { exact: true })).toBeVisible();
    await expect(jobId).toHaveText(/^job_/);
    console.log(`Created sample job: ${await jobId.textContent()}`);
  }
});
