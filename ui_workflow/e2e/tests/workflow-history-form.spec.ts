import { expect, openStandaloneApp, test } from "../fixtures/base";

// Borrowed history remains untouched. This is form/query verification, not a
// real Resume execution gate. Actual resume requires an isolated import fixture.
test("historical continuation reads frozen inputs and preserves the completed prefix without submitting", async ({
  page,
}) => {
  let writes = 0;
  const versions: string[] = [];
  await page.route("**/api/v1/**", async (route) => {
    if (route.request().method() !== "GET") {
      writes++;
      await route.abort();
    } else await route.continue();
  });
  page.on("request", (r) => {
    const pathname = new URL(r.url()).pathname;
    if (/\/workflows\/[^/]+\/versions\/\d+$/.test(pathname))
      versions.push(pathname);
  });
  await openStandaloneApp(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  const list = await page.request.get(
    "/api/v1/jobs?status=awaiting_resume&page_size=20",
  );
  expect(list.ok()).toBeTruthy();
  const job = (await list.json()).items[0];
  expect(job.origin).toBe("legacy_import");
  expect(job.status).toBe(7);
  const read = async () => {
    const response = await page.request.get(`/api/v1/jobs/${job.id}`);
    expect(response.ok()).toBeTruthy();
    return response.json();
  };
  const before = await read();
  const outputs = before.step_runs.map(
    (s: { step_key: string; status: number; output_asset_id: string }) => ({
      key: s.step_key,
      status: s.status,
      asset: s.output_asset_id,
    }),
  );
  expect(
    outputs.filter((s: { status: number }) => s.status === 5).length,
  ).toBeGreaterThan(0);
  const response = await page.request.get(
    `/api/v1/workflows/${job.workflow_id}/versions/${job.workflow_version}`,
  );
  expect(response.ok()).toBeTruthy();
  const frozen = await response.json();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await page
    .getByRole("button", { name: /^待继续$|^Awaiting continuation$/ })
    .click();
  await page
    .getByTestId(`execution-row-${job.id}`)
    .getByRole("button")
    .filter({ has: page.getByRole("heading", { level: 2 }) })
    .click();
  await page
    .getByRole("button", {
      name: /^补缺输入继续$|^Resume with missing inputs$/,
    })
    .click();
  await expect
    .poll(() =>
      versions.includes(
        `/api/v1/workflows/${job.workflow_id}/versions/${job.workflow_version}`,
      ),
    )
    .toBe(true);
  for (const entry of frozen.input_schema.entries) {
    const present =
      entry.resource_type === "PRODUCT"
        ? before.job.input.product
        : entry.resource_type === "VIRTUAL_IP"
          ? before.job.input.virtual_ip
          : (entry.resource_type === "ASSET"
              ? before.job.input.asset
              : before.job.input.material
            )?.some((item: { name: string }) => item.name === entry.name);
    const picker = page.getByTestId(`run-input-${entry.name}`);
    if (present) await expect(picker).toHaveCount(0);
    else {
      await expect(picker).toBeVisible({ timeout: 60000 });
      await expect
        .poll(() => picker.locator("option").count(), { timeout: 60000 })
        .toBeGreaterThan(1);
      const choice = await picker
        .locator("option")
        .evaluateAll((options) =>
          options.map((o) => (o as HTMLOptionElement).value).find(Boolean),
        );
      if (!choice)
        throw new Error("Missing compatible historical input option");
      await picker.selectOption(choice);
    }
  }
  await expect(
    page.getByRole("button", { name: /^提交执行$|^Submit execution$/ }),
  ).toBeEnabled();
  expect(writes).toBe(0);
  const after = await read();
  expect(after.job.status).toBe(7);
  expect(after.job.input).toEqual(before.job.input);
  expect(
    after.step_runs.map(
      (s: { step_key: string; status: number; output_asset_id: string }) => ({
        key: s.step_key,
        status: s.status,
        asset: s.output_asset_id,
      }),
    ),
  ).toEqual(outputs);
});
