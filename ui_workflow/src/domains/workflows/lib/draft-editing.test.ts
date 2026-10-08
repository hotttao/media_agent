import { describe, expect, it } from "vitest";
import type { WorkflowInput } from "@/domains/workflows/types";
import {
  removalImpact,
  removeDraftStep,
  updateDraftStep,
} from "./draft-editing";

const draft: WorkflowInput = {
  name: "Video",
  scope: "media",
  output_step_key: "video",
  steps: [
    { key: "image", template_id: "t1", slot_bindings: [] },
    {
      key: "video",
      template_id: "t2",
      slot_bindings: [
        { source: "step_output", to_input: "first_frame", ref: "image" },
        { source: "workflow_input", to_input: "movement", ref: "image" },
      ],
    },
  ],
};
describe("workflow draft editing", () => {
  it("renames only step-output references, without rewriting workflow input names", () => {
    const next = updateDraftStep(draft, 0, { key: "portrait" });
    expect(next.steps[1].slot_bindings.map((binding) => binding.ref)).toEqual([
      "portrait",
      "image",
    ]);
    expect(draft.steps[0].key).toBe("image");
  });
  it("updates the final output reference when that node is renamed", () => {
    expect(updateDraftStep(draft, 1, { key: "clip" }).output_step_key).toBe(
      "clip",
    );
  });
  it("lists deletion impact and keeps broken bindings visible for explicit repair", () => {
    expect(removalImpact(draft, "image")).toEqual(["video → first_frame"]);
    expect(removeDraftStep(draft, "image").steps[0].slot_bindings[0].ref).toBe(
      "image",
    );
  });
  it("clears a removed output instead of silently selecting an unrelated node", () => {
    expect(removeDraftStep(draft, "video").output_step_key).toBe("");
  });
});
