import { useQuery } from "@tanstack/react-query";
import { getTemplate } from "@/domains/templates/lib/api";

export function useTemplate(id: string | null | undefined) {
  return useQuery({
    queryKey: ["template", id],
    queryFn: () => getTemplate(id as string),
    enabled: !!id,
  });
}
