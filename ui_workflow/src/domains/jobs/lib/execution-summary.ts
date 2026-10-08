import type { GetJobResponse, JobStatus } from "@/domains/jobs/types";

/** A missing snapshot must not masquerade as an empty or cancelled execution. */
export function executionPosition(
  status: JobStatus,
  detail?: GetJobResponse,
): { key: string; values?: Record<string, string> } {
  if (
    ["awaiting_resume", "paused", "succeeded", "cancelled", "unknown"].includes(
      status,
    )
  )
    return { key: `v12.jobPosition.${status}` };
  if (!detail)
    return {
      key:
        status === "failed"
          ? "v12.jobPosition.failed"
          : "v12.jobPosition.loading",
      values: status === "failed" ? { step: "—" } : undefined,
    };
  const steps = [...detail.step_runs].sort(
    (a, b) => a.step_order - b.step_order,
  );
  const review = steps.filter((step) => step.status === "need_confirm");
  if (review.length)
    return {
      key: "v12.jobPosition.review",
      values: { steps: review.map((step) => step.step_key).join(" / ") },
    };
  const failed = steps.find((step) => step.status === "failed");
  if (status === "failed" && failed)
    return { key: "v12.jobPosition.failed", values: { step: failed.step_key } };
  const running = steps.filter(
    (step) => step.status === "running" || step.status === "claimed",
  );
  if (running.length)
    return {
      key: "v12.jobPosition.running",
      values: { steps: running.map((step) => step.step_key).join(" / ") },
    };
  const pending = steps.find((step) => step.status === "pending");
  if (pending)
    return {
      key: "v12.jobPosition.pending",
      values: { step: pending.step_key },
    };
  return {
    key: steps.length
      ? "v12.jobPosition.waiting"
      : "v12.jobPosition.notStarted",
  };
}

export function jobPriority(status: JobStatus): number {
  return (
    (
      {
        need_confirm: 0,
        awaiting_resume: 1,
        failed: 2,
        running: 3,
        pending: 4,
        paused: 5,
      } as Partial<Record<JobStatus, number>>
    )[status] ?? 6
  );
}
