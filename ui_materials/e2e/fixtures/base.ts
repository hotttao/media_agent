import { test as base } from "@playwright/test";
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
    page.on("response", async (res) => {
      if (res.status() >= 400 && !isApiErrorMuted()) {
        const body = await res.text().catch(() => "");
        console.log(`[API ${res.status()}] ${res.url()}\n${body}`);
      }
    });
    await use(page);
  },
});

export { expect } from "@playwright/test";
