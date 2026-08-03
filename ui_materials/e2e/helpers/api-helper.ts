import type { Page } from "@playwright/test";

/** Module-level flag — when true the page response listener should skip logging */
let _muteApiErrors = false;

/** Whether API error logging is currently muted (used by the page fixture) */
export function isApiErrorMuted(): boolean {
  return _muteApiErrors;
}

export function muteApiErrors(): void {
  _muteApiErrors = true;
}

export function unmuteApiErrors(): void {
  _muteApiErrors = false;
}

/**
 * Generic API helper. Calls the project's backend via `page.evaluate` so the
 * request inherits the browser's auth session (cookies / localStorage).
 *
 * Extend with `createResource` / `deleteResource` / etc. helpers as the
 * project's API surface grows. See `contributing/e2e.md` > ApiHelper for the
 * contract this class is expected to satisfy.
 */
export class ApiHelper {
  constructor(readonly page: Page) {}

  /** Ensure the page is on the app origin so localStorage is accessible. */
  private async ensureOnAppOrigin(): Promise<void> {
    const url = this.page.url();
    if (url === "about:blank" || url === "") {
      await this.page.goto("/");
      await this.page.waitForLoadState("networkidle");
    }
  }

  /**
   * Low-level fetch wrapper. Pulls the auth token out of localStorage and
   * forwards the request through the browser so cookies/session are honored.
   *
   * Override or wrap this in subclasses to inject per-resource headers.
   */
  protected async api<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    await this.ensureOnAppOrigin();
    return this.page.evaluate(
      async ({ method, path, body }) => {
        // Pull every localStorage entry that looks like an auth token.
        // Adjust the predicate to match your auth backend.
        const token = Object.entries(localStorage)
          .map(([k, v]) => {
            const m = /auth[-_]?token/i.exec(k);
            return m ? v : null;
          })
          .find(Boolean);

        const res = await fetch(`/api/v1${path}`, {
          method,
          headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: body ? JSON.stringify(body) : undefined,
        });

        if (!res.ok) {
          throw new Error(
            `ApiHelper.${method} ${path} failed: ${res.status} ${res.statusText}`,
          );
        }
        return res.json() as Promise<T>;
      },
      { method, path, body },
    );
  }
}
