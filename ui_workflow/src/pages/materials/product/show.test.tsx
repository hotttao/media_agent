import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listGeneratedAssets } from "@/domains/generated-assets/lib/api";
import type { GeneratedAsset } from "@/domains/generated-assets/types";
import { ProductShow } from "./show";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, args?: { name?: string }) =>
      args?.name ? `${key} ${args.name}` : key,
  }),
}));
vi.mock("@/foundation/lib/i18n", () => ({
  default: { t: (key: string) => key },
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: { useShellContext: () => ({ organization: { id: "org" } }) },
}));
vi.mock("@/domains/products/hooks/use-product", () => ({
  useProduct: (id: string) => ({
    data: {
      id,
      name: id,
      organization_id: "org",
      scope: "ai_clothing",
      status: "active",
      target_audience: "Product audience",
    },
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/domains/products/hooks/use-product-mutations", () => ({
  useDeleteProduct: () => ({ isPending: false }),
  useArchiveProduct: () => ({ isPending: false }),
}));
vi.mock("./workflow-launch", () => ({ ProductWorkflowLaunch: () => null }));
vi.mock("@/foundation/components/library-gallery", () => ({
  LibraryGallery: () => null,
}));
vi.mock("@/foundation/components/library-media", () => ({
  LibraryMedia: ({
    alt,
    interactive,
  }: {
    alt: string;
    interactive: boolean;
  }) => (
    <div data-testid={interactive === false ? "thumbnail" : "preview"}>
      {alt}
    </div>
  ),
}));
vi.mock("@/domains/generated-assets/lib/api", () => ({
  listGeneratedAssets: vi.fn(),
}));

function asset(name: string): GeneratedAsset {
  return {
    id: name,
    name,
    scope: "ai_clothing",
    asset_kind: "first_frame",
    object_path: `materials/org/${name}.png`,
    url: "",
    owner_user_id: "owner",
    visibility: "private",
    job_id: "job",
    step_run_id: "step",
    template_id: "template",
    template_version: 1,
    source_product_id: "product",
    created_at: "2026-10-26T12:00:00Z",
  };
}
function open() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = (id: string) => (
    <QueryClientProvider client={client}>
      <ProductShow id={id} onBack={vi.fn()} onEdit={vi.fn()} />
    </QueryClientProvider>
  );
  return { ...render(view("product")), view };
}
function related() {
  fireEvent.mouseDown(
    screen.getByRole("tab", { name: "productDetail.relatedTab" }),
    { button: 0, ctrlKey: false },
  );
}
beforeEach(() => {
  vi.mocked(listGeneratedAssets).mockResolvedValue({ items: [], total: 0 });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("product detail related assets", () => {
  it("loads only after tab selection, filters on the server, pages and previews without leaving the product", async () => {
    vi.mocked(listGeneratedAssets).mockImplementation(async (params) => ({
      items: [asset(params?.page === 2 ? "second" : "first")],
      total: 24,
    }));
    open();
    expect(screen.getByText("Product audience")).toBeTruthy();
    expect(listGeneratedAssets).not.toHaveBeenCalled();
    related();
    await screen.findByRole("button", {
      name: "productDetail.previewAsset first",
    });
    expect(listGeneratedAssets).toHaveBeenCalledWith({
      organization_id: "org",
      source_product_id: "product",
      scope: "ai_clothing",
      page: 1,
      page_size: 12,
    });
    fireEvent.click(
      screen.getByRole("button", { name: "productDetail.previewAsset first" }),
    );
    expect(
      within(screen.getByRole("dialog")).getByTestId("preview").textContent,
    ).toBe("first");
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "common.next" }));
    await screen.findByRole("button", {
      name: "productDetail.previewAsset second",
    });
    expect(listGeneratedAssets).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, source_product_id: "product" }),
    );
  });

  it("resets tabs and source paging when the selected product changes", async () => {
    const { rerender, view } = open();
    related();
    await screen.findByText("productDetail.relatedEmpty");
    rerender(view("other-product"));
    expect(
      screen
        .getByRole("tab", { name: "productDetail.infoTab" })
        .getAttribute("data-state"),
    ).toBe("active");
    related();
    await waitFor(() =>
      expect(listGeneratedAssets).toHaveBeenLastCalledWith(
        expect.objectContaining({
          source_product_id: "other-product",
          page: 1,
        }),
      ),
    );
  });

  it("keeps a failed query distinct from an empty result and supports retry", async () => {
    vi.mocked(listGeneratedAssets).mockRejectedValueOnce(
      new Error("Access denied"),
    );
    open();
    related();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Access denied",
    );
    expect(screen.queryByText("productDetail.relatedEmpty")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "v12.retry" }));
    await screen.findByText("productDetail.relatedEmpty");
  });
});
