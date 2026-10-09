import { expect, test } from "../fixtures/base";

// TestRail case ID pending. This gate registers definitions, never runs a Provider.
test("creates, validates, versions and publishes a workflow from the visual editor", async ({
  page,
}) => {
  const name = `ui-flow-${Date.now()}`;
  let id: string | undefined;
  let templateID: string | undefined;
  let expectedName = name;
  try {
    const templateResponse = await page.request.get(
      "/api/v1/templates/tpl_ec01393d-9d36-49dd-a8e8-5e75da35d1f1",
    );
    expect(templateResponse.ok()).toBeTruthy();
    const sourceTemplate = await templateResponse.json();
    expect(sourceTemplate.status).toBe(2);
    const workspaceResponse = await page.request.get("/api/v1/workspaces");
    expect(workspaceResponse.ok()).toBeTruthy();
    const { workspaces } = await workspaceResponse.json();
    expect(workspaces.length).toBeGreaterThan(0);
    const setup = await page.request.post(
      `/api/v1/templates?scope=${encodeURIComponent(sourceTemplate.scope)}`,
      {
        data: {
          ...sourceTemplate,
          id: undefined,
          status: undefined,
          version: undefined,
          organization_id: workspaces[0].organization_id,
          name: `${name}-template`,
          prompt_body:
            "UI workflow contract: use the supplied product and character references.",
        },
      },
    );
    expect(setup.ok()).toBeTruthy();
    templateID = (await setup.json()).id;
    expect(templateID).toBeTruthy();
    expect(
      (await page.request.post(`/api/v1/templates/${templateID}/publish`)).ok(),
    ).toBeTruthy();
    const template = await (
      await page.request.get(`/api/v1/templates/${templateID}`)
    ).json();
    expect(template.organization_id).toBe(workspaces[0].organization_id);
    await page.goto("/");
    await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
    await page
      .getByRole("button", { name: /^新建工作流$|^New workflow$/ })
      .click();
    const settings = page.getByRole("dialog");
    await settings.getByRole("textbox", { name: /^名称$|^Name$/ }).fill(name);
    await settings
      .getByRole("textbox", { name: /^业务范围$|^Scope$/ })
      .fill(template.scope);
    await settings
      .getByRole("combobox", { name: /^归属工作空间$|^Workspace$/ })
      .selectOption(workspaces[0].id);
    await page.keyboard.press("Escape");
    await page
      .getByRole("complementary", { name: /^节点工具箱$|^Step toolbox$/ })
      .getByRole("button", { name: /生成节点|Generation step/ })
      .click();
    const picker = page.getByRole("dialog", {
      name: /^选择生成模板$|^Choose a generation template$/,
    });
    await picker
      .getByRole("textbox", { name: /^搜索模板名称$|^Search template names$/ })
      .fill(template.name);
    await picker.getByRole("button").filter({ hasText: template.name }).click();
    await page
      .getByRole("textbox", { name: /^节点标识$|^Step key$/ })
      .fill("image");
    await page.getByText(/^配置输入 ·|^Configure inputs ·/).click();
    for (const slot of template.inputs) {
      const type = String(slot.resource_type);
      const ref =
        type === "VIRTUAL_IP" || type === "4"
          ? "virtual_ip"
          : type === "PRODUCT" || type === "3"
            ? "product"
            : slot.name;
      await page
        .getByRole("textbox", {
          name: `工作流输入名称 ${slot.name}`,
          exact: true,
        })
        .fill(ref);
    }
    const validatedResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().includes("/api/v1/workflows/validate"),
    );
    await page.getByRole("button", { name: /^校验$|^Validate$/ }).click();
    const validation = await validatedResponse;
    expect(validation.ok()).toBeTruthy();
    await expect(
      page.getByText(/结构校验通过|Structure is valid/),
    ).toBeVisible();
    const createdResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/v1\/workflows(?:\?|$)/.test(response.url()),
    );
    await page.getByRole("button", { name: /^保存草稿$|^Save draft$/ }).click();
    const created = await createdResponse;
    expect(created.ok()).toBeTruthy();
    const receipt = await created.json();
    id = receipt.id;
    expect(id).toBeTruthy();
    await page.getByRole("button", { name: /^编辑$|^Edit$/ }).click();
    await page
      .getByRole("button", { name: /^工作流设置$|^Workflow settings$/ })
      .click();
    expectedName = `${name}-revised`;
    await page
      .getByRole("dialog")
      .getByRole("textbox", { name: /^名称$|^Name$/ })
      .fill(expectedName);
    await page.keyboard.press("Escape");
    const updatedResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().includes(`/api/v1/workflows/${id}`),
    );
    await page.getByRole("button", { name: /^保存草稿$|^Save draft$/ }).click();
    expect((await updatedResponse).ok()).toBeTruthy();
    await page.getByRole("button", { name: /^发布$|^Publish$/ }).click();
    await expect
      .poll(
        async () =>
          (await (await page.request.get(`/api/v1/workflows/${id}`)).json())
            .publication_status,
        { timeout: 60000 },
      )
      .toBe("registered");
    const published = await (
      await page.request.get(`/api/v1/workflows/${id}`)
    ).json();
    expect(published.version).toBe(2);
    expect(published.steps).toHaveLength(1);
    expect(published.steps[0].key).toBe("image");
    await expect(
      page.getByRole("button", { name: /^运行工作流$|^Run workflow$/ }),
    ).toBeVisible();
  } finally {
    if (id) {
      const owned = await (
        await page.request.get(`/api/v1/workflows/${id}`)
      ).json();
      expect([name, expectedName]).toContain(owned.name);
      expect(
        (await page.request.post(`/api/v1/workflows/${id}/archive`)).ok(),
      ).toBeTruthy();
    }
    if (templateID) {
      const owned = await (
        await page.request.get(`/api/v1/templates/${templateID}`)
      ).json();
      expect(owned.name).toBe(`${name}-template`);
      expect(
        (
          await page.request.post(`/api/v1/templates/${templateID}/archive`)
        ).ok(),
      ).toBeTruthy();
    }
  }
});
