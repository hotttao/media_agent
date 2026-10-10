import {
  expect,
  type Page,
  type Request,
  test as base,
} from "@playwright/test";
import { isApiErrorMuted } from "../helpers/api-helper";

/**
 * Extend this with resource-specific fixtures as the project grows. The
 * recommended pattern from `contributing/e2e.md` is:
 *
 *   type ResourceFixtures = {
 *     <resource>: ResourcePage;
 *     apiHelper: ApiHelper;
 *   };
 *
 *   export const test = base.extend<ResourceFixtures>({
 *     page: async ({ page }, use) => {
 *       page.on("response", async (res) => {
 *         if (res.status() >= 400 && !isApiErrorMuted()) {
 *           const body = await res.text().catch(() => "");
 *           console.log(`[API ${res.status()}] ${res.url()}\n${body}`);
 *         }
 *       });
 *       await use(page);
 *     },
 *     // <resource>: async ({ page }, use) => {
 *     //   await use(new ResourcePage(page, { routeName: "<resource>" }));
 *     // },
 *     apiHelper: async ({ page }, use) => {
 *       await use(new ApiHelper(page));
 *     },
 *   });
 */
export const test = base.extend({
  page: async ({ page }, use) => {
    page.on("response", (res) => {
      if (res.status() >= 400 && !isApiErrorMuted()) {
        // Never log payloads or URL queries: both may contain private inputs
        // or storage signatures. Trace archives remain local diagnostic data.
        console.log(`[API ${res.status()}] ${new URL(res.url()).pathname}`);
      }
    });
    page.on("requestfailed", (request) => {
      console.log(
        `[requestfailed] ${new URL(request.url()).pathname} ${request.failure()?.errorText.replace(/https?:\/\/\S+/g, "[URL]")}`,
      );
    });
    await use(page);
  },
});

export { expect } from "@playwright/test";

/** Wait for authenticated Shell bootstrap before exercising an App. */
export async function openStandaloneApp(page: Page) {
  let networkChanged = false;
  const observe = (request: Request) => {
    if (request.failure()?.errorText === "net::ERR_NETWORK_CHANGED")
      networkChanged = true;
  };
  const ready = () =>
    expect(
      page.getByRole("heading", { name: /Media (Agent|Workflow)/ }),
    ).toBeVisible({
      timeout: Number(process.env.E2E_APP_READY_TIMEOUT_MS || 15_000),
    });
  page.on("requestfailed", observe);
  try {
    await page.goto("/");
    try {
      await ready();
    } catch (error) {
      if (process.env.E2E_RETRY_NETWORK_BOOTSTRAP !== "true" || !networkChanged)
        throw error;
      // Explicit infrastructure diagnostic only: one document reload before
      // App interaction. Never retry a test or any business command.
      console.log(
        "[bootstrap] observed ERR_NETWORK_CHANGED; reloading once before App interaction",
      );
      await page.reload();
      await ready();
    }
  } finally {
    page.off("requestfailed", observe);
  }
}
