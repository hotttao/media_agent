import { expect, test } from "@playwright/test";

test.describe("Ory browser authentication", () => {
  test("registration renders the Kratos profile step", {
    tag: "@C900006",
  }, async ({ browser }) => {
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();

    await page.goto("/registration");
    await page.waitForURL(/\/registration\?flow=/);

    await expect(
      page.getByRole("heading", { name: "Create your account" }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("textbox", { name: /email/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /sign up/i })).toBeVisible();
    await expect(page.getByRole("link", { name: "Sign in" })).toHaveAttribute(
      "href",
      "/login",
    );
    await context.close();
  });

  test("authenticated session resolves the stable business user", {
    tag: "@C900003",
  }, async ({ page }) => {
    const response = await page.request.get("/api/auth/me");

    await expect(response).toBeOK();
    expect(await response.json()).toMatchObject({
      username: "admin@media-agent.local",
      role: "admin",
    });
  });

  test("sign out invalidates the Kratos session", { tag: "@C900004" }, async ({
    page,
  }) => {
    await page.goto("/");
    await page.getByRole("button", { name: /sign out/i }).click();
    await page.waitForURL(/\/login(?:\?|$)/);

    expect((await page.request.get("/sessions/whoami")).status()).toBe(401);
    expect((await page.request.get("/api/auth/me")).status()).toBe(401);
  });

  test("password recovery entry is reachable without a session", {
    tag: "@C900005",
  }, async ({ page }) => {
    await page.context().clearCookies();
    await page.goto("/recovery");

    await expect(page.getByRole("heading", { name: /recover/i })).toBeVisible();
    await expect(page.getByRole("textbox", { name: /email/i })).toBeVisible();
  });
});
