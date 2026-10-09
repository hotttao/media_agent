import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api-client";
import { resolveLibraryMedia, uploadLibraryObject } from "./library-object";

afterEach(() => vi.restoreAllMocks());
describe("uploadLibraryObject", () => {
  it("uploads multipart data without forcing a JSON content type", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
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
describe("frozen media reference display", () => {
  it("unwraps the execution reference without changing its organization path", async () => {
    const request = vi
      .spyOn(api, "request")
      .mockResolvedValue({ url: "https://storage.example/signed" });
    await resolveLibraryMedia("org", "object_path:materials/org/video.mp4");
    const query = new URLSearchParams(request.mock.calls[0][0].split("?")[1]);
    expect(query.get("object_path")).toBe("materials/org/video.mp4");
    expect(query.get("organization_id")).toBe("org");
  });
  it("does not rewrite a foreign organization path or unwrap URL text", async () => {
    const request = vi
      .spyOn(api, "request")
      .mockResolvedValue({ url: "https://storage.example/signed" });
    await resolveLibraryMedia("org", "object_path:materials/other/file.png");
    expect(
      new URLSearchParams(request.mock.calls[0][0].split("?")[1]).get(
        "object_path",
      ),
    ).toBe("materials/other/file.png");
    const url =
      "https://example.test/path/object_path:file.png?signature=sample";
    await resolveLibraryMedia("org", url);
    expect(
      new URLSearchParams(request.mock.calls[1][0].split("?")[1]).get(
        "object_path",
      ),
    ).toBe(url);
  });
});
