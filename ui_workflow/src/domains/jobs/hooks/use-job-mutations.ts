import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  cancelJob,
  confirmStepRun,
  regenerateStepRun,
  retryJob,
  submitJob,
} from "@/domains/jobs/lib/api";
import type { RetryJobInput, SubmitJobInput } from "@/domains/jobs/types";

/** Invalidate the jobs list + the active job detail (if any). Caller passes
 * the active job id so we can refresh the detail view after mutations. */
function invalidateAll(qc: ReturnType<typeof useQueryClient>, jobId?: string) {
  qc.invalidateQueries({ queryKey: ["jobs"] });
  if (jobId) {
    qc.invalidateQueries({ queryKey: ["job", jobId] });
  }
}

export function useSubmitJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SubmitJobInput) => submitJob(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
}

export function useCancelJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId }: { jobId: string }) => cancelJob(jobId),
    onSuccess: (_job, { jobId }) => invalidateAll(qc, jobId),
  });
}

export function useRetryJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      jobId,
      body,
    }: {
      jobId: string;
      body?: RetryJobInput;
    }) => retryJob(jobId, body ?? {}),
    onSuccess: (_job, { jobId }) => invalidateAll(qc, jobId),
  });
}

export function useConfirmStepRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      stepRunId,
      selectedIndex,
    }: {
      jobId: string;
      stepRunId: string;
      selectedIndex: number;
    }) => confirmStepRun(stepRunId, selectedIndex),
    onSuccess: (_stepRun, { jobId }) => invalidateAll(qc, jobId),
  });
}

export function useRegenerateStepRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ stepRunId }: { jobId: string; stepRunId: string }) =>
      regenerateStepRun(stepRunId),
    onSuccess: (_stepRun, { jobId }) => invalidateAll(qc, jobId),
  });
}
