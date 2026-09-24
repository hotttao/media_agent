import { useQuery } from "@tanstack/react-query";
import { listAssetKinds } from "@/domains/materials/lib/api";
import type { AssetKindResourceType } from "@/domains/materials/types";

/** List asset kinds, optionally filtered to a resource type (e.g. MATERIAL for
 * the Material form's kind selector). */
export function useAssetKinds(resourceType?: AssetKindResourceType) {
  return useQuery({
    queryKey: ["asset-kinds", resourceType],
    queryFn: async () => {
      const all = await listAssetKinds();
      return resourceType
        ? all.items.filter((k) => k.resource_type === resourceType)
        : all.items;
    },
  });
}
