import { expect, openStandaloneApp, test } from "../fixtures/base";

// Isolated definitions; a borrowed product is read-only. No Job is submitted.
test("product launches an instance-scoped run draft and never overwrites a cleared choice", async ({
  page,
}) => {
  const name = `ui-product-route-${Date.now()}`;
  let templateID: string | undefined;
  let workflowID: string | undefined;
  try {
    const workspacesResponse = await page.request.get("/api/v1/workspaces");
    expect(workspacesResponse.ok()).toBeTruthy();
    const { workspaces } = await workspacesResponse.json();
    const workspace = workspaces[0];
    const sourceResponse = await page.request.get(
      "/api/v1/workflows/wf_2c1231e5-7379-4207-a0de-a9bb5d93b278",
    );
    expect(sourceResponse.ok()).toBeTruthy();
    const source = await sourceResponse.json();
    const first = source.steps.find(
      (step: { key: string }) => step.key === "model_image",
    );
    const templateResponse = await page.request.get(
      `/api/v1/templates/${first.template_id}/versions/${first.template_version}`,
    );
    expect(templateResponse.ok()).toBeTruthy();
    const template = await templateResponse.json();
    const createdTemplate = await page.request.post(
      `/api/v1/templates?scope=${encodeURIComponent(source.scope)}`,
      {
        data: {
          ...template,
          id: undefined,
          status: undefined,
          version: undefined,
          organization_id: workspace.organization_id,
          name: `${name}-template`,
          prompt_body: "UI launch contract: use the supplied references.",
        },
      },
    );
    expect(createdTemplate.ok()).toBeTruthy();
    templateID = (await createdTemplate.json()).id;
    expect(
      (await page.request.post(`/api/v1/templates/${templateID}/publish`)).ok(),
    ).toBeTruthy();
    const createdWorkflow = await page.request.post(
      `/api/v1/workflows?scope=${encodeURIComponent(source.scope)}`,
      {
        data: {
          organization_id: workspace.organization_id,
          workspace_id: workspace.id,
          name,
          output_step_key: first.key,
          steps: [{ ...first, template_id: templateID, template_version: 1 }],
        },
      },
    );
    expect(createdWorkflow.ok()).toBeTruthy();
    workflowID = (await createdWorkflow.json()).id;
    if (!workflowID) throw new Error("Workflow create receipt lacks an ID");
    expect(
      (await page.request.post(`/api/v1/workflows/${workflowID}/publish`)).ok(),
    ).toBeTruthy();
    await expect
      .poll(
        async () =>
          (
            await (
              await page.request.get(`/api/v1/workflows/${workflowID}`)
            ).json()
          ).publication_status,
        { timeout: 60000 },
      )
      .toBe("registered");
    const productsResponse = await page.request.get(
      `/api/v1/products?organization_id=${workspace.organization_id}&scope=${source.scope}&status=active&page_size=1`,
    );
    expect(productsResponse.ok()).toBeTruthy();
    const { items } = await productsResponse.json();
    const product = items[0];
    expect(product.id).toBeTruthy();
    await openStandaloneApp(page);
    await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
    await page.getByRole("tab", { name: /^商品$|^Products$/ }).click();
    await page
      .getByRole("textbox", {
        name: /^按名称搜索商品$|^Search products by name$/,
      })
      .fill(product.name);
    await page
      .getByTestId(`product-card-${product.id}`)
      .getByRole("button")
      .filter({ has: page.getByRole("heading") })
      .click();
    await page
      .getByRole("button", {
        name: /^选择兼容工作流$|^Choose compatible workflow$/,
      })
      .click();
    const dialog = page.getByRole("dialog", {
      name: /^选择兼容工作流$|^Choose compatible workflow$/,
    });
    await dialog
      .getByRole("combobox", { name: /^可用工作流$|^Available workflow$/ })
      .selectOption(workflowID);
    await dialog
      .getByRole("button", {
        name: /^带入商品并准备运行$|^Prepare run with this product$/,
      })
      .click();
    await expect(
      page.getByRole("combobox", { name: /^工作流$|^Workflows$/ }),
    ).toHaveValue(workflowID);
    const selectedProduct = page.getByTestId("run-input-product");
    await expect(selectedProduct).toHaveValue(product.id);
    await expect(
      page.getByRole("button", { name: /^提交执行$|^Submit execution$/ }),
    ).toBeDisabled();
    await selectedProduct.selectOption("");
    await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
    await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
    await expect(selectedProduct).toHaveValue("");
    const jobsResponse = await page.request.get(
      `/api/v1/jobs?workflow_id=${workflowID}`,
    );
    expect(jobsResponse.ok()).toBeTruthy();
    const jobs = await jobsResponse.json();
    expect(jobs.total ?? 0).toBe(0);
    expect(jobs.items ?? []).toHaveLength(0);
  } finally {
    if (workflowID) {
      expect(
        (
          await (
            await page.request.get(`/api/v1/workflows/${workflowID}`)
          ).json()
        ).name,
      ).toBe(name);
      expect(
        (
          await page.request.post(`/api/v1/workflows/${workflowID}/archive`)
        ).ok(),
      ).toBeTruthy();
    }
    if (templateID) {
      expect(
        (
          await (
            await page.request.get(`/api/v1/templates/${templateID}`)
          ).json()
        ).name,
      ).toBe(`${name}-template`);
      expect(
        (
          await page.request.post(`/api/v1/templates/${templateID}/archive`)
        ).ok(),
      ).toBeTruthy();
    }
  }
});
