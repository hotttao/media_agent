import { useQuery } from "@tanstack/react-query";
import { listMaterials } from "@/domains/materials/lib/api";
import type { MaterialListParams } from "@/domains/materials/types";

export function useMaterialsList(params: MaterialListParams = {}) {
  return useQuery({
    queryKey: ["materials", params],
    queryFn: () => listMaterials(params),
  });
}
