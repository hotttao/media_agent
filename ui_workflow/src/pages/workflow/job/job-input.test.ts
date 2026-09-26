import { describe, expect, it, vi } from "vitest";
import type { WorkflowOption } from "@/domains/jobs/types";
import { buildJobInput, missingRequiredInputs } from "./job-input";

const workflow: WorkflowOption = {
  id: "wf-1",
  name: "商品视频",
  scope: "ai_clothing",
  version: 1,
  status: 2,
  input_schema: {
    entries: [
      { name: "product", resource_type: "PRODUCT", required: true },
      {
        name: "movement",
        resource_type: "MATERIAL",
        asset_kind: "movement",
        required: true,
      },
      { name: "virtual_ip", resource_type: "VIRTUAL_IP", required: true },
    ],
  },
};

describe("job input form mapping", () => {
  it("reports required inputs from the workflow contract", () => {
    expect(missingRequiredInputs(workflow, { product: "p-1" })).toEqual([
      "movement",
      "virtual_ip",
    ]);
  });

  it("builds a typed bundle and resolves storage object paths", async () => {
    const resolveObject = vi.fn(async (path?: string) =>
      path ? `https://storage/${path}` : "",
    );
    const input = await buildJobInput(
      workflow,
      { product: "p-1", movement: "m-1", virtual_ip: "v-1" },
      {
        product: [
          {
            id: "p-1",
            name: "裙子",
            scope: "ai_clothing",
            status: "active",
            created_by: "u",
            created_at: "",
            updated_at: "",
            display_actions: ["转身"],
          },
        ],
        movement: [
          {
            id: "m-1",
            name: "转身",
            scope: "ai_clothing",
            asset_kind: "movement",
            object_path: "movement.png",
            owner_user_id: "u",
            visibility: "public",
            created_at: "",
            updated_at: "",
          },
        ],
        virtual_ip: [
          {
            id: "v-1",
            name: "模特",
            scope: "ai_clothing",
            status: "active",
            visibility: "public",
            owner_user_id: "u",
            avatar_object_path: "avatar.png",
            full_body_object_path: "body.png",
            created_at: "",
            updated_at: "",
          },
        ],
      },
      resolveObject,
    );

    expect(input.product?.display_actions).toBe('["转身"]');
    expect(input.material).toEqual([
      {
        name: "movement",
        asset_kind: "movement",
        url: "https://storage/movement.png",
        content: "",
      },
    ]);
    expect(input.virtual_ip?.full_body_url).toBe("https://storage/body.png");
    expect(resolveObject).toHaveBeenCalledWith("avatar.png");
  });
});
