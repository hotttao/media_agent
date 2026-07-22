import { useQuery } from "@tanstack/react-query";
import { listAssetKinds } from "@/domains/asset-kinds/lib/api";
import type { ListAssetKindsParams } from "@/domains/asset-kinds/types";

export function useAssetKindsList(params: ListAssetKindsParams = {}) {
  return useQuery({
    queryKey: ["asset-kinds", params],
    queryFn: () => listAssetKinds(params),
  });
}
