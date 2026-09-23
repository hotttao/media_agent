/**
 * Generic HTTP client. Cookie-based auth: all requests use
 * `credentials: "include"` (mw_token cookie set by materials/workflow auth
 * middleware after OAuth). On 401, calls the configured onUnauthorized
 * handler (standalone redirects to /api/v1/auth/login; plugin leaves it to
 * the host).
 *
 * Resource-specific endpoints live in domains/<name>/lib/api.ts - this file
 * only holds the generic request + auth/me + logout.
 *
 * Adapted from ui_platform/src/foundation/lib/api-client.ts (trimmed: removed
 * media_auth-specific auth/wecom/oauth/users/service-tokens endpoints, moved
 * 401 redirect into an injectable handler, base path /api/v1).
 */
import { API_BASE } from "@/foundation/lib/constants";
import type { User } from "@/foundation/types/auth";

export class ApiClientError extends Error {
  readonly status: number;
  readonly body: Record<string, unknown>;

  constructor(message: string, status: number, body: Record<string, unknown>) {
    super(message);
    this.name = "ApiClientError";
    this.status = status;
    this.body = body;
  }
}

type UnauthorizedHandler = () => void;

let onUnauthorized: UnauthorizedHandler = () => {};

/** Configure the 401 handler. Called once from the standalone entry. */
export function configureApi(opts: {
  onUnauthorized?: UnauthorizedHandler;
}): void {
  if (opts.onUnauthorized) onUnauthorized = opts.onUnauthorized;
}

class ApiClient {
  private baseUrl = API_BASE;
  private organizationId = "";

  setOrganizationId(organizationId: string): void {
    this.organizationId = organizationId;
  }

  private scopedPath(path: string): string {
    const organizationScopedResources = [
      "/materials",
      "/products",
      "/virtual-ips",
      "/generated-assets",
      "/library-objects",
    ];
    if (
      !this.organizationId ||
      !organizationScopedResources.some(
        (prefix) =>
          path === prefix ||
          path.startsWith(`${prefix}/`) ||
          path.startsWith(`${prefix}?`),
      )
    ) {
      return path;
    }
    const [pathname, query = ""] = path.split("?", 2);
    const searchParams = new URLSearchParams(query);
    if (!searchParams.has("organization_id")) {
      searchParams.set("organization_id", this.organizationId);
    }
    return `${pathname}?${searchParams.toString()}`;
  }

  async request<T>(path: string, options?: RequestInit): Promise<T> {
    const url = `${this.baseUrl}${this.scopedPath(path)}`;
    const response = await fetch(url, {
      ...options,
      headers: { "Content-Type": "application/json", ...options?.headers },
      credentials: "include",
    });

    if (response.status === 401) {
      onUnauthorized();
      throw new ApiClientError("Unauthorized", 401, {});
    }

    if (!response.ok) {
      const body: Record<string, unknown> = await response
        .json()
        .catch(() => ({ error: "Request failed" }));
      const message =
        (typeof body.error === "string" && body.error) ||
        `Request failed: ${response.status}`;
      throw new ApiClientError(message, response.status, body);
    }

    if (
      response.status === 204 ||
      response.headers.get("Content-Length") === "0"
    ) {
      return undefined as T;
    }
    return response.json();
  }

  // ── Auth (shared by materials/workflow auth middleware) ──────────────

  async getMe(): Promise<User> {
    return this.requestAuth<User>("/me");
  }

  /** Auth endpoints are mounted at /api/auth, outside the versioned resource API. */
  private async requestAuth<T>(
    path: string,
    options?: RequestInit,
  ): Promise<T> {
    const response = await fetch(`/api/auth${path}`, {
      ...options,
      headers: { "Content-Type": "application/json", ...options?.headers },
      credentials: "include",
    });
    if (response.status === 401) {
      onUnauthorized();
      throw new ApiClientError("Unauthorized", 401, {});
    }
    if (!response.ok) {
      const body: Record<string, unknown> = await response
        .json()
        .catch(() => ({ error: "Request failed" }));
      throw new ApiClientError(
        (typeof body.error === "string" && body.error) ||
          `Request failed: ${response.status}`,
        response.status,
        body,
      );
    }
    if (
      response.status === 204 ||
      response.headers.get("Content-Length") === "0"
    )
      return undefined as T;
    return response.json();
  }
}

export const api = new ApiClient();
