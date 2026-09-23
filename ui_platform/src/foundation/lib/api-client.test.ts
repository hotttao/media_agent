import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "@/foundation/lib/api-client";
import type { User } from "@/foundation/types/auth";

describe("ApiClient", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("logs in through a Kratos browser flow and resolves the business user", async () => {
    const user: User = {
      id: "u_1",
      username: "alice",
      role: "user",
      auth_source: "password",
      default_prompt_id: null,
      auto_evolution: false,
    };
    const action = `${window.location.origin}/self-service/login?flow=flow-1`;
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ui: {
              action,
              nodes: [{ attributes: { name: "csrf_token", value: "csrf-1" } }],
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(new Response("", { status: 200 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify(user), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.login("alice", "secret")).resolves.toEqual(user);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      expect.stringMatching(/^\/self-service\/login\/browser\?return_to=/),
      expect.objectContaining({ credentials: "include" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      action,
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: expect.any(URLSearchParams),
      }),
    );
    const loginBody = fetchMock.mock.calls[1]?.[1]?.body as URLSearchParams;
    expect(loginBody.get("identifier")).toBe("alice");
    expect(loginBody.get("password")).toBe("secret");
    expect(loginBody.get("csrf_token")).toBe("csrf-1");
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/auth/me",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("creates a same-origin Kratos logout URL", async () => {
    const logoutURL = `${window.location.origin}/self-service/logout?token=once`;
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ logout_url: logoutURL }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.createLogoutURL()).resolves.toBe(logoutURL);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/^\/self-service\/logout\/browser\?return_to=/),
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("rejects a cross-origin Kratos logout URL", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            logout_url: "https://evil.example/self-service/logout?token=once",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    await expect(api.createLogoutURL()).rejects.toThrow("invalid logout URL");
  });
});
