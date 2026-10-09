import { expect, test } from "../fixtures/base";
import { readFileSync } from "node:fs";

const zh = JSON.parse(readFileSync("src/locales/zh-CN.json", "utf8")) as {
  materialsList: { kind: Record<string, string> };
};

// TestRail ID is unassigned. Only this test's own material is mutated.
test("uploads, creates, edits and deletes an isolated material through the UI", async ({
  page,
}) => {
  const name = `ui-material-${Date.now()}`;
  let id: string | undefined;
  let organizationId = "";
  const kindsResponse = await page.request.get(
    "/api/v1/asset-kinds?resource_type=MATERIAL&media_type=IMAGE&page_size=200",
  );
  expect(kindsResponse.ok()).toBe(true);
  const kinds = (await kindsResponse.json()) as {
    items: { name: string; resource_type: string; media_type: string }[];
  };
  const kind = kinds.items.find(
    (item) =>
      ["MATERIAL", "1"].includes(String(item.resource_type)) &&
      ["IMAGE", "1"].includes(String(item.media_type)),
  );
  expect(kind).toBeTruthy();
  try {
    await page.goto("/");
    await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
    await page.getByRole("tab", { name: /^素材$|^Materials$/ }).click();
    await page.getByRole("button", { name: /^新建$|^New$/ }).click();
    const create = page.getByRole("button", { name: /^创建$|^Create$/ });
    await expect(create).toBeDisabled();
    await page
      .getByRole("textbox", { name: /^业务范围$|^Scope$/ })
      .fill("ai_clothing");
    await page.getByRole("textbox", { name: /^名称$|^Name$/ }).fill(name);
    await page
      .getByRole("combobox", { name: /^资源类别$|^Material type$/ })
      .click();
    const kindLabel =
      (zh.materialsList.kind as Record<string, string>)[kind!.name] ||
      kind!.name;
    await page.getByRole("option", { name: kindLabel, exact: true }).click();
    const uploaded = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().includes("/library-objects:upload"),
    );
    await page.getByLabel(/^素材图片$|^Material image$/).setInputFiles({
      name: "ui-probe.png",
      mimeType: "image/png",
      buffer: Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aWQAAAABJRU5ErkJggg==",
        "base64",
      ),
    });
    expect((await uploaded).ok()).toBe(true);
    await expect(create).toBeEnabled();
    const created = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/v1\/materials(?:\?|$)/.test(response.url()),
    );
    await create.click();
    const response = await created;
    expect(response.ok()).toBe(true);
    const body = (await response.json()) as { id: string; object_path: string };
    id = body.id;
    organizationId =
      new URL(response.url()).searchParams.get("organization_id") || "";
    expect(organizationId).toBeTruthy();
    expect(body.object_path).toBeTruthy();
    const card = page
      .getByRole("article")
      .filter({ has: page.getByRole("heading", { name, exact: true }) });
    await expect(card).toBeVisible({ timeout: 60000 });
    await card
      .getByRole("button")
      .filter({ has: page.getByRole("heading", { name, exact: true }) })
      .click();
    await expect(
      page.getByRole("heading", { name, exact: true }),
    ).toBeVisible();
    await expect
      .poll(
        () =>
          page
            .locator(".media-app img")
            .first()
            .evaluate((image: HTMLImageElement) => image.naturalWidth),
        { timeout: 60000 },
      )
      .toBeGreaterThan(0);
    await page
      .getByRole("button", { name: /^编辑素材$|^Edit material$/ })
      .click();
    const description = "Isolated UI upload and editing probe";
    await page
      .getByRole("textbox", { name: /^描述$|^Description$/ })
      .fill(description);
    const updated = page.waitForResponse(
      (result) =>
        result.request().method() === "PUT" &&
        result.url().includes(`/materials/${id}`),
    );
    await page.getByRole("button", { name: /^保存$|^Save$/ }).click();
    expect((await updated).ok()).toBe(true);
    const saved = await page.request.get(
      `/api/v1/materials/${id}?organization_id=${organizationId}`,
    );
    expect(saved.ok()).toBe(true);
    expect((await saved.json()).description).toBe(description);
    await page
      .getByRole("button", { name: /^删除素材$|^Delete material$/ })
      .click();
    const removed = page.waitForResponse(
      (result) =>
        result.request().method() === "DELETE" &&
        result.url().includes(`/materials/${id}`),
    );
    await page
      .getByRole("dialog")
      .getByRole("button", { name: /^确认执行$|^Confirm$/ })
      .click();
    expect((await removed).ok()).toBe(true);
    id = undefined;
  } finally {
    if (id) {
      const owned = await page.request.get(
        `/api/v1/materials/${id}?organization_id=${organizationId}`,
      );
      if (owned.ok() && (await owned.json()).name === name) {
        expect(
          (
            await page.request.delete(
              `/api/v1/materials/${id}?organization_id=${organizationId}`,
            )
          ).ok(),
        ).toBe(true);
      }
    }
  }
});
