import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("joins multiple class strings", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("filters falsy values", () => {
    expect(cn("foo", false, undefined, null, "", "bar")).toBe("foo bar");
  });

  it("merges conflicting tailwind utilities (later wins)", () => {
    expect(cn("px-2", "px-4")).toBe("px-4");
  });
});
