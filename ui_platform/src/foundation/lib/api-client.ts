/**
 * API client for media_auth backend.
 *
 * Cookie-based auth: all requests use `credentials: 'include'`.
 * 401 on non-auth endpoints auto-redirects to /login.
 *
 * Adapted from agent-platform/web/src/lib/api/client.ts, trimmed to auth endpoints only.
 */
import type {
  Identity,
  OAuthAuthorizeInfo,
  OAuthAuthorizeResponse,
  ServiceTokenRecord,
  User,
  UsersPage,
  WeComAuthorizeResponse,
  WeComEnabledResponse,
} from "@/foundation/types/auth";

interface KratosLoginFlow {
  ui: {
    action: string;
    nodes: Array<{
      attributes: { name?: string; value?: unknown };
      messages?: Array<{ text?: string }>;
    }>;
    messages?: Array<{ text?: string }>;
  };
}

function kratosErrorMessage(body: Record<string, unknown>): string | undefined {
  const ui = body.ui as KratosLoginFlow["ui"] | undefined;
  return [
    ...(ui?.messages ?? []),
    ...(ui?.nodes.flatMap((n) => n.messages ?? []) ?? []),
  ]
    .map((message) => message.text)
    .find((message): message is string => Boolean(message));
}

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

class ApiClient {
  private baseUrl = "/api";

