import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { createHash } from "node:crypto";
import { expect, openStandaloneApp, test } from "../fixtures/base";

const path = resolve("../media_worker/.fixture-ui-resume-ledger.json");
const prefix = (d: {
  step_runs: {
    origin: string;
    step_key: string;
    status: number;
    output_asset_id: string;
    source_record_id: string;
  }[];
}) =>
  d.step_runs
    .filter((s) => s.origin === "legacy_import")
    .map((s) => ({
      key: s.step_key,
      status: s.status,
      asset: s.output_asset_id,
      source: s.source_record_id,
    }));
const digest = (value: unknown) =>
  createHash("sha256").update(JSON.stringify(value)).digest("hex");

test("resume one isolated historical prefix through the UI and accept only the video suffix", async ({
  page,
}) => {
  test.skip(
    process.env.E2E_HISTORY_RESUME !== "true",
    "explicit approved isolated Resume fixture only",
  );
  const l = JSON.parse(readFileSync(path, "utf8"));
  const save = () => writeFileSync(path, JSON.stringify(l), { mode: 0o600 });
  const read = async (id = l.job) => {
    const r = await page.request.get(`/api/v1/jobs/${id}`, { timeout: 60000 });
    expect(r.ok()).toBeTruthy();
    return r.json();
  };
  const before = await read();
  expect(before.job.workspace_id).toBe(l.workspace);
  expect(before.job.workflow_id).toBe(l.workflow);
  expect(l.job).not.toBe(l.source);
  expect(digest(prefix(before))).toBe(l.prefixHash);
  await openStandaloneApp(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  const continuing = l.resumeAccepted === true;
  const statusFilter = continuing ? "need_confirm" : "awaiting_resume";
  await page
    .getByRole("button", {
      name: continuing
        ? /^待确认$|^Awaiting review$/
        : /^待继续$|^Awaiting continuation$/,
    })
    .click();
  const row = page.getByTestId(`execution-row-${l.job}`);
  for (let i = 0; i < 10; i++) {
    await expect(
      page.getByRole("status", { name: /^正在加载$|^Loading$/ }),
    ).toHaveCount(0, { timeout: 60000 });
    if (await row.count()) break;
    const next = page.getByRole("button", { name: /^下一页$|^Next$/ });
    await expect(next).toBeEnabled();
    const response = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/api/v1/jobs" &&
        new URL(r.url()).searchParams.get("status") === statusFilter,
      { timeout: 120000 },
    );
    await next.click();
    expect((await response).ok()).toBeTruthy();
  }
  await expect(row).toBeVisible();
  await row
    .getByRole("button")
    .filter({ has: page.getByRole("heading", { level: 2 }) })
    .click();
  if (!continuing) {
    expect(
      l.resumeStarted,
      "unknown outcome: inspect original command, do not submit another",
    ).not.toBe(true);
    await page
      .getByRole("button", {
        name: /^补缺输入继续$|^Resume with missing inputs$/,
      })
      .click();
    const movement = page.getByTestId("run-input-movement");
    await expect(movement).toBeVisible({ timeout: 60000 });
    await expect(page.getByTestId("run-input-product")).toHaveCount(0);
    await expect(page.getByTestId("run-input-virtual_ip")).toHaveCount(0);
    await expect
      .poll(() => movement.locator("option").count(), { timeout: 60000 })
      .toBeGreaterThan(1);
    const chosen = await movement
      .locator("option")
      .evaluateAll((options) =>
        options.map((o) => (o as HTMLOptionElement).value).find(Boolean),
      );
    if (!chosen) throw new Error("No compatible movement input");
    await movement.selectOption(chosen);
    const submit = page.getByRole("button", {
      name: /^提交执行$|^Submit execution$/,
    });
    await expect(submit).toBeEnabled();
    l.resumeStarted = true;
    save();
    const accepted = page.waitForResponse(
      (r) =>
        r.request().method() === "POST" &&
        new URL(r.url()).pathname === `/api/v1/jobs/${l.job}/resume`,
      { timeout: 120000 },
    );
    await submit.click();
    const receipt = await accepted;
    expect(receipt.status()).toBe(202);
    expect((await receipt.json()).id).toBe(l.job);
    l.resumeAccepted = true;
    save();
  } else {
    expect(l.resumeStarted).toBe(true);
    expect(before.job.status).toBe(3);
  }
  await expect
    .poll(
      async () => {
        const current = await read();
        expect(digest(prefix(current))).toBe(l.prefixHash);
        return current.job.status;
      },
      { timeout: 180000, intervals: [1000, 2000, 5000] },
    )
    .toBe(3);
  const review = await read();
  const video = review.step_runs.find(
    (s: { step_key: string }) => s.step_key === "video",
  );
  expect(video.candidates).toHaveLength(1);
  expect(
    l.confirmStarted,
    "unknown approval: observe original receipt, never submit another decision",
  ).not.toBe(true);
  await page
    .getByRole("button", { name: "选择节点 video", exact: true })
    .click();
  await page.getByRole("radio", { name: /^候选 1$|^Candidate 1$/ }).check();
  await expect(
    page.getByRole("button", { name: /^确认使用$|^Accept result$/ }),
  ).toBeEnabled({ timeout: 60000 });
  l.confirmStarted = true;
  save();
  const decision = page.waitForResponse(
    (r) =>
      r.request().method() === "POST" &&
      new URL(r.url()).pathname === `/api/v1/step-runs/${video.id}/confirm`,
    { timeout: 120000 },
  );
  await page
    .getByRole("button", { name: /^确认使用$|^Accept result$/ })
    .click();
  expect((await decision).ok()).toBeTruthy();
  await expect
    .poll(async () => (await read()).job.status, { timeout: 120000 })
    .toBe(4);
  const final = await read();
  expect(digest(prefix(final))).toBe(l.prefixHash);
  const completedVideo = final.step_runs.find(
    (s: { step_key: string }) => s.step_key === "video",
  );
  expect(completedVideo.status).toBe(5);
  expect(completedVideo.output_asset_id).toBeTruthy();
  const assetResponse = await page.request.get(
    `/api/v1/generated-assets/${completedVideo.output_asset_id}?organization_id=${l.organization}`,
  );
  expect(assetResponse.ok()).toBeTruthy();
  const asset = await assetResponse.json();
  const objectResponse = await page.request.get(
    `/api/v1/library-objects:resolve?organization_id=${l.organization}&object_path=${encodeURIComponent(asset.object_path)}`,
  );
  expect(objectResponse.ok()).toBeTruthy();
  const object = await objectResponse.json();
  const media = await page.request.get(object.url, {
    headers: { Range: "bytes=0-0" },
  });
  expect([200, 206]).toContain(media.status());
  const original = await read(l.source);
  expect(original.job.status).toBe(7);
  expect(digest(prefix(original))).toBe(l.prefixHash);
  for (const id of l.originals) {
    const originalJob = await read(id);
    expect(originalJob.job.status).toBe(7);
  }
  l.completed = true;
  l.output = completedVideo.output_asset_id;
  save();
  for (const id of [l.workflow, ...Object.values(l.templates)]) {
    const kind = id === l.workflow ? "workflows" : "templates";
    const response = await page.request.post(`/api/v1/${kind}/${id}/archive`);
    expect(response.ok()).toBeTruthy();
  }
});
