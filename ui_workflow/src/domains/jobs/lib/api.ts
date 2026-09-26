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
import type {
  GetJobResponse,
  Job,
  JobListParams,
  JobsPage,
  RetryJobInput,
  StepRun,
  SubmitJobInput,
  WorkflowOption,
  WorkflowsListResponse,
} from "../types";

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
  return api.request<JobsPage>(`/jobs${buildQuery(params)}`);
}

export async function getJob(id: string): Promise<GetJobResponse> {
  return api.request<GetJobResponse>(`/jobs/${encodeURIComponent(id)}`);
}

export async function submitJob(input: SubmitJobInput): Promise<Job> {
  return api.request<Job>("/jobs", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function cancelJob(id: string): Promise<Job> {
  return api.request<Job>(`/jobs/${encodeURIComponent(id)}/cancel`, {
    method: "POST",
  });
}

export async function retryJob(
  id: string,
  body: RetryJobInput = {},
): Promise<Job> {
  return api.request<Job>(`/jobs/${encodeURIComponent(id)}/retry`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

// ── StepRun (user-facing) ───────────────────────────────────────────────

export async function confirmStepRun(
  stepRunId: string,
  selectedIndex: number,
): Promise<StepRun> {
  return api.request<StepRun>(
    `/step-runs/${encodeURIComponent(stepRunId)}/confirm`,
    {
      method: "POST",
      body: JSON.stringify({ selected_index: selectedIndex }),
    },
  );
}

export async function regenerateStepRun(stepRunId: string): Promise<StepRun> {
  return api.request<StepRun>(
    `/step-runs/${encodeURIComponent(stepRunId)}/regenerate`,
    { method: "POST" },
  );
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
