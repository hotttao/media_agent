import { expect, test as setup } from "@playwright/test";
import { config } from "../config";

setup("authenticate", async ({ page }) => {
  const { username, password } = config.auth;

  if (!username || !password) {
    throw new Error(
      "Auth credentials must be set via profile YAML (auth.username / auth.password) or env vars (E2E_USER_USERNAME / E2E_USER_PASSWORD)",
    );
  }

  await page.goto("/login");

  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: /sign in/i }).click();

  // Wait for redirect to dashboard after successful login
  await page.waitForURL("**/");
  await expect(page).toHaveURL(/\/$/);

  // Save signed-in state
  await page.context().storageState({ path: "e2e/auth/storage-state.json" });
});
