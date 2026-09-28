import { defineConfig, devices } from "@playwright/test";
import { existsSync } from "node:fs";
import { loadEnvFile } from "node:process";

if (existsSync("e2e/.env")) loadEnvFile("e2e/.env");

export default defineConfig({
  testDir: "./e2e/tests",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [["list"]],
  outputDir: "e2e/test-results",

  use: {
    baseURL: process.env.BASE_URL || "http://localhost:5173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    // Allow explicitly slower diagnostics on a shared/loaded workstation;
    // the normal regression budget remains 10 seconds.
    actionTimeout: Number(process.env.E2E_ACTION_TIMEOUT_MS || 10_000),
    ...(process.env.E2E_CHROMIUM_EXECUTABLE
      ? { launchOptions: { executablePath: process.env.E2E_CHROMIUM_EXECUTABLE } }
      : {}),
  },

  projects: [
    {
      name: "setup",
      testDir: "./e2e/auth",
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        storageState: "e2e/auth/storage-state.json",
      },
      dependencies: ["setup"],
    },
  ],
});
