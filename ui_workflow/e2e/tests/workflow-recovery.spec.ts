import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import type { Page } from "@playwright/test";
import { expect, openStandaloneApp, test } from "../fixtures/base";

// A separate partial sample. The complete successful sample is never touched.
// The independent provider_fixture client fails its first model dispatch only.
// Stop that client between retry and rerun gates. No historical Job is resumed.
const path = resolve(
  process.env.E2E_WORKFLOW_RECOVERY_STAGE === "rerun"
    ? "../media_worker/.fixture-ui-rerun-ledger.json"
    : "../media_worker/.fixture-ui-recovery-ledger.json",
);
type Ledger = {
  name: string;
  organization: string;
  workflow: string;
  job: string;
  templates: Record<string, string>;
  retryStarted?: boolean;
  retryDone?: boolean;
  rerunStarted?: boolean;
  rerunDone?: boolean;
};
function load(): Ledger {
  return JSON.parse(readFileSync(path, "utf8"));
}
function save(value: Ledger) {
  writeFileSync(path, JSON.stringify(value), { mode: 0o600 });
}
async function read(page: Page, ledger: Ledger) {
  const response = await page.request.get(`/api/v1/jobs/${ledger.job}`, {
    timeout: 60000,
  });
  expect(response.ok()).toBeTruthy();
  const detail = await response.json();
  expect(detail.job.workflow_id).toBe(ledger.workflow);
  expect(detail.job.organization_id).toBe(ledger.organization);
  expect(detail.job.workflow_version).toBe(1);
  return detail;
}
async function openJob(page: Page, ledger: Ledger) {
  await openStandaloneApp(page);
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page
    .getByTestId(`execution-row-${ledger.job}`)
    .getByRole("button")
    .filter({ has: page.getByRole("heading", { level: 2 }) })
    .click();
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
}
async function cancel(page: Page, ledger: Ledger) {
  await page
    .getByRole("button", { name: /^取消执行$|^Cancel execution$/ })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const response = page.waitForResponse(
    (r) =>
      r.request().method() === "POST" &&
      new URL(r.url()).pathname === `/api/v1/jobs/${ledger.job}/cancel`,
  );
  await dialog
    .getByRole("button", { name: /^确认操作$|^Confirm action$/ })
    .click();
  expect((await response).ok()).toBeTruthy();
  await expect
    .poll(async () => (await read(page, ledger)).job.status, { timeout: 60000 })
    .toBe(6);
}

test("continue an isolated failed workflow through native retry and cancel before downstream", async ({
  page,
}) => {
  test.skip(
    process.env.E2E_WORKFLOW_RECOVERY_STAGE !== "retry",
    "explicit partial recovery gate",
  );
  const ledger = load();
  expect(
    ledger.retryDone,
    "already verified: do not create a new retry intent",
  ).not.toBe(true);
  const before = await read(page, ledger);
  if (!ledger.retryStarted) {
    expect(before.job.status).toBe(5);
    const failed = before.step_runs.find(
      (s: { step_key: string }) => s.step_key === "model_image",
    );
    expect(failed.status).toBe(6);
    await openJob(page, ledger);
    await page
      .getByRole("button", { name: /^失败继续$|^Continue failed execution$/ })
      .click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    ledger.retryStarted = true;
    save(ledger);
    const accepted = page.waitForResponse(
      (r) =>
        r.request().method() === "POST" &&
        new URL(r.url()).pathname === `/api/v1/jobs/${ledger.job}/retry`,
      { timeout: 120000 },
    );
    await dialog
      .getByRole("button", { name: /^确认操作$|^Confirm action$/ })
      .click();
    const response = await accepted;
    expect(response.ok()).toBeTruthy();
    expect(response.request().postDataJSON()).toEqual({});
  } else {
    // Unknown outcomes are observed, not replayed with a new native retry.
    expect([2, 3, 6]).toContain(before.job.status);
    await openJob(page, ledger);
  }
  if ((await read(page, ledger)).job.status !== 6) {
    await expect
      .poll(async () => (await read(page, ledger)).job.status, {
        timeout: 120000,
        intervals: [1000, 2000, 5000],
      })
      .toBe(3);
    const after = await read(page, ledger);
    const model = after.step_runs.find(
      (s: { step_key: string }) => s.step_key === "model_image",
    );
    expect(model.status).toBe(4);
    expect(model.candidates).toHaveLength(2);
    expect(
      after.step_runs.filter(
        (s: { step_key: string; status: number }) =>
          s.step_key !== "model_image" && [3, 4, 5].includes(s.status),
      ),
    ).toHaveLength(0);
    await cancel(page, ledger);
  }
  ledger.retryDone = true;
  save(ledger);
});

