import { resolve } from "node:path";
import { expect, openStandaloneApp, test } from "../fixtures/base";

test("v1.2 keeps application view when switching standalone slots", async ({
  page,
}) => {
  await openStandaloneApp(page);
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^任务$|^Jobs$/ }).click();
  await expect(
    page.getByRole("heading", { name: /^我的任务$|^My tasks$/ }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: /^搜索名称$|^Search by name$/ })
    .fill("slot-state-check");
  await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
  await expect(
    page.getByRole("tab", { name: /^商品$|^Products$/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await expect(
    page.getByRole("heading", { name: /^我的任务$|^My tasks$/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: /^搜索名称$|^Search by name$/ }),
  ).toHaveValue("slot-state-check");
});

test("built plugin preserves host theme, narrow slot and portalled forms", async ({
  page,
}) => {
  // Host compatibility check, not an execution E2E or a production host loader.
  await page.goto("/e2e/host-fixture.html");
  await page.waitForFunction(
    () =>
      (window as unknown as { hostFixtureReady?: boolean }).hostFixtureReady,
  );
  const before = await page.evaluate(() => ({
    body: getComputedStyle(document.body).backgroundColor,
    font: getComputedStyle(document.body).fontFamily,
    primary: getComputedStyle(document.documentElement).getPropertyValue(
      "--primary",
    ),
  }));
  await page.addScriptTag({ path: resolve("dist/plugin/media-workflow.js") });
  await expect(
    page.getByRole("tab", { name: /^工作流$|^Workflows$/ }),
  ).toBeVisible();
  await expect(page.locator("#host-panel header")).toHaveCount(1); // App content, no nested Desktop.
  const after = await page.evaluate(() => ({
    body: getComputedStyle(document.body).backgroundColor,
    font: getComputedStyle(document.body).fontFamily,
    primary: getComputedStyle(document.documentElement).getPropertyValue(
      "--primary",
    ),
  }));
  expect(after).toEqual(before);
  await page.locator("#host-panel").evaluate((element) => {
    element.style.width = "480px";
  });
  const columns = await page
    .locator(".media-list-layout")
    .evaluate(
      (element) =>
        getComputedStyle(element).gridTemplateColumns.split(" ").length,
    );
  expect(columns).toBe(1);
  await page
    .getByRole("button", { name: /^新建工作流$|^New workflow$/ })
    .click();
  // A new draft opens required metadata immediately. Radix intentionally hides
  // the background toolbar from the accessibility tree while the modal is open.
  const settings = page.getByRole("dialog", { name: "工作流设置" });
  await expect(settings).toBeVisible();
  await expect(
    settings.getByRole("textbox", { name: "名称", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator('[data-media-ui="portal"]')
      .getByRole("dialog", { name: "工作流设置" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
});
