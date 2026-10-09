import { expect, test } from "../fixtures/base";

const resources = [
  { key: "material", name: /^素材$|^Materials$/ },
  { key: "virtual-ip", name: /^虚拟形象$|^Virtual IPs$/ },
  { key: "product", name: /^商品$|^Products$/ },
  { key: "generated", name: /^生成素材$|^生成产物$|^Generated$/ },
];
for (const resource of resources) {
  test(`${resource.key} details fit a narrow app without writes`, async ({
    page,
  }) => {
    await page.goto("/");
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.getByRole("button", { name: /^素材$|^Materials$/ }).click();
    await page.getByRole("tab", { name: resource.name }).click();
    const card = page.locator(".media-app article").first();
    await expect(card).toBeVisible({ timeout: 60000 });
    await card
      .getByRole("button")
      .filter({ has: page.getByRole("heading") })
      .first()
      .click();
    await expect(page.locator(".media-app h1")).toBeVisible();
    await expect(page.locator(".media-app pre")).toHaveCount(0);
    const image = page.locator(".media-app img").first();
    if (await image.count()) {
      await expect(image).toBeVisible();
      await expect
        .poll(
          () => image.evaluate((node: HTMLImageElement) => node.naturalWidth),
          { timeout: 30000 },
        )
        .toBeGreaterThan(0);
    }
    await page.screenshot({
      path: `/tmp/media-agent-v12-${resource.key}-detail-live.png`,
    });
    await page.setViewportSize({ width: 480, height: 900 });
    await expect
      .poll(() =>
        page
          .locator(".media-app")
          .evaluate((node) => node.scrollWidth <= node.clientWidth + 2),
      )
      .toBeTruthy();
    await page.screenshot({
      path: `/tmp/media-agent-v12-${resource.key}-detail-narrow-live.png`,
    });
    await page.setViewportSize({ width: 1440, height: 900 });
    // Existing data is borrowed read-only: no save/delete/archive/upload commands.
  });
}
