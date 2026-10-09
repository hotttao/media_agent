import { expect, test } from "../fixtures/base";

// TestRail ID has not been assigned. Do not borrow an unrelated case ID.
test("creates, versions and publishes an isolated template through the UI", async ({
  page,
}) => {
  const name = `ui-contract-${Date.now()}`;
  const prompt =
    "UI contract probe: preserve the supplied character reference.";
  let id: string | undefined;
  try {
    await page.goto("/");
    await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
    await page.getByRole("tab", { name: /^模板$|^Templates$/ }).click();
    await page
      .getByRole("button", { name: /^新建模板$|^New template$/ })
      .click();
    const settings = page.getByRole("dialog");
    await settings
      .getByRole("textbox", { name: /^业务范围$|^Scope$/ })
      .fill("ai_clothing");
    await settings
      .getByRole("textbox", { name: /^模板名称$|^Template name$/ })
      .fill(name);
    await page.keyboard.press("Escape");
    await page
      .getByRole("textbox", { name: /^提示词模板$|^Prompt template$/ })
      .fill(prompt);
    await page
      .getByRole("textbox", {
        name: /^生成能力契约$|^Generation capability contract$/,
      })
      .fill("jimeng.i2i.multi_image_prompt");
    await page.getByRole("button", { name: /^添加槽位$|^Add slot$/ }).click();
    await page
      .getByRole("textbox", { name: /^槽位名称$|^Slot name$/ })
      .fill("person");
    await page
      .getByRole("combobox", { name: /^资源类别 1$|^Resource type 1$/ })
      .click();
    await page.getByRole("option", { name: "VIRTUAL_IP", exact: true }).click();
    await page
      .getByRole("combobox", {
        name: /^注册资源类型 1$|^Registered asset kind 1$/,
      })
      .click();
    await page.getByRole("option", { name: "virtual_ip", exact: true }).click();
    await page
      .getByRole("combobox", { name: /^产出资源类型$|^Output asset kind$/ })
      .click();
    await page
      .getByRole("option", { name: "model_image", exact: true })
      .click();
    await page.getByRole("checkbox", { name: /^必填$|^Required$/ }).check();
    await page
      .getByRole("button", {
        name: /^按槽位生成图片调用映射$|^Build image invocation from slots$/,
      })
      .click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: /^确认替换映射$|^Replace mappings$/ })
      .click();
    const createdResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/v1\/templates(?:\?|$)/.test(response.url()),
    );
    await page.getByRole("button", { name: /^保存草稿$|^Save draft$/ }).click();
    const created = await createdResponse;
    const receipt = await created.json();
    id = receipt.id;
    expect(created.ok()).toBeTruthy();
    expect(id).toBeTruthy();
    await expect(
      page.getByRole("heading", { name, exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: /^发布当前版本$|^Publish current version$/ })
      .click();
    await expect
      .poll(
        async () =>
          (await (await page.request.get(`/api/v1/templates/${id}`)).json())
            .status,
        { timeout: 30000 },
      )
      .toBe(2);
    await page.getByRole("button", { name: /^编辑$|^Edit$/ }).click();
    await page
      .getByRole("textbox", { name: /^提示词模板$|^Prompt template$/ })
      .fill(`${prompt}\nSecond draft version.`);
    const updatedResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().includes(`/api/v1/templates/${id}`),
    );
    await page.getByRole("button", { name: /^保存草稿$|^Save draft$/ }).click();
    expect((await updatedResponse).ok()).toBeTruthy();
    await expect(
      page.getByRole("button", {
        name: /^发布当前版本$|^Publish current version$/,
      }),
    ).toBeVisible();
    const current = await (
      await page.request.get(`/api/v1/templates/${id}`)
    ).json();
    expect(current.version).toBe(2);
    expect(current.status).toBe(1);
    const frozen = await (
      await page.request.get(`/api/v1/templates/${id}/versions/1`)
    ).json();
    expect(frozen.prompt_body).toBe(prompt);
    expect(frozen.invocation_mapping.images.items[0].slot).toBe("person");
    await page
      .getByRole("button", { name: /^发布当前版本$|^Publish current version$/ })
      .click();
    await expect
      .poll(
        async () =>
          (await (await page.request.get(`/api/v1/templates/${id}`)).json())
            .status,
        { timeout: 30000 },
      )
      .toBe(2);
  } finally {
    if (id) {
      const owned = await (
        await page.request.get(`/api/v1/templates/${id}`)
      ).json();
      expect(owned.name).toBe(name);
      const archived = await page.request.post(
        `/api/v1/templates/${id}/archive`,
      );
      expect(archived.ok()).toBeTruthy();
    }
  }
});
