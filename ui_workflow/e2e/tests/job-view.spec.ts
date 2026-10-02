import { expect, test } from "../fixtures/base";

test("shows workflow progress and lets the user inspect a single execution node", async ({
  page,
}) => {
  const identity = (await (await page.request.get("/api/auth/me")).json()) as {
    id: string;
  };
  const jobs = (await (
    await page.request.get("/api/v1/jobs?page_size=100")
  ).json()) as { items: { submitted_by: string }[] };
  expect(jobs.items.length).toBeGreaterThan(0);
  expect(jobs.items.every((job) => job.submitted_by === identity.id)).toBe(
    true,
  );
  await page.goto("/");
  await page.getByRole("button", { name: /Workflow|工作流/ }).click();
  await page.getByRole("tab", { name: /Jobs|任务/ }).click();

  const firstJob = page.getByRole("article").first();
  await expect(firstJob).toBeVisible();
  await expect(firstJob).toContainText("media_ai · 图片与动作视频生成");
  await expect(firstJob.getByRole("button").first()).toBeVisible();
  await firstJob.getByRole("button").first().click();

  await expect(page.getByRole("heading", { name: "执行位置" })).toBeVisible();
  await expect(page.locator(".workflow-node-card")).toHaveCount(4);
  const nodes = page
    .getByRole("navigation", { name: "选择执行节点" })
    .getByRole("button");
  await expect(nodes.first()).toBeVisible();
  await nodes.first().click();
  await expect(nodes.first()).toHaveAttribute("aria-current", "step");
  await expect(page.getByRole("article")).toHaveCount(1);
  await expect(page.getByText("Step Runs (4)")).toHaveCount(0);
  if (process.env.E2E_VERIFY_MEDIA === "true") {
    const image = page.getByRole("img", { name: "候选图片 1" });
    await expect(image).toBeVisible();
    await expect
      .poll(() =>
        image.evaluate((element: HTMLImageElement) => element.naturalWidth),
      )
      .toBeGreaterThan(0);
    await nodes.last().click();
    const video = page.getByLabel("候选视频 1");
    await expect(video).toBeVisible();
    await expect
      .poll(() =>
        video.evaluate((element: HTMLVideoElement) => element.readyState),
      )
      .toBeGreaterThan(0);
  }
});
