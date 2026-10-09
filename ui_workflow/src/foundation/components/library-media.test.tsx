import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resolveLibraryMedia } from "../lib/library-object";
import { LibraryMedia } from "./library-media";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("../lib/library-object", () => ({ resolveLibraryMedia: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("library media preview", () => {
  it("uses the storage content type to preview a video without a file extension", async () => {
    vi.mocked(resolveLibraryMedia).mockResolvedValue({
      url: "https://storage.example/object-key",
      content_type: "video/mp4",
    });
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LibraryMedia
          organizationId="org"
          objectPath="objects/opaque-key"
          alt="Preview clip"
        />
      </QueryClientProvider>,
    );
    const preview = await screen.findByLabelText("Preview clip");
    expect(preview.tagName).toBe("VIDEO");
    expect(preview.getAttribute("src")).toBe(
      "https://storage.example/object-key",
    );
    expect(resolveLibraryMedia).toHaveBeenCalledWith(
      "org",
      "objects/opaque-key",
    );
  });
});
