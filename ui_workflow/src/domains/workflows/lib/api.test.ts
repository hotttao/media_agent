import { afterEach, describe, expect, it, vi } from "vitest";
import { getWorkflow, validateWorkflow, updateWorkflow } from "./api";

describe("workflow wire normalization", () => {
  it("does not treat explicit rejection with an empty reasons array as valid", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ valid: false, reject_reasons: [] })),
        ),
    );
    expect(
      (
        await validateWorkflow({
          name: "x",
          scope: "media",
          steps: [],
          output_step_key: "",
        })
      ).errors.length,
    ).toBe(1);
  });
  afterEach(() => vi.unstubAllGlobals());

  it("reads actual rejection reasons instead of claiming validation succeeded", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            valid: false,
            reject_reasons: [
              { subject: "video", message: "Missing image input" },
            ],
            derived_edges: [],
          }),
        ),
      ),
    );
    expect(
      (
        await validateWorkflow({
          scope: "media",
          name: "test",
          steps: [],
          output_step_key: "video",
        })
      ).errors,
    ).toEqual([{ step_key: "video", message: "Missing image input" }]);
  });
  it("accepts a version-only save receipt without inventing a workflow ID or draft", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify({ version: 4 }))),
    );
    expect(await updateWorkflow("wf-1", { name: "new" })).toEqual({
      version: 4,
    });
  });
  it("does not interpret an incomplete validation response as success", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("{}")));
    expect(
      (
        await validateWorkflow({
          name: "x",
          scope: "media",
          steps: [],
          output_step_key: "",
        })
      ).errors.length,
    ).toBe(1);
  });

  it("normalizes protobuf numeric status and binding source enums", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            id: "wf-1",
            scope: "media",
            name: "Video",
            output_step_key: "video",
            status: 2,
            visibility: "private",
            version: 1,
            steps: [
              {
                key: "image",
                template_id: "tpl-image",
                slot_bindings: {
                  product: { source: 1, ref: "product" },
                },
              },
              {
                key: "video",
                template_id: "tpl-video",
                slot_bindings: {
                  image: { source: 2, ref: "image" },
                  duration: { source: 3, const_value: "5" },
                },
              },
            ],
            created_at: "1",
            updated_at: "1",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    const workflow = await getWorkflow("wf-1");

    expect(workflow.status).toBe("published");
    expect(workflow.steps[0]?.slot_bindings[0]?.source).toBe("workflow_input");
    expect(
      workflow.steps[1]?.slot_bindings.map((binding) => binding.source),
    ).toEqual(["step_output", "const"]);
  });
});
