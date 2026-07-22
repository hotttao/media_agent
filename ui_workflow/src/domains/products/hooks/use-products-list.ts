import { useQuery } from "@tanstack/react-query";
import { listProducts } from "@/domains/products/lib/api";
import type { ProductListParams } from "@/domains/products/types";

export function useProductsList(params: ProductListParams = {}) {
  return useQuery({
    queryKey: ["products", params],
    queryFn: () => listProducts(params),
  });
}
