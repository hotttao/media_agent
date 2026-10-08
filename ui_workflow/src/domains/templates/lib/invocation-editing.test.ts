import { describe, expect, it } from "vitest";
import type { TemplateInput } from "@/domains/templates/types";
import {
  defaultImageInvocation,
  updateTemplateSlot,
  providerIdentity,
} from "./invocation-editing";

const value: TemplateInput = {
  scope: "media",
  name: "portrait",
  prompt_body: "test",
  output_asset_kind: "model_image",
  platform: "jimeng",
  gen_way: "i2i",
  gen_type: "multi_image_prompt",
  capability_key: "jimeng.i2i.multi_image_prompt",
  capability_version: 1,
  inputs: [
    {
      name: "person",
      resource_type: "ASSET",
      asset_kind: "image",
      required: true,
    },
  ],
  invocation_mapping: {
    prompt: { op: "render_template" },
    images: { op: "list", items: [{ op: "asset_ref", slot: "person" }] },
    extra: { op: "asset_ref", slot: "other" },
  },
};
describe("template invocation editing", () => {
  it("derives provider fields without mistaking a business capability for a provider", () => {
    expect(providerIdentity("jimeng.i2i.multi_image_prompt")).toEqual({
      platform: "jimeng",
      gen_way: "i2i",
      gen_type: "multi_image_prompt",
    });
    expect(providerIdentity("media.image.model")).toBeNull();
    expect(
      providerIdentity("doubao.i2v.first_frame_prompt.managed")?.platform,
    ).toBe("doubao");
  });
  it("preserves explicit mappings on unrelated slot edits", () => {
    expect(
      updateTemplateSlot(value, 0, { required: false }).invocation_mapping,
    ).toBe(value.invocation_mapping);
  });
  it("renames nested asset references without dropping other parameters", () => {
    const next = updateTemplateSlot(value, 0, { name: "portrait" });
    expect(next.invocation_mapping.images.items?.[0].slot).toBe("portrait");
    expect(next.invocation_mapping.extra).toEqual(
      value.invocation_mapping.extra,
    );
    expect(value.invocation_mapping.images.items?.[0].slot).toBe("person");
  });
  it("builds the provider images parameter in declared slot order and excludes text", () => {
    expect(
      defaultImageInvocation([
        {
          name: "b",
          resource_type: "ASSET",
          required: true,
          asset_kind: "image",
        },
        {
          name: "note",
          resource_type: "TEXT",
          required: false,
          asset_kind: "text",
        },
        {
          name: "a",
          resource_type: "PRODUCT",
          required: true,
          asset_kind: "product",
        },
      ]).images.items?.map((item) => item.slot),
    ).toEqual(["b", "a"]);
  });
});
