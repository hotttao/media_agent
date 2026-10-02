import { describe, expect, it } from "vitest";
import { workflowToFlowDocument } from "./workflow-graph";

describe("workflowToFlowDocument", () => {
  it("derives visual edges from canonical slot bindings", () => {
    const document = workflowToFlowDocument({
      outputStepKey: "video",
      templateNameByID: { "tpl-image": "模特图生成" },
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

    expect(document.nodes.map((node) => node.id)).toEqual(["image", "video"]);
    expect(document.nodes[0]?.data?.subtitle).toBe("模特图生成");
    expect(document.nodes[1]?.data?.subtitle).toBe("生成节点");
    expect(document.edges).toEqual([
      { sourceNodeID: "image", targetNodeID: "video" },
    ]);
    const imageX = document.nodes[0]?.meta?.position?.x ?? 0;
    const videoX = document.nodes[1]?.meta?.position?.x ?? 0;
    expect(videoX).toBeGreaterThan(imageX);
  });
});
