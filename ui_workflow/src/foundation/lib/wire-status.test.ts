import { describe, expect, it } from "vitest";
import { wireStatus } from "./wire-status";

describe("wire status", () => {
  const names = [
    "unknown",
    "draft",
    "published",
    "archived",
    "publishing",
    "publish_failed",
  ] as const;
  it.each([
    [4, "publishing"],
    ["5", "publish_failed"],
    ["WF_PUBLISHED", "published"],
    ["draft", "draft"],
    [99, "unknown"],
    ["UNRECOGNIZED", "unknown"],
    [null, "unknown"],
  ])("preserves %s instead of guessing draft", (input, expected) => {
    expect(wireStatus(input, names, ["wf_"])).toBe(expected);
  });
});
