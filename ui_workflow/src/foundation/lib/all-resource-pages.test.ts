import { describe, expect, it, vi } from "vitest";
import { allResourcePages } from "./all-resource-pages";
describe("authorized metadata pagination", () => {
  it("loads beyond the first page and deduplicates IDs", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce({ items: [{ id: "a" }, { id: "b" }], total: 3 })
      .mockResolvedValueOnce({ items: [{ id: "b" }, { id: "c" }], total: 3 });
    expect(await allResourcePages(load)).toEqual([
      { id: "a" },
      { id: "b" },
      { id: "c" },
    ]);
    expect(load).toHaveBeenNthCalledWith(2, 2);
  });
  it("rejects a repeated page instead of looping or silently hiding resources", async () => {
    await expect(
      allResourcePages(
        vi.fn().mockResolvedValue({ items: [{ id: "a" }], total: 2 }),
      ),
    ).rejects.toThrow("did not advance");
  });
});
