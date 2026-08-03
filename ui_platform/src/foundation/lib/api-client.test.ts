import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "@/foundation/lib/api-client";
import type { User } from "@/foundation/types/auth";

describe("ApiClient login", () => {
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
});
