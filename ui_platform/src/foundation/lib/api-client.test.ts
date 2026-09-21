import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "@/foundation/lib/api-client";
import type { User } from "@/foundation/types/auth";

describe("ApiClient", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("unwraps the user returned by media_auth", async () => {
    const user: User = {
      id: "u_1",
      username: "alice",
      role: "user",
      auth_source: "password",
      default_prompt_id: null,
      auto_evolution: false,
    };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ user }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.login("alice", "secret")).resolves.toEqual(user);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/login",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({ username: "alice", password: "secret" }),
      }),
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
