import { useQuery } from "@tanstack/react-query";
import { getGeneratedAsset } from "@/domains/generated-assets/lib/api";

export function useGeneratedAsset(id: string | null | undefined) {
  return useQuery({
    queryKey: ["generated-asset", id],
    queryFn: () => getGeneratedAsset(id as string),
    enabled: !!id,
  });
}