test("rerun the original partial Job with explicit input override then cancel", async ({
  page,
}) => {
  test.skip(
    process.env.E2E_WORKFLOW_RECOVERY_STAGE !== "rerun",
    "stop external mock client before this gate",
  );
  const ledger = load();
  expect(
    ledger.rerunStarted,
    "unknown rerun: query original Job, never blindly POST again",
  ).not.toBe(true);
  const before = await read(page, ledger);
  expect(before.job.status).toBe(5);
  const versions: string[] = [];
  page.on("request", (r) => {
    const pathname = new URL(r.url()).pathname;
    if (pathname.includes(`/workflows/${ledger.workflow}/versions/`))
      versions.push(pathname);
  });
  await openJob(page, ledger);
  await page
    .getByRole("button", {
      name: /^修改输入重新运行$|^Rerun with changed inputs$/,
    })
    .click();
  const frozenResponse = await page.request.get(
    `/api/v1/workflows/${ledger.workflow}/versions/1`,
  );
  expect(frozenResponse.ok()).toBeTruthy();
  const frozen = await frozenResponse.json();
  let changed = false;
  for (const entry of frozen.input_schema.entries) {
    const picker = page.getByTestId(`run-input-${entry.name}`);
    await expect(picker).toBeVisible({ timeout: 60000 });
    await expect
      .poll(() => picker.locator("option").count(), { timeout: 60000 })
      .toBeGreaterThan(1);
    const choices = await picker
      .locator("option")
      .evaluateAll((options) =>
        options.map((o) => (o as HTMLOptionElement).value).filter(Boolean),
      );
    const original =
      entry.resource_type === "PRODUCT"
        ? before.job.input.product?.id
        : entry.resource_type === "VIRTUAL_IP"
          ? before.job.input.virtual_ip?.id
          : undefined;
    const chosen =
      choices.find((id) => original && id !== original) ?? choices[0];
    if (original && chosen !== original) changed = true;
    await picker.selectOption(chosen);
  }
  expect(
    changed,
    "must explicitly change a resource, not just replay old input",
  ).toBe(true);
  const submit = page.getByRole("button", {
    name: /^提交执行$|^Submit execution$/,
  });
  await expect(submit).toBeEnabled();
  expect(versions).toContain(`/api/v1/workflows/${ledger.workflow}/versions/1`);
  ledger.rerunStarted = true;
  save(ledger);
  const accepted = page.waitForResponse(
    (r) =>
      r.request().method() === "POST" &&
      new URL(r.url()).pathname === `/api/v1/jobs/${ledger.job}/retry`,
    { timeout: 120000 },
  );
  await submit.click();
  const response = await accepted;
  expect(response.ok()).toBeTruthy();
  const request = response.request().postDataJSON();
  expect(request.input_override).toBeTruthy();
  expect(request.input_override).not.toEqual(before.job.input);
  const receipt = await response.json();
  expect(receipt.id).toBe(ledger.job);
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
  await expect
    .poll(async () => (await read(page, ledger)).job.status, { timeout: 60000 })
    .toBe(2);
  await cancel(page, ledger);
  const jobs = await page.request.get(
    `/api/v1/jobs?workflow_id=${ledger.workflow}`,
  );
  expect(jobs.ok()).toBeTruthy();
  expect((await jobs.json()).total).toBe(1);
  ledger.rerunDone = true;
  save(ledger);
  for (const id of [ledger.workflow, ...Object.values(ledger.templates)]) {
    const kind = id === ledger.workflow ? "workflows" : "templates";
    expect(
      (await page.request.post(`/api/v1/${kind}/${id}/archive`)).ok(),
    ).toBeTruthy();
  }
});
