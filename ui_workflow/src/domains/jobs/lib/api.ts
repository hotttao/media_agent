/**
 * media_workflow Execution API client. Wraps foundation/lib/api-client
 * (cookie auth, onUnauthorized). Base path /api/v1 is prepended by the
 * ApiClient.
 *
 * Routes per idl/media_workflow/job.proto (JobService) +
 * docs/ddd/media_workflow/domain/0003-execution:
 *   POST   /jobs                       -> SubmitJob (body: {workflow_id, input})
 *   GET    /jobs/:id                   -> GetJobResp { job, step_runs }
 *   GET    /jobs                       -> ListJobsResp { items, total }
 *   POST   /jobs/:id/cancel            -> Job
 *   POST   /jobs/:id/retry             -> Job
 *   POST   /step-runs/:id/confirm      -> StepRun (body: {selected_index})
 *   POST   /step-runs/:id/regenerate   -> StepRun
 *   GET    /jobs/:id/events (SSE)      -> TODO: real-time stream; first cut
 *        uses polling (see use-job.ts refetchInterval). SSE will replace the
 *        poller in a follow-up.
 * Worker-facing endpoints (claim/complete/fail) are intentionally not exposed
 * here - the UI is a user client, not a worker.
 *
 * Workflow listing (/workflows) is read here for the JobCreate selector only;
 * a full workflows domain does not exist yet (see WorkflowOption in types.ts).
 */
import { api } from "@/foundation/lib/api-client";
import { wireStatus } from "@/foundation/lib/wire-status";
import type {
  GetJobResponse,
  Job,
  JobListParams,
  JobStatus,
  JobsPage,
  RetryJobInput,
  StepRun,
  StepRunStatus,
  SubmitJobInput,
  WorkflowOption,
  WorkflowsListResponse,
} from "../types";

const JOB_STATUSES: JobStatus[] = [
  "unknown",
  "pending",
  "running",
  "need_confirm",
  "succeeded",
  "failed",
  "cancelled",
  "awaiting_resume",
  "paused",
];
const STEP_STATUSES: StepRunStatus[] = [
  "blocked",
  "blocked",
  "pending",
  "claimed",
  "need_confirm",
  "succeeded",
  "failed",
  "skipped",
];

type WireJob = Omit<Job, "status" | "retry_history"> & {
  status: number | JobStatus;
  retry_history?: string | Job["retry_history"];
};
type WireStepRun = Omit<StepRun, "status" | "resolved_inputs"> & {
  status: number | StepRunStatus;
  resolved_inputs?: string | StepRun["resolved_inputs"];
};

function parseJSON<T>(value: string | T | undefined, fallback: T): T {
  if (typeof value !== "string") return value ?? fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

function normalizeJob(job: WireJob): Job {
  return {
    ...job,
    status: wireStatus(job.status, JOB_STATUSES, ["job_status_"]),
    retry_history: parseJSON(job.retry_history, []),
  };
}

function normalizeStepRun(stepRun: WireStepRun): StepRun {
  return {
    ...stepRun,
    status:
      typeof stepRun.status === "number"
        ? (STEP_STATUSES[stepRun.status] ?? "blocked")
        : stepRun.status,
    resolved_inputs: parseJSON(stepRun.resolved_inputs, {}),
    selected_index: stepRun.selected_index ?? 0,
    candidates: (stepRun.candidates ?? []).map((candidate, index) => ({
      ...candidate,
      index: candidate.index ?? index,
    })),
  };
}

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

// ── Job ─────────────────────────────────────────────────────────────────

export async function listJobs(params: JobListParams = {}): Promise<JobsPage> {
  const result = await api.request<{ items?: WireJob[]; total: number }>(
    `/jobs${buildQuery(params)}`,
  );
  return { ...result, items: (result.items ?? []).map(normalizeJob) };
}

export async function getJob(id: string): Promise<GetJobResponse> {
  const result = await api.request<{ job: WireJob; step_runs?: WireStepRun[] }>(
    `/jobs/${encodeURIComponent(id)}`,
  );
  return {
    job: normalizeJob(result.job),
    step_runs: (result.step_runs ?? []).map(normalizeStepRun),
  };
}

export async function submitJob(
  input: SubmitJobInput,
): Promise<{ id: string }> {
  const result = await api.request<{ job_id: string }>("/jobs", {
    method: "POST",
    body: JSON.stringify(input),
  });
  return { id: result.job_id };
}

export async function cancelJob(id: string): Promise<Job> {
  const result = await api.request<WireJob>(
    `/jobs/${encodeURIComponent(id)}/cancel`,
    {
      method: "POST",
    },
  );
  return normalizeJob(result);
}

export async function retryJob(
  id: string,
  body: RetryJobInput = {},
): Promise<Job> {
  const result = await api.request<WireJob>(
    `/jobs/${encodeURIComponent(id)}/retry`,
    {
      method: "POST",
      body: JSON.stringify(body),
    },
  );
  return normalizeJob(result);
}

// ── StepRun (user-facing) ───────────────────────────────────────────────

export async function confirmStepRun(
  stepRunId: string,
  selectedIndex: number,
): Promise<StepRun> {
  const result = await api.request<WireStepRun>(
    `/step-runs/${encodeURIComponent(stepRunId)}/confirm`,
    {
      method: "POST",
      body: JSON.stringify({ selected_index: selectedIndex }),
    },
  );
  return normalizeStepRun(result);
}

export async function regenerateStepRun(stepRunId: string): Promise<StepRun> {
  const result = await api.request<WireStepRun>(
    `/step-runs/${encodeURIComponent(stepRunId)}/regenerate`,
    { method: "POST" },
  );
  return normalizeStepRun(result);
}

// ── Workflow list (minimal projection for JobCreate selector) ───────────

export async function listWorkflows(): Promise<WorkflowOption[]> {
  const page = await api.request<WorkflowsListResponse>(
    "/workflows?status=published&page_size=200",
  );
  return page.items.filter(
    (workflow) =>
      workflow.status === 2 ||
      String(workflow.status).toLowerCase().includes("published"),
  );
}
