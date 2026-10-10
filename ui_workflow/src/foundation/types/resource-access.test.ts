import { describe, expect, it } from "vitest";
import { allows } from "./resource-access";
describe("resource action projection", () => {
  it("fails closed when projection is absent and does not derive commands from resource role", () => {
    expect(allows(undefined, "update")).toBe(false);
    expect(allows({}, "update")).toBe(false);
    expect(allows({ allowed_actions: ["read"] }, "publish")).toBe(false);
    expect(allows({ allowed_actions: ["read", "update"] }, "update")).toBe(
      true,
    );
  });
});
