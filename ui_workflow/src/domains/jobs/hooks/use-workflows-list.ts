import { useQuery } from "@tanstack/react-query";
import { listWorkflows } from "@/domains/jobs/lib/api";

/**
 * Minimal workflow list for the JobCreate selector. Returns just
 * {id, name, version} per WorkflowOption - enough to label a <Select>
 * option. A full workflows domain is not yet wired up; this reads the
 * /workflows index directly via the jobs domain (dep-cruiser forbids L2
 * cross-domain imports).
 */
export function useWorkflowsList() {
  return useQuery({
    queryKey: ["jobs", "workflows-for-selector"],
    queryFn: () => listWorkflows(),
  });
}
