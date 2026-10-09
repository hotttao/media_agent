import { useQuery } from "@tanstack/react-query";
import { listJobs } from "@/domains/jobs/lib/api";
import type { JobListParams } from "@/domains/jobs/types";

export function useJobsList(params: JobListParams = {}, enabled = true) {
  return useQuery({
    queryKey: ["jobs", params],
    queryFn: () => listJobs(params),
    enabled,
  });
}
