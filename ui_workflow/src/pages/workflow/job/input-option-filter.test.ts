import { describe, expect, it } from "vitest";
import type { Material } from "@/domains/materials/types";
import type { Product } from "@/domains/products/types";
import { compatibleInputOptions } from "./input-option-filter";

describe("run input compatibility", () => {
  it("does not present orphan-scope or wrong-kind material as a compatible choice", () => {
    const valid = {
      id: "valid",
      scope: "ai_clothing",
      asset_kind: "movement",
    } as Material;
    const orphan = { ...valid, id: "orphan", scope: "orphan" };
    const wrongKind = { ...valid, id: "wrong", asset_kind: "pose" };
    expect(
      compatibleInputOptions(
        "ai_clothing",
        {
          name: "movement",
          resource_type: "MATERIAL",
          asset_kind: "movement",
          required: true,
        },
        [valid, orphan, wrongKind],
      ),
    ).toEqual([valid]);
  });
  it("also restricts product choices to the definition scope", () => {
    const valid = { id: "valid", scope: "ai_clothing" } as Product;
    expect(
      compatibleInputOptions(
        "ai_clothing",
        {
          name: "product",
          resource_type: "PRODUCT",
          asset_kind: "product",
          required: true,
        },
        [valid, { ...valid, scope: "elsewhere" }],
      ),
    ).toEqual([valid]);
  });
});
