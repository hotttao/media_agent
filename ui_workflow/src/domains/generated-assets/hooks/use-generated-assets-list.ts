import { useQuery } from "@tanstack/react-query";
import { listGeneratedAssets } from "@/domains/generated-assets/lib/api";
import type { GeneratedAssetListParams } from "@/domains/generated-assets/types";

export function useGeneratedAssetsList(params: GeneratedAssetListParams = {}) {
  return useQuery({
    queryKey: ["generated-assets", params],
    queryFn: () => listGeneratedAssets(params),
  });
}
