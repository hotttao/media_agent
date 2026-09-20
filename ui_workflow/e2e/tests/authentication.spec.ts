import { expect, test } from "@playwright/test";

test.describe("Ory browser authentication", () => {
  test(
    "authenticated session resolves the stable business user",
    { tag: "@C900003" },
    async ({ page }) => {
      const response = await page.request.get("/api/auth/me");

      await expect(response).toBeOK();
      expect(await response.json()).toMatchObject({
        username: "admin@media-agent.local",
        role: "admin",
      });
    },
  );

  test(
    "sign out invalidates the Kratos session",
    { tag: "@C900004" },
    async ({ page }) => {
      await page.goto("/");
      await page.getByRole("button", { name: /sign out/i }).click();
      await page.waitForURL(/\/login(?:\?|$)/);

      expect((await page.request.get("/sessions/whoami")).status()).toBe(401);
      expect((await page.request.get("/api/auth/me")).status()).toBe(401);
    },
  );

  test(
    "password recovery entry is reachable without a session",
    { tag: "@C900005" },
    async ({ page }) => {
      await page.context().clearCookies();
      await page.goto("/recovery");

      await expect(page.getByRole("heading", { name: /recover/i })).toBeVisible();
      await expect(page.getByRole("textbox", { name: /email/i })).toBeVisible();
    },
  );
});
