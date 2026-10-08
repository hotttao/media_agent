import { describe, expect, it } from "vitest";
import type { GetJobResponse } from "@/domains/jobs/types";
import { executionPosition, jobPriority } from "./execution-summary";

describe("execution summaries", () => {
  it("never describes imported unfinished work or a pause as cancellation", () => {
    expect(executionPosition("awaiting_resume").key).toBe(
      "v12.jobPosition.awaiting_resume",
    );
    expect(executionPosition("paused").key).toBe("v12.jobPosition.paused");
    expect(executionPosition("unknown").key).toBe("v12.jobPosition.unknown");
  });
  it("shows the action needed before a parallel running branch", () => {
    const detail = {
      step_runs: [
        { step_order: 2, step_key: "video", status: "claimed" },
        { step_order: 1, step_key: "image", status: "need_confirm" },
      ],
    } as GetJobResponse;
    expect(executionPosition("need_confirm", detail)).toEqual({
      key: "v12.jobPosition.review",
      values: { steps: "image" },
    });
    expect(jobPriority("need_confirm")).toBeLessThan(jobPriority("running"));
  });
  it("does not invent node states before a snapshot is loaded", () => {
    expect(executionPosition("running").key).toBe("v12.jobPosition.loading");
  });
});
