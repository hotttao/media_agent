import { describe, expect, it } from "vitest";
import { workflowToFlowDocument } from "./workflow-graph";

describe("workflowToFlowDocument", () => {
  it("compacts a linear studio flow without adding or changing dependency edges", () => {
    const steps = ["a", "b", "c", "d"].map((key, index, keys) => ({
      key,
      template_id: "tpl",
      slot_bindings: index
        ? [
            {
              source: "step_output" as const,
              to_input: "image",
              ref: keys[index - 1],
            },
          ]
        : [],
    }));
    const horizontal = workflowToFlowDocument({ steps, outputStepKey: "d" });
    const studio = workflowToFlowDocument({
      steps,
      outputStepKey: "d",
      layout: "studio",
    });
    expect(studio.edges).toEqual(horizontal.edges);
    expect(studio.nodes.map((node) => node.id)).toEqual(["a", "b", "c", "d"]);
    expect(studio.nodes[2].meta?.position?.x).toBe(
      studio.nodes[0].meta?.position?.x,
    );
    expect(studio.nodes[2].meta?.position?.y).toBeGreaterThan(
      studio.nodes[0].meta?.position?.y ?? 0,
    );
  });
  it("keeps rank layout for branching dependencies", () => {
    const steps = [
      { key: "a", template_id: "tpl", slot_bindings: [] },
      { key: "b", template_id: "tpl", slot_bindings: [] },
    ];
    expect(
      workflowToFlowDocument({ steps, outputStepKey: "b", layout: "studio" }),
    ).toEqual(workflowToFlowDocument({ steps, outputStepKey: "b" }));
  });
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