  private async request<T>(
    endpoint: string,
    options?: RequestInit,
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const response = await fetch(url, {
      ...options,
      headers: { "Content-Type": "application/json", ...options?.headers },
      credentials: "include",
    });

    if (!response.ok) {
      if (
        response.status === 401 &&
        !endpoint.includes("/auth/") &&
        window.location.pathname !== "/login" &&
        window.location.pathname !== "/register"
      ) {
        window.location.href = "/login";
        throw new Error("Session expired");
      }
      const body: Record<string, unknown> = await response
        .json()
        .catch(() => ({ error: "Request failed" }));
      const message =
        (typeof body.error === "string" && body.error) || "Request failed";
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

  // ── Auth ──────────────────────────────────────────────────────────

  async getMe(): Promise<User> {
    return this.request<User>("/auth/me");
  }

  async login(username: string, password: string): Promise<User> {
    const returnTo = new URL("/", window.location.origin).toString();
    const flowResponse = await fetch(
      `/self-service/login/browser?return_to=${encodeURIComponent(returnTo)}`,
      {
        credentials: "include",
        headers: { Accept: "application/json" },
      },
    );
    if (!flowResponse.ok) {
      throw new ApiClientError(
        "Unable to create login flow",
        flowResponse.status,
        {},
      );
    }

    const flow = (await flowResponse.json()) as KratosLoginFlow;
    const action = new URL(flow.ui.action, window.location.origin);
    if (
      action.origin !== window.location.origin ||
      action.pathname !== "/self-service/login"
    ) {
      throw new Error("Kratos returned an invalid login action");
    }
    const csrfToken = flow.ui.nodes.find(
      (node) => node.attributes.name === "csrf_token",
    )?.attributes.value;
    if (typeof csrfToken !== "string" || csrfToken === "") {
      throw new Error("Kratos login flow is missing a CSRF token");
    }

    const form = new URLSearchParams({
      method: "password",
      identifier: username,
      password,
      csrf_token: csrfToken,
    });
    const loginResponse = await fetch(action.toString(), {
      method: "POST",
      body: form,
      credentials: "include",
      headers: { Accept: "application/json" },
    });
    if (!loginResponse.ok) {
      const body = (await loginResponse.json().catch(() => ({}))) as Record<
        string,
        unknown
      >;
      throw new ApiClientError(
        kratosErrorMessage(body) ?? "Invalid email or password",
        loginResponse.status,
        body,
      );
    }
    return this.getMe();
  }

  async register(data: {
    username: string;
    password: string;
    display_name?: string;
    email?: string;
  }): Promise<User> {
    return this.request<{ user: User }>("/auth/register", {
      method: "POST",
      body: JSON.stringify(data),
    }).then((r) => r.user);
  }

  async createLogoutURL(): Promise<string> {
    const returnTo = new URL("/login", window.location.origin).toString();
    const response = await fetch(
      `/self-service/logout/browser?return_to=${encodeURIComponent(returnTo)}`,
      { credentials: "include", headers: { Accept: "application/json" } },
    );
    if (!response.ok) throw new Error("Unable to create logout flow");
    const body = (await response.json()) as { logout_url?: string };
    const logoutURL = new URL(body.logout_url ?? "", window.location.origin);
    if (
      logoutURL.origin !== window.location.origin ||
      logoutURL.pathname !== "/self-service/logout"
    ) {
      throw new Error("Kratos returned an invalid logout URL");
    }
    return logoutURL.toString();
  }

  async changeMyPassword(
    currentPassword: string,
    newPassword: string,
  ): Promise<void> {
    await this.request("/auth/me/password", {
      method: "PUT",
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    });
  }

  async patchMe(patch: { auto_evolution?: boolean }): Promise<void> {
    await this.request("/auth/me", {
      method: "PATCH",
      body: JSON.stringify(patch),
    });
  }

  // ── WeCom ─────────────────────────────────────────────────────────

  async getWeComEnabled(): Promise<WeComEnabledResponse> {
    return this.request<WeComEnabledResponse>("/auth/wecom/enabled");
  }

  async getWeComAuthorizeUrl(
    mode: "login" | "bind",
  ): Promise<WeComAuthorizeResponse> {
    return this.request<WeComAuthorizeResponse>(
      `/auth/wecom/authorize?mode=${mode}`,
    );
  }

  async getIdentities(): Promise<{ identities: Identity[] }> {
    return this.request("/auth/identities");
  }

  async unbindIdentity(provider: string): Promise<void> {
    await this.request(`/auth/identities/${provider}`, { method: "DELETE" });
  }

  // ── OAuth ─────────────────────────────────────────────────────────

  async getOAuthAuthorizeInfo(params: {
    client_id: string;
    redirect_uri: string;
    response_type: string;
    scope?: string;
  }): Promise<OAuthAuthorizeInfo> {
    const qs = new URLSearchParams({
      client_id: params.client_id,
      redirect_uri: params.redirect_uri,
      response_type: params.response_type,
      ...(params.scope ? { scope: params.scope } : {}),
    });
    return this.request<OAuthAuthorizeInfo>(`/oauth/authorize?${qs}`);
  }

  async approveOAuthAuthorization(data: {
    client_id: string;
    redirect_uri: string;
    response_type: string;
    state?: string;
    code_challenge?: string;
    code_challenge_method?: string;
    scope?: string;
  }): Promise<OAuthAuthorizeResponse> {
    return this.request<OAuthAuthorizeResponse>("/oauth/authorize", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // ── Service Tokens ────────────────────────────────────────────────

  async listServiceTokens(): Promise<{ tokens: ServiceTokenRecord[] }> {
    return this.request("/service-tokens");
  }

  async createServiceToken(
    name: string,
  ): Promise<{ id: string; name: string; token: string; created_at: string }> {
    return this.request("/service-tokens", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
  }

  async revokeServiceToken(id: string): Promise<void> {
    await this.request(`/service-tokens/${id}`, { method: "DELETE" });
  }

  // ── User Management (admin) ───────────────────────────────────────

  async listUsers(params: {
    q?: string;
    sort?: string;
    order?: string;
    page?: number;
    page_size?: number;
  }): Promise<UsersPage> {
    const qs = new URLSearchParams();
    if (params.q) qs.set("q", params.q);
    if (params.sort) qs.set("sort", params.sort);
    if (params.order) qs.set("order", params.order);
    if (params.page) qs.set("page", String(params.page));
    if (params.page_size) qs.set("page_size", String(params.page_size));
    return this.request<UsersPage>(`/users?${qs}`);
  }

  async createUser(data: {
    username: string;
    display_name: string;
    password: string;
    email?: string;
    role?: string;
  }): Promise<void> {
    await this.request("/users", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteUser(id: string): Promise<void> {
    await this.request(`/users/${id}`, { method: "DELETE" });
  }

  async changeUserPassword(id: string, password: string): Promise<void> {
    await this.request(`/users/${id}/password`, {
      method: "PUT",
      body: JSON.stringify({ password }),
    });
  }
}

export const api = new ApiClient();
