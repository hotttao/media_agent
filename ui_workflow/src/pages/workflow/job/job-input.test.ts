import { describe, expect, it, vi } from "vitest";
import type { WorkflowOption } from "@/domains/jobs/types";
import {
  buildJobInput,
  missingRequiredInputs,
  isWorkflowReady,
  selectedInputPreview,
} from "./job-input";

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
  it("does not invent media for a text-only input or choose an unrelated resource", () => {
    expect(selectedInputPreview("MATERIAL")).toBeUndefined();
    expect(
      selectedInputPreview("MATERIAL", {
        id: "text",
        name: "Movement",
        scope: "ai_clothing",
        asset_kind: "movement",
        content: "Turn slowly",
        owner_user_id: "owner",
        visibility: "private",
        created_at: "",
        updated_at: "",
      }),
    ).toEqual({ paths: [], content: "Turn slowly" });
  });
  it("previews the selected character full-body reference before its avatar", () => {
    expect(
      selectedInputPreview("VIRTUAL_IP", {
        id: "character",
        name: "Character",
        scope: "ai_clothing",
        status: "active",
        visibility: "private",
        owner_user_id: "owner",
        avatar_object_path: "avatar.png",
        full_body_object_path: "body.png",
        created_at: "",
        updated_at: "",
      })?.paths,
    ).toEqual(["body.png"]);
  });
  it("requires the selected detail receipt, not an incomplete index row, for execution", () => {
    expect(isWorkflowReady(workflow)).toBe(false);
    expect(
      isWorkflowReady({ status: "published", publication_status: "pending" }),
    ).toBe(false);
    expect(
      isWorkflowReady({ status: "draft", publication_status: "registered" }),
    ).toBe(false);
    expect(
      isWorkflowReady({
        status: "published",
        publication_status: "registered",
        allowed_actions: ["run"],
      }),
    ).toBe(true);
    expect(
      isWorkflowReady({
        status: "published",
        publication_status: "registered",
        allowed_actions: ["read"],
      }),
    ).toBe(false);
    expect(
      isWorkflowReady({
        status: "published",
        allowed_actions: ["read", "run"],
      }),
    ).toBe(true);
  });
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
            images: [
              { id: "secondary", object_path: "secondary.png", is_main: false },
              { id: "main", object_path: "product.png", is_main: true },
            ],
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
    expect(input.product?.image_url).toBe("https://storage/product.png");
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
