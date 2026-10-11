import { expect, test as setup } from "@playwright/test";
import { config } from "../config";
import { openStandaloneApp } from "../fixtures/base";

setup("authenticate", async ({ page }) => {
  // Optional shared-workstation diagnostic budget; normal authentication
  // acceptance stays at 30 seconds. This never retries a login/submission.
  setup.setTimeout(Number(process.env.E2E_AUTH_TIMEOUT_MS || 30_000));
  if (process.env.E2E_DEV_AUTH === "true") {
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: /Media (Agent|Workflow)/ }),
    ).toBeVisible();
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

  await page.getByLabel(/^(email|邮箱|用户名)/i).fill(email);
  await page.getByLabel(/^(password|密码)/i).fill(password);
  await page
    .getByRole("button", { name: /sign in with password|登录/i })
    .click();

  // Kratos returns to the requested application path with its session cookie.
  await page.waitForURL(/\/$/);
  // Use the same authenticated bootstrap gate as the App tests. Its optional
  // diagnostic reload is restricted to an observed ERR_NETWORK_CHANGED,
  // before any business interaction, and never retries a submission.
  await openStandaloneApp(page);

  // Save signed-in state
  await page.context().storageState({ path: "e2e/auth/storage-state.json" });
});
