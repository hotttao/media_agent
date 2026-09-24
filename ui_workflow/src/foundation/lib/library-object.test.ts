import { afterEach, describe, expect, it, vi } from "vitest";
import { uploadLibraryObject } from "./library-object";

describe("uploadLibraryObject", () => {
  afterEach(() => vi.restoreAllMocks());

  it("uploads multipart data without forcing a JSON content type", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          object_path: "materials/org/demo/material/file.png",
          size_bytes: 3,
          content_type: "image/png",
          checksum: "abc",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    const result = await uploadLibraryObject({
      organizationId: "org",
      scope: "demo",
      purpose: "material",
      file: new File(["png"], "file.png", { type: "image/png" }),
      idempotencyKey: "key-1",
    });
    expect(result.object_path).toContain("materials/org/");
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toContain("organization_id=org");
    expect(String(url)).toContain("scope=demo");
    expect(init?.body).toBeInstanceOf(FormData);
    expect(init?.headers).toEqual({ "Idempotency-Key": "key-1" });
  });

  it("surfaces the storage error body", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("file too large", { status: 413 }),
    );
    await expect(
      uploadLibraryObject({
        organizationId: "org",
        scope: "demo",
        purpose: "material",
        file: new File(["x"], "x.png"),
        idempotencyKey: "key-2",
      }),
    ).rejects.toThrow("file too large");
  });
});
