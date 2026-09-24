import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAssetKindsList } from "@/domains/asset-kinds/hooks/use-asset-kinds-list";
import { useMaterialsList } from "@/domains/materials/hooks/use-materials-list";
import { useProductsList } from "@/domains/products/hooks/use-products-list";
import { MaterialsList } from "./material/list";
import { ProductList } from "./product/list";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh-CN" },
  }),
}));
vi.mock("@/domains/materials/hooks/use-materials-list", () => ({
  useMaterialsList: vi.fn(),
}));
vi.mock("@/domains/products/hooks/use-products-list", () => ({
  useProductsList: vi.fn(),
}));
vi.mock("@/domains/asset-kinds/hooks/use-asset-kinds-list", () => ({
  useAssetKindsList: vi.fn(),
}));
vi.mock("@/domains/materials/hooks/use-material-mutations", () => ({
  useDeleteMaterial: () => ({ isPending: false, mutate: vi.fn() }),
}));
vi.mock("@/domains/products/hooks/use-product-mutations", () => ({
  useDeleteProduct: () => ({ isPending: false, mutate: vi.fn() }),
}));

afterEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("materials list", () => {
  it("queries multiple expanded asset-kind selections across the organization", () => {
    vi.mocked(useMaterialsList).mockReturnValue({
      data: { items: [], total: 0 },
    } as unknown as ReturnType<typeof useMaterialsList>);
    vi.mocked(useAssetKindsList).mockReturnValue({
      data: { items: [{ name: "scene" }, { name: "pose" }], total: 2 },
    } as unknown as ReturnType<typeof useAssetKindsList>);
    render(<MaterialsList onSelect={vi.fn()} />);
    expect(useAssetKindsList).toHaveBeenCalledWith({
      resource_type: "MATERIAL",
      media_type: "IMAGE",
      page_size: 200,
    });
    fireEvent.click(screen.getByLabelText("materialsList.kind.scene"));
    expect(useMaterialsList).toHaveBeenLastCalledWith(
      expect.objectContaining({ asset_kind: "scene", only_mine: false }),
    );
    fireEvent.click(screen.getByLabelText("materialsList.kind.pose"));
    expect(useMaterialsList).toHaveBeenLastCalledWith(
      expect.objectContaining({
        asset_kind: "scene,pose",
        only_mine: false,
      }),
    );
    fireEvent.click(screen.getByLabelText("materialsList.allTypes"));
    expect(useMaterialsList).toHaveBeenLastCalledWith(
      expect.objectContaining({ asset_kind: undefined, only_mine: false }),
    );
  });
});

describe("products list", () => {
  it("requests server pages and searches names from the first page", () => {
    vi.mocked(useProductsList).mockReturnValue({
      data: { items: [], total: 41 },
      isFetching: false,
    } as unknown as ReturnType<typeof useProductsList>);
    render(<ProductList onSelect={vi.fn()} />);
    expect(useProductsList).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 20,
      name: "",
    });
    fireEvent.click(screen.getByRole("button", { name: "common.next" }));
    expect(useProductsList).toHaveBeenLastCalledWith({
      page: 2,
      page_size: 20,
      name: "",
    });
    vi.useFakeTimers();
    fireEvent.change(screen.getByPlaceholderText("productsList.search"), {
      target: { value: "coats" },
    });
    act(() => vi.advanceTimersByTime(300));
    expect(useProductsList).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 20,
      name: "coats",
    });
  });
});
