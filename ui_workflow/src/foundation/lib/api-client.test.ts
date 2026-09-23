import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api-client";

describe("ApiClient organization scope", () => {
  afterEach(() => {
    api.setOrganizationId("");
    vi.unstubAllGlobals();
  });

  it("adds the selected company to organization-scoped resource requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ items: [], total: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    api.setOrganizationId("org-123");

    await api.request("/materials?page=2");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/materials?page=2&organization_id=org-123",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("does not add company scope to global registries", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    api.setOrganizationId("org-123");

    await api.request("/asset-kinds");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/asset-kinds",
      expect.any(Object),
    );
  });
});
