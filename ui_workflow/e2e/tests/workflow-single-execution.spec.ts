import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, openStandaloneApp, test } from "../fixtures/base";

// Operator launches media_worker between these two gates. No embedded Worker,
// repeated Submit, borrowed definition edits or automatic historical Resume.
const recovery = ["true", "rerun"].includes(
  process.env.E2E_WORKFLOW_RECOVERY ?? "",
);
const ledgerPath = resolve(
  process.env.E2E_WORKFLOW_RECOVERY === "rerun"
    ? "../media_worker/.fixture-ui-rerun-ledger.json"
    : recovery
      ? "../media_worker/.fixture-ui-recovery-ledger.json"
      : "../media_worker/.fixture-ui-single-ledger.json",
);
type Ledger = {
  name: string;
  organization: string;
  workspace: string;
  workflow?: string;
  job?: string;
  templates?: Record<string, string>;
  submitStarted?: boolean;
  regenerationStarted?: boolean;
  regenerationReceipt?: string;
  confirmed?: string[];
};
function readLedger(): Ledger {
  return JSON.parse(readFileSync(ledgerPath, "utf8"));
}
function saveLedger(value: Ledger, exclusive = false) {
  writeFileSync(ledgerPath, JSON.stringify(value), {
    mode: 0o600,
    flag: exclusive ? "wx" : "w",
  });
}

