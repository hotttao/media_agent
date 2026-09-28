import { expect, test } from "@playwright/test";
import { config } from "../config";

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
      page.getByRole("heading", { name: /^(Create your account|创建账户)$/ }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      page.getByRole("textbox", { name: /email|邮箱/i }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: /sign up/i })).toBeVisible();
    await expect(
      page.getByRole("link", { name: /^(Sign in|登录)$/ }),
    ).toHaveAttribute("href", "/login");
    await context.close();
  });

  test("authenticated session resolves the stable business user", {
    tag: "@C900003",
  }, async ({ page }) => {
    const response = await page.request.get("/api/auth/me");

    await expect(response).toBeOK();
    const user = await response.json();
    expect(user.username).toBe(config.auth.email);
    expect(user.id).toEqual(expect.any(String));
    expect(user.id.length).toBeGreaterThan(0);
    expect(["user", "admin"]).toContain(user.role);
    const repeated = await page.request.get("/api/auth/me");
    await expect(repeated).toBeOK();
    expect((await repeated.json()).id).toBe(user.id);
  });

  test("sign out invalidates the Kratos session", { tag: "@C900004" }, async ({
    browser,
  }) => {
    test.setTimeout(60_000);
    // Logout must not invalidate the setup Session reused by other tests.
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto("/login");
    await page
      .getByRole("textbox", { name: /email|邮箱|用户名/i })
      .fill(config.auth.email);
    await page
      .getByRole("textbox", { name: /password|密码/i })
      .fill(config.auth.password);
    await page
      .getByRole("button", { name: /sign in with password|登录/i })
      .click();
    await page.waitForURL(/\/$/);
    await expect(
      page.getByRole("heading", { name: /Media (Agent|Workflow)/ }),
    ).toBeVisible({ timeout: 15_000 });
    await page
      .getByRole("button", { name: /sign out|退出登录/i })
      .click({ timeout: 30_000 });
    await page.waitForURL(/\/login(?:\?|$)/);

    expect((await page.request.get("/sessions/whoami")).status()).toBe(401);
    expect((await page.request.get("/api/auth/me")).status()).toBe(401);
    await context.close();
  });

  test("password recovery entry is reachable without a session", {
    tag: "@C900005",
  }, async ({ page }) => {
    await page.context().clearCookies();
    await page.goto("/login");
    await page.getByRole("link", { name: /forgot password|忘记密码/i }).click();

    await expect(
      page.getByRole("heading", { name: /recover|恢复|找回/i }),
    ).toBeVisible();
    await expect(page.getByRole("textbox", { name: /email/i })).toBeVisible();
    await expect(page.locator('[data-testid="app-express"]')).toBeVisible();
  });
});
