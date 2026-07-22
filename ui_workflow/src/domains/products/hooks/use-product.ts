import { useQuery } from "@tanstack/react-query";
import { getProduct } from "@/domains/products/lib/api";

export function useProduct(id: string | null | undefined) {
  return useQuery({
    queryKey: ["product", id],
    queryFn: () => getProduct(id as string),
    enabled: !!id,
  });
}
