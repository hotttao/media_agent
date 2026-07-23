import { useQuery } from "@tanstack/react-query";
import { getJob } from "@/domains/jobs/lib/api";

/**
 * Fetch a single Job + its StepRuns (GET /jobs/:id returns GetJobResp).
 *
 * First-cut monitoring uses polling: while the Job is `running` we refetch
 * every 2s so the StepRun list and Job status stay fresh without manual
 * refresh. The poller stops for any other status (terminal or waiting on
 * user action).
 *
 * TODO: replace the poller with SSE (GET /jobs/:id/events) for real-time
 * StepRun status transitions. The SSE stream will be wired in a follow-up;
 * until then the 2s poll gives adequate UX for short-to-medium jobs.
 */
export function useJob(id: string | null | undefined) {
  return useQuery({
    queryKey: ["job", id],
    queryFn: () => getJob(id as string),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.job.status;
      return status === "running" ? 2000 : false;
    },
  });
}