test("submit one isolated workflow from the UI", async ({ page }) => {
  test.skip(
    process.env.E2E_SINGLE_WORKFLOW_STAGE !== "submit",
    "explicit single sample only",
  );
  await openStandaloneApp(page);
  const sourceResponse = await page.request.get(
    "/api/v1/workflows/wf_2c1231e5-7379-4207-a0de-a9bb5d93b278",
  );
  expect(sourceResponse.ok()).toBeTruthy();
  const source = await sourceResponse.json();
  expect(source.steps).toHaveLength(4);
  const me = await (await page.request.get("/api/auth/me")).json();
  const workspaceResponse = await page.request.get("/api/v1/workspaces");
  expect(workspaceResponse.ok()).toBeTruthy();
  const available = (await workspaceResponse.json()).workspaces;
  const workspace =
    available.find((item: { id: string }) => item.id === source.workspace_id) ||
    available[0];
  expect(workspace?.organization_id).toBeTruthy();
  let ledger: Ledger;
  if (existsSync(ledgerPath)) {
    ledger = readLedger();
    expect(
      ledger.job,
      "existing Job: use review stage, never submit another",
    ).toBeUndefined();
    expect(
      ledger.submitStarted,
      "unknown Submit outcome: inspect original intent before proceeding",
    ).not.toBe(true);
  } else {
    for (let pageNumber = 1; ; pageNumber++) {
      const response = await page.request.get(
        `/api/v1/jobs?page=${pageNumber}&page_size=200`,
        { timeout: 120000 },
      );
      expect(response.ok()).toBeTruthy();
      const jobs = await response.json();
      expect(
        (jobs.items ?? []).filter((job: { status: number }) =>
          [1, 2, 3, 8].includes(job.status),
        ),
      ).toHaveLength(0);
      if (pageNumber * 200 >= jobs.total) break;
      expect(jobs.items?.length).toBeGreaterThan(0);
    }
    ledger = {
      name: `ui-${recovery ? "recovery" : "single"}-mock-${Date.now()}`,
      organization: workspace.organization_id,
      workspace: workspace.id,
      confirmed: [],
    };
    saveLedger(ledger, true);
  }
  if (!ledger.workflow) {
    const matches = await (
      await page.request.get(
        `/api/v1/workflows?name=${encodeURIComponent(ledger.name)}&page_size=200`,
      )
    ).json();
    const exact = (matches.items ?? []).filter(
      (item: { name: string; created_by: string }) =>
        item.name === ledger.name && item.created_by === me.id,
    );
    expect(exact).toHaveLength(0); // Previously refused creation must not be blindly replayed.
    ledger.organization = workspace.organization_id;
    ledger.workspace = workspace.id;
    ledger.templates ??= {};
    saveLedger(ledger);
    for (const step of source.steps) {
      const name = `${ledger.name}-${step.key}`;
      if (!ledger.templates[step.key]) {
        const list = await (
          await page.request.get(
            `/api/v1/templates?name=${encodeURIComponent(name)}&page_size=200`,
          )
        ).json();
        const found = (list.items ?? []).filter(
          (item: { name: string; created_by: string }) =>
            item.name === name && item.created_by === me.id,
        );
        expect(found.length).toBeLessThanOrEqual(1);
        if (found.length) ledger.templates[step.key] = found[0].id;
        else {
          const frozenResponse = await page.request.get(
            `/api/v1/templates/${step.template_id}/versions/${step.template_version}`,
          );
          expect(frozenResponse.ok()).toBeTruthy();
          const frozen = await frozenResponse.json();
          const contractResponse = await page.request.get(
            `/api/v1/capabilities/${frozen.capability_key}/versions/${frozen.capability_version}`,
          );
          expect(contractResponse.ok()).toBeTruthy();
          const contract = await contractResponse.json();
          const created = await page.request.post(
            `/api/v1/templates?scope=${encodeURIComponent(source.scope)}`,
            {
              data: {
                ...frozen,
                name,
                organization_id: ledger.organization,
                platform: contract.provider,
                gen_way: contract.generation_method,
                gen_type: contract.generation_type,
                needs_confirm: true,
                candidate_count: step.key === "model_image" ? 2 : 1,
              },
            },
          );
          expect(
            created.ok(),
            `isolated template creation status=${created.status()}`,
          ).toBeTruthy();
          ledger.templates[step.key] = (await created.json()).id;
        }
        saveLedger(ledger);
      }
      const published = await page.request.post(
        `/api/v1/templates/${ledger.templates[step.key]}/publish`,
      );
      expect(published.ok()).toBeTruthy();
    }
    const created = await page.request.post(
      `/api/v1/workflows?scope=${encodeURIComponent(source.scope)}`,
      {
        data: {
          name: ledger.name,
          organization_id: ledger.organization,
          workspace_id: ledger.workspace,
          output_step_key: source.output_step_key,
          steps: source.steps.map((step: { key: string }) => ({
            ...step,
            template_id: ledger.templates![step.key],
            template_version: 1,
            approval_policy: "always",
            reuse_policy: "force_new",
          })),
        },
      },
    );
    expect(
      created.ok(),
      `isolated workflow creation status=${created.status()}`,
    ).toBeTruthy();
    ledger.workflow = (await created.json()).id;
    expect(ledger.workflow).toBeTruthy();
    saveLedger(ledger);
  }
  const current = await (
    await page.request.get(`/api/v1/workflows/${ledger.workflow}`)
  ).json();
  if (current.status === 1) {
    const published = await page.request.post(
      `/api/v1/workflows/${ledger.workflow}/publish`,
    );
    expect([200, 202]).toContain(published.status());
  }
  await expect
    .poll(
      async () => {
        const response = await page.request.get(
          `/api/v1/workflows/${ledger.workflow}`,
        );
        if (!response.ok()) return "unavailable";
        return (await response.json()).publication_status;
      },
      { timeout: 90000, intervals: [1000, 2000, 5000] },
    )
    .toBe("registered");
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page
    .getByRole("button", { name: /^运行工作流$|^Run workflow$/ })
    .click();
  await page
    .getByRole("combobox", { name: /^工作流$|^Workflows$/ })
    .selectOption(ledger.workflow!);
  const definition = await (
    await page.request.get(`/api/v1/workflows/${ledger.workflow}`)
  ).json();
  for (const entry of definition.input_schema.entries) {
    const picker = page.getByTestId(`run-input-${entry.name}`);
    await expect(picker).toBeVisible({ timeout: 60000 });
    await expect
      .poll(() => picker.locator("option").count(), { timeout: 60000 })
      .toBeGreaterThan(1);
    const choice = await picker
      .locator("option")
      .evaluateAll((options) =>
        options
          .map((option) => (option as HTMLOptionElement).value)
          .find(Boolean),
      );
    expect(choice).toBeTruthy();
    await picker.selectOption(choice!);
  }
  await page
    .getByRole("combobox", { name: /^归属工作空间$|^Workspace$/ })
    .selectOption(ledger.workspace);
  const submit = page.getByRole("button", {
    name: /^提交执行$|^Submit execution$/,
  });
  await expect(submit).toBeEnabled();
  ledger.submitStarted = true;
  saveLedger(ledger);
  const accepted = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/api\/v1\/jobs(?:\?|$)/.test(response.url()),
    { timeout: 120000 },
  );
  await submit.click();
  const response = await accepted;
  expect(response.ok()).toBeTruthy();
  ledger.job = (await response.json()).job_id;
  expect(ledger.job).toBeTruthy();
  saveLedger(ledger);
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
  console.log(`single UI accepted job_id=${ledger.job}`);
});

