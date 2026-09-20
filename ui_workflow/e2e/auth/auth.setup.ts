import { expect, test as setup } from "@playwright/test";
import { config } from "../config";

setup("authenticate", async ({ page }) => {
  if (process.env.E2E_DEV_AUTH === "true") {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Media Workflow" })).toBeVisible();
    await page.context().storageState({ path: "e2e/auth/storage-state.json" });
    return;
  }

  const { email, password } = config.auth;

  if (!email || !password) {
    throw new Error(
      "Auth credentials must be set via profile YAML (auth.email / auth.password) or env vars (E2E_USER_EMAIL / E2E_USER_PASSWORD)",
    );
  }

  await page.goto("/login");

  await page.getByRole("textbox", { name: /email/i }).fill(email);
  await page.getByRole("textbox", { name: /^password \*$/i }).fill(password);
  await page.getByRole("button", { name: /sign in with password/i }).click();

  // Kratos returns to the requested application path with its session cookie.
  await page.waitForURL(/\/$/);
  await expect(page.getByRole("heading", { name: "Media Workflow" })).toBeVisible();

  // Save signed-in state
  await page.context().storageState({ path: "e2e/auth/storage-state.json" });
});
