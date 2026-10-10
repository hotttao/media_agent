import { expect, test as setup } from "@playwright/test";
import { config } from "../config";

setup("authenticate", async ({ page }) => {
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
  await expect(
    page.getByRole("heading", { name: /Media (Agent|Workflow)/ }),
  ).toBeVisible({
    timeout: Number(process.env.E2E_APP_READY_TIMEOUT_MS || 15_000),
  });

  // Save signed-in state
  await page.context().storageState({ path: "e2e/auth/storage-state.json" });
});