test("review the original mock workflow through completion", async ({
  page,
}) => {
  test.skip(
    recovery || process.env.E2E_SINGLE_WORKFLOW_STAGE !== "review",
    "external Worker must already be running",
  );
  const ledger = readLedger();
  expect(ledger.job).toBeTruthy();
  await page.goto("/");
  const read = async () => {
    const response = await page.request.get(`/api/v1/jobs/${ledger.job}`, {
      timeout: 60000,
    });
    expect(response.ok()).toBeTruthy();
    const detail = await response.json();
    expect(detail.job.workflow_id).toBe(ledger.workflow);
    expect(detail.job.organization_id).toBe(ledger.organization);
    return detail;
  };
  const initial = await read();
  expect([2, 3, 4]).toContain(initial.job.status);
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page
    .getByTestId(`execution-row-${ledger.job}`)
    .getByRole("button")
    .filter({ has: page.getByRole("heading", { level: 2 }) })
    .click();
  for (const key of ["model_image", "style_image", "first_frame", "video"]) {
    const existing = (await read()).step_runs.find(
      (item: { step_key: string }) => item.step_key === key,
    );
    if (existing?.status === 5) {
      expect(existing.output_asset_id).toBeTruthy();
      expect(existing.selected_index ?? 0).toBe(key === "model_image" ? 1 : 0);
      if (!ledger.confirmed?.includes(key)) {
        ledger.confirmed = [...(ledger.confirmed ?? []), key];
        saveLedger(ledger);
      }
      continue;
    }
    await expect
      .poll(
        async () => {
          const state = await read();
          if ([5, 6].includes(state.job.status)) return "terminal-failure";
          const step = state.step_runs.find(
            (item: { step_key: string }) => item.step_key === key,
          );
          return step?.status;
        },
        { timeout: 150000, intervals: [1000, 3000, 5000] },
      )
      .toBe(ledger.confirmed?.includes(key) ? 5 : 4);
    if (ledger.confirmed?.includes(key)) continue;
    await page
      .getByRole("button", { name: `选择节点 ${key}`, exact: true })
      .click();
    const panel = page.getByRole("article", {
      name: /^节点结果$|^Step results$/,
    });
    if (key === "model_image" && !ledger.regenerationReceipt) {
      expect(
        ledger.regenerationStarted,
        "unknown regeneration result: inspect same receipt before proceeding",
      ).not.toBe(true);
      await expect(panel.getByRole("radio")).toHaveCount(2);
      await expect(
        panel.getByRole("button", { name: /^确认使用$|^Accept result$/ }),
      ).toBeDisabled();
      await panel
        .getByRole("button", {
          name: /^反馈重新生成$|^Regenerate with feedback$/,
        })
        .click();
      await panel
        .getByRole("textbox", {
          name: /^重新生成反馈$|^Regeneration feedback$/,
        })
        .fill(
          "UI mock review: retain the reference and refine the composition.",
        );
      ledger.regenerationStarted = true;
      saveLedger(ledger);
      const accepted = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          response.url().includes("/regenerate"),
      );
      await panel
        .getByRole("button", { name: /^提交重新生成$|^Submit regeneration$/ })
        .click();
      const response = await accepted;
      expect(response.ok()).toBeTruthy();
      ledger.regenerationReceipt = (await response.json()).last_review_id;
      expect(ledger.regenerationReceipt).toBeTruthy();
      saveLedger(ledger);
      await expect
        .poll(
          async () => {
            const state = await read();
            const step = state.step_runs.find(
              (item: { step_key: string }) => item.step_key === key,
            );
            return (
              step?.status === 4 &&
              step?.last_review_id === ledger.regenerationReceipt &&
              step?.last_review_status === "applied"
            );
          },
          { timeout: 150000, intervals: [1000, 3000, 5000] },
        )
        .toBe(true);
    }
    const position = key === "model_image" ? 2 : 1;
    const radio = panel.getByRole("radio", {
      name: new RegExp(`^候选 ${position}$|^Candidate ${position}$`),
    });
    await expect(radio).toBeEnabled({ timeout: 60000 });
    await radio.check();
    const accepted = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().includes("/confirm"),
    );
    await panel
      .getByRole("button", { name: /^确认使用$|^Accept result$/ })
      .click();
    const response = await accepted;
    expect(response.ok()).toBeTruthy();
    await expect
      .poll(
        async () => {
          const state = await read();
          return state.step_runs.find(
            (item: { step_key: string }) => item.step_key === key,
          )?.status;
        },
        { timeout: 90000, intervals: [1000, 3000, 5000] },
      )
      .toBe(5);
    ledger.confirmed = [...(ledger.confirmed ?? []), key];
    saveLedger(ledger);
  }
  await expect
    .poll(async () => (await read()).job.status, {
      timeout: 90000,
      intervals: [1000, 3000, 5000],
    })
    .toBe(4);
  const final = await read();
  expect(final.step_runs).toHaveLength(4);
  for (const step of final.step_runs) {
    expect(step.output_asset_id).toBeTruthy();
    const output = await page.request.get(
      `/api/v1/generated-assets/${step.output_asset_id}?organization_id=${ledger.organization}`,
    );
    expect(output.ok()).toBeTruthy();
  }
  expect(
    new Set(
      final.step_runs.map(
        (step: { output_asset_id: string }) => step.output_asset_id,
      ),
    ).size,
  ).toBe(4);
  expect(ledger.regenerationReceipt).toBeTruthy();
  const archived = await page.request.post(
    `/api/v1/workflows/${ledger.workflow}/archive`,
  );
  expect(archived.ok()).toBeTruthy();
  for (const id of Object.values(ledger.templates ?? {})) {
    const archived = await page.request.post(`/api/v1/templates/${id}/archive`);
    expect(archived.ok()).toBeTruthy();
  }
  console.log(
    `single UI completed job_id=${ledger.job}; four outputs, regenerated first review, all approvals`,
  );
});
