import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e/product",
  workers: 1,
  fullyParallel: false,
  reporter: "list",
  outputDir: "e2e/test-results/agent-product",
  webServer: {
    command:
      "node_modules/.bin/vite build --outDir /tmp/media-agent-product-e2e && node_modules/.bin/vite preview --outDir /tmp/media-agent-product-e2e --host 127.0.0.1 --port 15178 --strictPort",
    url: "http://127.0.0.1:15178",
    reuseExistingServer: false,
  },
  use: {
    baseURL: "http://127.0.0.1:15178",
    channel: "chrome",
    viewport: { width: 1440, height: 1000 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
