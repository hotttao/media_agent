import { describe, expect, it } from "vitest";
import { normalizeProductImages } from "./product-images-field";

describe("normalizeProductImages", () => {
  it("assigns the first image as main and reindexes sort order", () => {
    expect(
      normalizeProductImages([
        { object_path: "a", is_main: false, sort_order: 8 },
        { object_path: "b", is_main: false, sort_order: 4 },
      ]),
    ).toEqual([
      { object_path: "a", is_main: true, sort_order: 0 },
      { object_path: "b", is_main: false, sort_order: 1 },
    ]);
  });

  it("preserves an explicitly selected main image", () => {
    expect(
      normalizeProductImages([
        { object_path: "a", is_main: false, sort_order: 0 },
        { object_path: "b", is_main: true, sort_order: 1 },
      ])[1].is_main,
    ).toBe(true);
  });
});
