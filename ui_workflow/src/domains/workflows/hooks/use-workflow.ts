import { useQuery } from "@tanstack/react-query";
import { getWorkflow, getWorkflowVersion } from "@/domains/workflows/lib/api";

export function useWorkflow(id: string | null | undefined, version?: number) {
  return useQuery({
    queryKey:
      version === undefined
        ? ["workflow", id]
        : ["workflow", id, "version", version],
    queryFn: () =>
      version === undefined
        ? getWorkflow(id as string)
        : getWorkflowVersion(id as string, version),
    enabled: !!id,
    refetchInterval: (query) =>
      query.state.data?.status === "publishing" ? 2000 : false,
  });
}
