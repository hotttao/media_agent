import { useQuery } from "@tanstack/react-query";
import { getAssetKind } from "@/domains/asset-kinds/lib/api";

export function useAssetKind(name: string | null | undefined) {
  return useQuery({
    queryKey: ["asset-kind", name],
    queryFn: () => getAssetKind(name as string),
    enabled: !!name,
  });
}
