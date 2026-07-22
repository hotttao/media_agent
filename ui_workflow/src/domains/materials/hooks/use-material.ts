import { useQuery } from "@tanstack/react-query";
import { getMaterial } from "@/domains/materials/lib/api";

export function useMaterial(id: string | null | undefined) {
  return useQuery({
    queryKey: ["material", id],
    queryFn: () => getMaterial(id as string),
    enabled: !!id,
  });
}
