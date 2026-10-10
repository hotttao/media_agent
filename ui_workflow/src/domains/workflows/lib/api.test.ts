import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getWorkflow,
  getWorkflowVersion,
  validateWorkflow,
  updateWorkflow,
  createWorkflow,
} from "./api";

describe("workflow wire normalization", () => {
  it("reads a frozen version without consulting the current head", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({
            id: "wf",
            version: 1,
            status: 2,
            definition_source: "snapshot",
            my_permission: "editor",
            allowed_actions: ["read"],
            steps: [
              {
                key: "old",
                slot_bindings: { source: { source: 1, ref: "old_input" } },
              },
            ],
            input_schema: {
              entries: [
                { name: "old_input", resource_type: "TEXT", required: true },
              ],
            },
          }),
        ),
      );
    vi.stubGlobal("fetch", fetch);
    const frozen = await getWorkflowVersion("wf", 1);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][0]).toContain("/workflows/wf/versions/1");
    expect(frozen.steps[0].slot_bindings[0]).toMatchObject({
      source: "workflow_input",
      ref: "old_input",
    });
    expect(frozen.allowed_actions).toEqual(["read"]);
    expect(frozen.input_schema?.entries).toEqual([
      { name: "old_input", resource_type: "TEXT", required: true },
    ]);
    await expect(getWorkflowVersion("wf", 0)).rejects.toThrow();
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("serializes binding enums as numeric HTTP values for validate, create and update", async () => {
    const fetch = vi
      .fn()
      .mockImplementation((_url: string, _options?: RequestInit) =>
        Promise.resolve(
          new Response(JSON.stringify({ valid: true, id: "wf", version: 1 })),
        ),
      );
    vi.stubGlobal("fetch", fetch);
    const input = {
      name: "Probe",
      scope: "media",
      output_step_key: "video",
      steps: [
        {
          key: "video",
          template_id: "template",
          template_version: 1,
          slot_bindings: [
            {
              to_input: "product",
              source: "workflow_input" as const,
              ref: "product",
            },
            { to_input: "image", source: "step_output" as const, ref: "image" },
            {
              to_input: "note",
              source: "const" as const,
              const_value: "literal",
            },
          ],
        },
      ],
    };
    await validateWorkflow(input);
    await createWorkflow(input);
    await updateWorkflow("wf", input);
    for (const call of fetch.mock.calls) {
      const body = JSON.parse(String(call[1]?.body));
      expect(body.steps[0].slot_bindings).toEqual({
        product: { source: 1, ref: "product", const_value: "" },
        image: { source: 2, ref: "image", const_value: "" },
        note: { source: 3, ref: "", const_value: "literal" },
      });
    }
  });
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
