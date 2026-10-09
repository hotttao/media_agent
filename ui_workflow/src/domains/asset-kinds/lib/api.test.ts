import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/foundation/lib/api-client";
import {
  createAssetKind,
  getAssetKind,
  listAssetKinds,
  updateAssetKind,
} from "./api";

const row = { name: "scene", resource_type: 1, media_type: 1, status: 1 };
afterEach(() => vi.restoreAllMocks());
describe("asset kind HTTP contract", () => {
  it("normalizes list enums so material kind filtering works", async () => {
    vi.spyOn(api, "request").mockResolvedValue({ items: [row], total: 1 });
    expect((await listAssetKinds()).items[0]).toMatchObject({
      resource_type: "MATERIAL",
      media_type: "IMAGE",
      status: "SYSTEM",
    });
  });
  it("accepts named enums and protobuf empty lists", async () => {
    const request = vi.spyOn(api, "request");
    request.mockResolvedValueOnce({
      ...row,
      resource_type: "MATERIAL",
      media_type: "IMAGE",
      status: "SYSTEM",
    });
    expect((await getAssetKind("scene")).status).toBe("SYSTEM");
    request.mockResolvedValueOnce({});
    expect(await listAssetKinds()).toEqual({ items: [], total: 0 });
  });
  it("sends numeric create enums but preserves the update string status contract", async () => {
    const request = vi.spyOn(api, "request").mockResolvedValue(row);
    await createAssetKind({
      name: "scene",
      resource_type: "MATERIAL",
      media_type: "IMAGE",
    });
    expect(JSON.parse(request.mock.calls[0][1]!.body as string)).toMatchObject({
      resource_type: 1,
      media_type: 1,
    });
    await updateAssetKind("scene", {
      description: "Updated",
      status: "DISABLED",
    });
    expect(JSON.parse(request.mock.calls[1][1]!.body as string)).toMatchObject({
      status: "DISABLED",
    });
  });
});
