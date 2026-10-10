import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, openStandaloneApp, test } from "../fixtures/base";

test("completed isolated history video uses a readable candidate link in the browser", async ({
  page,
}) => {
  test.skip(
    process.env.E2E_HISTORY_MEDIA !== "true",
    "explicit completed history sample only; no writes",
  );
  const l = JSON.parse(
    readFileSync(
      resolve("../media_worker/.fixture-ui-resume-ledger.json"),
      "utf8",
    ),
  );
  expect(l.completed).toBe(true);
  const response = await page.request.get(`/api/v1/jobs/${l.job}`);
  expect(response.ok()).toBeTruthy();
  const detail = await response.json();
  expect(detail.job.status).toBe(4);
  const video = detail.step_runs.find(
    (s: { step_key: string }) => s.step_key === "video",
  );
  expect(video.output_asset_id).toBe(l.output);
  const probe = await page.request.get(video.candidates[0].url, {
    headers: { Range: "bytes=0-0" },
  });
  expect(probe.status()).toBe(206);
  expect(probe.headers()["content-type"]).toContain("video/");
  await openStandaloneApp(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page.getByRole("button", { name: /^已完成$|^Completed$/ }).click();
  const row = page.getByTestId(`execution-row-${l.job}`);
  for (let i = 0; i < 20; i++) {
    await expect(
      page.getByRole("status", { name: /^正在加载$|^Loading$/ }),
    ).toHaveCount(0, { timeout: 60000 });
    if (await row.count()) break;
    const next = page.getByRole("button", { name: /^下一页$|^Next$/ });
    await expect(next).toBeEnabled();
    const loaded = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/api/v1/jobs" &&
        new URL(r.url()).searchParams.get("status") === "succeeded",
      { timeout: 120000 },
    );
    await next.click();
    expect((await loaded).ok()).toBeTruthy();
  }
  await expect(row).toBeVisible();
  await row
    .getByRole("button")
    .filter({ has: page.getByRole("heading", { level: 2 }) })
    .click();
  await page
    .getByRole("button", { name: "选择节点 video", exact: true })
    .click();
  const media = page
    .getByRole("article", { name: /^节点结果$|^Step results$/ })
    .locator("video");
  await expect(media).toBeVisible();
  await expect
    .poll(() => media.evaluate((v: HTMLVideoElement) => v.videoWidth), {
      timeout: 60000,
    })
    .toBeGreaterThan(0);
  await expect(
    page.getByRole("button", { name: /^确认使用$|^Accept result$/ }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", {
      name: /^修改输入重新运行$|^Rerun with changed inputs$/,
    }),
  ).toHaveCount(0);
});
