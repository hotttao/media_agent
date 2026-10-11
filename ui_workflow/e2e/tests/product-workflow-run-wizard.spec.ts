import { expect, openStandaloneApp, test } from "../fixtures/base";

// Borrow existing data read-only. No templates, definitions, jobs or workers.
test("product opens a prefilled canvas run and cancellation returns to that product", async ({
  page,
}) => {
  test.setTimeout(120_000);
  await openStandaloneApp(page);
  const summaries = await (
    await page.request.get("/api/v1/workflows?page_size=200")
  ).json();
  let workflow;
  for (const item of summaries.items) {
    const response = await page.request.get(`/api/v1/workflows/${item.id}`);
    if (!response.ok()) continue;
    const candidate = await response.json();
    if (
      candidate.status === 2 && candidate.publication_status === "registered" &&
      candidate.allowed_actions?.includes("run") &&
      candidate.input_schema?.entries?.some(
        (entry: { resource_type: string }) => entry.resource_type === "PRODUCT",
      )
    ) {
      workflow = candidate;
      break;
    }
  }
  if (!workflow)
    throw new Error("Requires an existing runnable PRODUCT workflow");
  const workspacesResponse = await page.request.get("/api/v1/workspaces");
  expect(workspacesResponse.ok()).toBeTruthy();
  const workspaces = await workspacesResponse.json();
  const organizationId =
    workflow.organization_id || workspaces.workspaces[0].organization_id;
  const productsResponse = await page.request.get(
    `/api/v1/products?organization_id=${encodeURIComponent(organizationId)}&scope=${encodeURIComponent(workflow.scope)}&status=active&page_size=1`,
  );
  expect(productsResponse.ok()).toBeTruthy();
  const products = await productsResponse.json();
  const product = products.items[0];
  expect(product).toBeTruthy();
  let submissions = 0;
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      /\/api\/v1\/(jobs|workflows|templates)(?:\/|\?|$)/.test(request.url())
    )
      submissions++;
  });
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
    .getByRole("textbox", {
      name: /^搜索可用工作流$|^Search available workflows$/,
    })
    .fill(workflow.name);
  await dialog
    .getByRole("button", {
      name: new RegExp(workflow.name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
    })
    .click();
  await dialog
    .getByRole("button", {
      name: /^带入商品并准备运行$|^Prepare run with this product$/,
    })
    .click();
  const productEntry = workflow.input_schema.entries.find(
    (entry: { resource_type: string }) => entry.resource_type === "PRODUCT",
  );
  const field = page.getByRole("region", {
    name: productEntry.name,
    exact: true,
  });
  await field.getByRole("textbox").fill(product.name);
  await expect(
    field.getByRole("button", { name: product.name, exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByTestId("workflow-canvas")).toBeVisible();
  await page.getByRole("button", { name: /^取消$|^Cancel$/ }).click();
  await expect(
    page.getByRole("heading", { name: product.name, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: /^商品$|^Products$/ }),
  ).toHaveAttribute("data-state", "active");
  expect(submissions).toBe(0);
});
