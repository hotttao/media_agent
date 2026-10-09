import { afterEach, describe, expect, it, vi } from "vitest";
import { getTemplateVersion, publishTemplate } from "./api";

afterEach(() => vi.unstubAllGlobals());
describe("template version API", () => {
  it("publishes via the native endpoint and reads the actual returned version/status", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ id: "tpl/a", version: 4, status: "PUBLISHED" }),
        ),
      );
    vi.stubGlobal("fetch", fetch);
    const result = await publishTemplate("tpl/a");
    expect(fetch.mock.calls[0][0]).toBe("/api/v1/templates/tpl%2Fa/publish");
    expect(fetch.mock.calls[0][1].method).toBe("POST");
    expect(result).toMatchObject({
      id: "tpl/a",
      version: 4,
      status: "published",
      inputs: [],
      invocation_mapping: {},
    });
  });
  it("retains historical mappings and does not substitute the latest template", async () => {
    const invocation_mapping = {
      images: { op: "list", items: [{ op: "asset_ref", slot: "old_slot" }] },
    };
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "tpl",
          version: 2,
          status: 2,
          invocation_mapping,
        }),
      ),
    );
    vi.stubGlobal("fetch", fetch);
    const result = await getTemplateVersion("tpl", 2);
    expect(fetch.mock.calls[0][0]).toBe("/api/v1/templates/tpl/versions/2");
    expect(result.invocation_mapping).toEqual(invocation_mapping);
    expect(result.version).toBe(2);
  });
});
