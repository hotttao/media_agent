import { describe, expect, it, vi } from "vitest";
import {
  loadBrowserFlow,
  type OryBrowserFlow,
  safeReturnURL,
  submitBrowserFlow,
  validateFlowAction,
} from "./ory-flow";

const flow: OryBrowserFlow = {
  id: "flow-1",
  state: "choose_method",
  return_to: "https://media.test/",
  ui: {
    action: "https://media.test/self-service/login?flow=flow-1",
    method: "POST",
    nodes: [],
  },
};

describe("Ory browser flow", () => {
  it("accepts only the expected same-origin action", () => {
    expect(
      validateFlowAction(flow.ui.action, "login", "https://media.test"),
    ).toBe(flow.ui.action);
    expect(() =>
      validateFlowAction(
        "https://evil.test/self-service/login",
        "login",
        "https://media.test",
      ),
    ).toThrow("invalid authentication action");
    expect(() =>
      validateFlowAction(
        "https://media.test/self-service/registration",
        "login",
        "https://media.test",
      ),
    ).toThrow("invalid authentication action");
  });

  it("falls back to the application root for cross-origin return URLs", () => {
    expect(safeReturnURL("https://evil.test/steal", "https://media.test")).toBe(
      "https://media.test/",
    );
    expect(safeReturnURL("/materials", "https://media.test")).toBe(
      "https://media.test/materials",
    );
  });

  it("loads the requested flow with browser credentials", async () => {
    const request = vi.fn(
      async () => new Response(JSON.stringify(flow), { status: 200 }),
    );

    await expect(loadBrowserFlow("login", "flow 1", request)).resolves.toEqual(
      flow,
    );
    expect(request).toHaveBeenCalledWith(
      "/self-service/login/flows?id=flow%201",
      {
        credentials: "include",
        headers: { Accept: "application/json" },
      },
    );
  });

  it("returns the next Kratos step after a partial registration submission", async () => {
    const nextFlow = {
      ...flow,
      ui: {
        ...flow.ui,
        action: "https://media.test/self-service/registration?flow=flow-1",
      },
    };
    const request = vi.fn(
      async () => new Response(JSON.stringify(nextFlow), { status: 200 }),
    );

    await expect(
      submitBrowserFlow(
        "registration",
        nextFlow,
        { "traits.email": "person@example.com", method: "profile" },
        request,
        "https://media.test",
      ),
    ).resolves.toEqual({ status: "flow", flow: nextFlow });
    expect(request).toHaveBeenCalledWith(
      "https://media.test/self-service/registration?flow=flow-1",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({
          "traits.email": "person@example.com",
          method: "profile",
        }),
      }),
    );
  });
});
