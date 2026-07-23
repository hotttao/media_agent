import { useQuery } from "@tanstack/react-query";
import { listWorkflows } from "@/domains/workflows/lib/api";
import type { WorkflowListParams } from "@/domains/workflows/types";

export function useWorkflowsList(params: WorkflowListParams = {}) {
  return useQuery({
    queryKey: ["workflows", params],
    queryFn: () => listWorkflows(params),
  });
}
