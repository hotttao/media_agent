import { describe, expect, it } from "vitest";
import { browserFlowURL, createBrowserLogoutURL } from "./ory-browser";

describe("browserFlowURL", () => {
  it("builds a same-origin Kratos browser flow return URL", () => {
    expect(browserFlowURL("login", "/materials?tab=all", "https://media.test")).toBe(
      "/self-service/login/browser?return_to=https%3A%2F%2Fmedia.test%2Fmaterials%3Ftab%3Dall",
    );
  });

  it("rejects protocol-relative return paths", () => {
    expect(browserFlowURL("logout", "//evil.test", "https://media.test")).toBe(
      "/self-service/logout/browser?return_to=https%3A%2F%2Fmedia.test%2F",
    );
  });

  it("accepts only the same-origin Kratos logout execution URL", async () => {
    const request = async () =>
      new Response(
        JSON.stringify({
          logout_url: "https://media.test/self-service/logout?token=once",
        }),
        { status: 200 },
      );

    await expect(
      createBrowserLogoutURL("/login", "https://media.test", request),
    ).resolves.toBe("https://media.test/self-service/logout?token=once");
  });

  it("rejects a cross-origin logout execution URL", async () => {
    const request = async () =>
      new Response(
        JSON.stringify({
          logout_url: "https://evil.test/self-service/logout?token=stolen",
        }),
        { status: 200 },
      );

    await expect(
      createBrowserLogoutURL("/login", "https://media.test", request),
    ).rejects.toThrow("invalid logout URL");
  });
});
