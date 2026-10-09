import { expect, test } from "../fixtures/base";

test("template mapping editing keeps slot references and unsaved draft across apps", async ({
  page,
}) => {
  await page.goto("/");
  await page.setViewportSize({ width: 1440, height: 900 });
  const response = await page.request.get("/api/v1/templates?page_size=100");
  expect(response.ok()).toBeTruthy();
  const { items } = await response.json();
  let template;
  for (const item of items) {
    const detail = await page.request.get(
      `/api/v1/templates/${encodeURIComponent(item.id)}`,
    );
    if (!detail.ok()) continue;
    const candidate = await detail.json();
    if (
      !["ARCHIVED", "TPL_ARCHIVED", "3"].includes(String(candidate.status)) &&
      candidate.inputs?.length >= 2 &&
      candidate.invocation_mapping?.images?.items?.length >= 2
    ) {
      template = candidate;
      break;
    }
  }
  expect(
    template,
    "requires a readable template with multiple image references",
  ).toBeTruthy();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await page.getByRole("tab", { name: /^模板$|^Templates$/ }).click();
  await page
    .getByRole("textbox", { name: /^搜索名称$|^Search by name$/ })
    .fill(template.name);
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: template.name, exact: true }),
    })
    .click();
  await expect(
    page.getByRole("region", {
      name: /^能力调用参数$|^Capability input parameters$/,
    }),
  ).toBeVisible();
  await page.screenshot({
    path: "/tmp/media-agent-v12-template-detail-live.png",
  });
  await page.getByRole("button", { name: /^编辑$|^Edit$/ }).click();
  const prompt = page.getByRole("textbox", {
    name: /^提示词模板$|^Prompt template$/,
  });
  // HTML textarea values normalize CRLF; keep the API snapshot unchanged.
  const visiblePrompt = template.prompt_body.replace(/\r\n?/g, "\n");
  await expect(prompt).toHaveValue(visiblePrompt);
  const changed = `${visiblePrompt}\nUI draft test`;
  await prompt.fill(changed);
  await page
    .getByRole("button", { name: /^上移 images\[1\]$|^Move images\[1\] up$/ })
    .click();
  const firstReference = page.getByRole("combobox", {
    name: /^images\[0\] 的资源槽位$|^Resource slot for images\[0\]$/,
  });
  await expect(firstReference).toHaveValue(
    template.invocation_mapping.images.items[1].slot,
  );
  await page.getByRole("button", { name: /^检查槽位$|^Check slots$/ }).click();
  await expect(
    page.getByRole("region", { name: /^槽位检查结果$|^Slot check results$/ }),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: /^按槽位生成图片调用映射$|^Build image invocation from slots$/,
    })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(/覆盖|overwritten/);
  await dialog.getByRole("button", { name: /^取消$|^Cancel$/ }).click();
  await expect(firstReference).toHaveValue(
    template.invocation_mapping.images.items[1].slot,
  );
  await page.screenshot({
    path: "/tmp/media-agent-v12-template-editor-live.png",
  });
  await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
  await page.getByRole("button", { name: /^工作流$|^Workflow$/ }).click();
  await expect(prompt).toHaveValue(changed);
  await expect(firstReference).toHaveValue(
    template.invocation_mapping.images.items[1].slot,
  );
  await page.setViewportSize({ width: 480, height: 900 });
  await firstReference.scrollIntoViewIfNeeded();
  await expect(firstReference).toBeVisible();
  await page.screenshot({
    path: "/tmp/media-agent-v12-template-narrow-live.png",
  });
  // Do not save or mutate this borrowed record; only the instance draft changed.
});
