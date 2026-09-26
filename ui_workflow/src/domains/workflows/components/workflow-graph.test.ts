import { describe, expect, it } from "vitest";
import { workflowToFlowDocument } from "./workflow-graph";

describe("workflowToFlowDocument", () => {
  it("derives visual edges from canonical slot bindings", () => {
    const document = workflowToFlowDocument({
      outputStepKey: "video",
      steps: [
        {
          key: "image",
          template_id: "tpl-image",
          slot_bindings: [
            { source: "workflow_input", to_input: "person", ref: "person" },
          ],
        },
        {
          key: "video",
          template_id: "tpl-video",
          slot_bindings: [
            { source: "step_output", to_input: "image", ref: "image" },
            { source: "workflow_input", to_input: "motion", ref: "motion" },
          ],
        },
      ],
    });

    expect(document.nodes.map((node) => node.id)).toEqual([
      "__workflow_input__",
      "image",
      "video",
    ]);
    expect(document.edges).toEqual([
      { sourceNodeID: "__workflow_input__", targetNodeID: "image" },
      { sourceNodeID: "image", targetNodeID: "video" },
      { sourceNodeID: "__workflow_input__", targetNodeID: "video" },
    ]);
    const imageX = document.nodes[1]?.meta?.position?.x ?? 0;
    const videoX = document.nodes[2]?.meta?.position?.x ?? 0;
    expect(videoX).toBeGreaterThan(imageX);
  });
});
