import { expect, test } from "../fixtures/base";

test("execution details show actual steps, preview results and explain frozen topology", async ({
  page,
}) => {
  await page.goto("/");
  await page.setViewportSize({ width: 1440, height: 900 });
  const response = await page.request.get("/api/v1/jobs?page=1&page_size=20", {
    timeout: 120000,
  });
  expect(response.ok()).toBeTruthy();
  const { items } = await response.json();
  let detail;
  for (const item of items) {
    const result = await page.request.get(
      `/api/v1/jobs/${encodeURIComponent(item.id)}`,
    );
    if (!result.ok()) continue;
    const candidate = await result.json();
    if (candidate.step_runs?.length) {
      detail = candidate;
      break;
    }
  }
  expect(detail, "requires an existing readable execution").toBeTruthy();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await expect(page.getByTestId(`execution-row-${detail.job.id}`)).toBeVisible({
    timeout: 120000,
  });
  await page
    .getByTestId(`execution-row-${detail.job.id}`)
    .getByRole("button")
    .filter({ has: page.getByRole("heading", { level: 2 }) })
    .click();
  const nodes = page.getByRole("navigation", {
    name: /^执行节点$|^Execution steps$/,
  });
  await expect(nodes).toBeVisible();
  const step = detail.step_runs[0];
  await nodes.getByRole("button").filter({ hasText: step.step_key }).click();
  await expect(
    page.getByRole("article", { name: /^节点结果$|^Step results$/ }),
  ).toContainText(step.step_key);
  await page.screenshot({ path: "/tmp/media-agent-v12-execution-live.png" });
  await page
    .getByRole("button", { name: /^查看执行图$|^View execution graph$/ })
    .click();
  await expect(
    page.getByRole("dialog", { name: /^查看执行图$|^View execution graph$/ }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 480, height: 900 });
  await expect(nodes).toBeVisible();
  await page.screenshot({
    path: "/tmp/media-agent-v12-execution-narrow-live.png",
  });
  // Read-only verification: no cancel, retry, confirm or regenerate commands.
});

test("run form requires a real workspace and typed inputs without submitting", async ({
  page,
}) => {
  await page.goto("/");
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page
    .getByRole("button", { name: /^运行工作流$|^Run workflow$/ })
    .click();
  await expect(
    page.getByRole("combobox", { name: /^归属工作空间$|^Workspace$/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^提交执行$|^Submit execution$/ }),
  ).toBeDisabled();
  const workflowID = "wf_2c1231e5-7379-4207-a0de-a9bb5d93b278";
  const definitionResponse = await page.request.get(
    `/api/v1/workflows/${workflowID}`,
  );
  expect(definitionResponse.ok()).toBeTruthy();
  const definition = await definitionResponse.json();
  expect(definition.publication_status).toBe("registered");
  await page
    .getByRole("combobox", { name: /^工作流$|^Workflows$/ })
    .selectOption(workflowID);
  for (const entry of definition.input_schema.entries) {
    const picker = page.getByTestId(`run-input-${entry.name}`);
    await expect(picker).toBeVisible({ timeout: 60000 });
    await expect
      .poll(
        () =>
          picker.evaluate(
            (node: HTMLSelectElement) =>
              Array.from(node.options).filter((option) => option.value).length,
          ),
        { timeout: 30000 },
      )
      .toBeGreaterThan(0);
    const choice = await picker.evaluate(
      (node: HTMLSelectElement) =>
        Array.from(node.options).find((option) => option.value)?.value,
    );
    await picker.selectOption(choice!);
  }
  const workspaceResponse = await page.request.get("/api/v1/workspaces");
  expect(workspaceResponse.ok()).toBeTruthy();
  const { workspaces } = await workspaceResponse.json();
  await page
    .getByRole("combobox", { name: /^归属工作空间$|^Workspace$/ })
    .selectOption(workspaces[0].id);
  await expect(
    page.getByRole("button", { name: /^提交执行$|^Submit execution$/ }),
  ).toBeEnabled();
  const images = page.getByRole("img");
  expect(await images.count()).toBeGreaterThan(0);
  await expect
    .poll(
      () =>
        images.evaluateAll((nodes) =>
          nodes.some(
            (node) => node instanceof HTMLImageElement && node.naturalWidth > 0,
          ),
        ),
      { timeout: 30000 },
    )
    .toBeTruthy();
  await page.screenshot({ path: "/tmp/media-agent-v12-run-form-live.png" });
  // Inputs and readiness only: do not submit another execution or consume a worker queue.
});
