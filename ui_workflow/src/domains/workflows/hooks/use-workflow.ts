import { useQuery } from "@tanstack/react-query";
import { getWorkflow } from "@/domains/workflows/lib/api";

export function useWorkflow(id: string | null | undefined) {
  return useQuery({
    queryKey: ["workflow", id],
    queryFn: () => getWorkflow(id as string),
    enabled: !!id,
  });
}
