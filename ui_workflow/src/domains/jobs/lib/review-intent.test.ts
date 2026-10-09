import { describe, expect, it } from "vitest";
import {
  candidateFingerprint,
  feedbackBytes,
  validCandidatePosition,
} from "./review-intent";
import type { StepRun } from "@/domains/jobs/types";

const step = {
  id: "s",
  rendered_prompt: "prompt",
  candidates: [
    { index: 88, url: "https://store/result.png?sig=a", content: "" },
  ],
} as StepRun;
describe("review UI intent", () => {
  it("uses array positions rather than a provider candidate index", () => {
    expect(validCandidatePosition(step, 0)).toBe(true);
    expect(validCandidatePosition(step, 88)).toBe(false);
    expect(validCandidatePosition(step, null)).toBe(false);
  });
  it("does not reset selection when only a short-lived signature changes", () => {
    expect(candidateFingerprint(step)).toBe(
      candidateFingerprint({
        ...step,
        candidates: [
          { ...step.candidates[0], url: "https://store/result.png?sig=b" },
        ],
      }),
    );
  });
  it("detects different immutable result objects without treating retry_count as generation", () => {
    expect(candidateFingerprint(step)).not.toBe(
      candidateFingerprint({
        ...step,
        candidates: [
          { ...step.candidates[0], url: "https://store/new-result.png" },
        ],
      }),
    );
    expect(candidateFingerprint(step)).toBe(
      candidateFingerprint({ ...step, retry_count: 7 }),
    );
  });
  it("limits feedback by UTF-8 bytes, not JavaScript string length", () => {
    expect(feedbackBytes("你好")).toBe(6);
    expect(feedbackBytes("😀")).toBe(4);
    expect(feedbackBytes("a".repeat(8192))).toBe(8192);
  });
});
