/**
 * media_workflow Execution domain types (Job + StepRun).
 *
 * Field shapes confirmed against idl/media_workflow/job.proto (JobService).
 * Like media_materials entities, Job/StepRun are flat rows - no k8s-style
 * Metadata/Spec/Status wrapping. Timestamps are int64 in the proto but typed
 * as string here (serialized as string to preserve precision, matching the
 * Material convention - see domains/generated-assets/types.ts).
 *
 * Status enums union the proto wire values (pending/running/need_confirm/
 * succeeded/failed/cancelled for Job; pending/running/succeeded/failed/
 * skipped/blocked/claimed/need_confirm for StepRun) with the doc's text state
 * machine (docs/ddd/media_workflow/domain/0003-execution/execution.md).
 */

// ── JobInput bundle (self-contained; proto message JobInput) ─────────────

export interface ProductData {
  id?: string;
  name?: string;
  target_audience?: string;
  product_details?: string;
  display_actions?: string;
  image_url?: string;
}

export interface VirtualIpData {
  id?: string;
  name?: string;
  avatar_url?: string;
  full_body_url?: string;
  three_view_url?: string;
  nine_view_url?: string;
}

export interface AssetData {
  name?: string;
  url?: string;
  content?: string;
  asset_kind?: string;
}

export interface MaterialData {
  name?: string;
  url?: string;
  content?: string;
  asset_kind?: string;
}

export interface JobInput {
  product?: ProductData;
  virtual_ip?: VirtualIpData;
  asset?: AssetData[];
  material?: MaterialData[];
}

// ── Candidate (StepRun output option, proto message Candidate) ──────────

export interface Candidate {
  index: number;
  url: string;
  content: string;
  is_primary: boolean;
}

// ── Status enums ────────────────────────────────────────────────────────

export type JobStatus =
  | "pending"
  | "running"
  | "need_confirm"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "awaiting_resume"
  | "paused"
  | "unknown";

export type StepRunStatus =
  | "pending"
  | "running"
  | "succeeded"
  | "failed"
  | "skipped"
  | "blocked"
  | "claimed"
  | "need_confirm";

/** Statuses in which the Job can be cancelled (pending/running/need_confirm). */
export const CANCELABLE_JOB_STATUS: ReadonlySet<JobStatus> = new Set([
  "pending",
  "running",
  "need_confirm",
]);

/** Statuses in which the Job can be retried (only failed). */
export const RETRYABLE_JOB_STATUS: ReadonlySet<JobStatus> = new Set(["failed"]);

// ── Retry history (Job.retry_history jsonb; doc §Retry) ─────────────────

export interface RetryHistoryEntry {
  retry_at: string;
  from_status: JobStatus;
  reason?: string;
  retry_inputs?: JobInput;
}

// ── Job ─────────────────────────────────────────────────────────────────

export interface Job {
  id: string;
  scope: string;
  workflow_id: string;
  workflow_version: number;
  input: JobInput;
  status: JobStatus;
  retry_count: number;
  retry_history: RetryHistoryEntry[];
  submitted_by: string;
  submitted_at: string;
  started_at?: string;
  finished_at?: string;
  failure_reason?: string;
}

// ── StepRun ─────────────────────────────────────────────────────────────

export interface StepRun {
  id: string;
  job_id: string;
  step_key: string;
  step_order: number;
  template_id: string;
  template_version: number;
  status: StepRunStatus;
  retry_count: number;
  /** jsonb: resolved inputs snapshot keyed by Template.Inputs[].name. */
  resolved_inputs: Record<string, unknown>;
  rendered_prompt?: string;
  candidates: Candidate[];
  selected_index: number;
  output_asset_id?: string;
  failure_reason?: string;
  claimed_by?: string;
  claimed_at?: string;
  started_at?: string;
  finished_at?: string;
}

// ── API request/response wrappers ───────────────────────────────────────

/** GET /jobs/:id returns { job, step_runs } per GetJobResp. */
export interface GetJobResponse {
  job: Job;
  step_runs: StepRun[];
}

export interface JobListParams {
  scope?: string;
  workflow_id?: string;
  status?: JobStatus;
  page?: number;
  page_size?: number;
}

/** GET /jobs returns { items, total } per ListJobsResp. */
export interface JobsPage {
  items: Job[];
  total: number;
}

/** POST /jobs body (SubmitJob). workflow_version is optional (server picks
 * latest when omitted). submitted_by is inferred server-side from auth. */
export interface SubmitJobInput {
  workflow_id: string;
  workflow_version?: number;
  input: JobInput;
}

/** POST /jobs/:id/retry body (RetryJob). reason is user-supplied;
 * input_override optionally replaces specific step inputs. */
export interface RetryJobInput {
  reason?: string;
  input_override?: JobInput;
}

// ── WorkflowOption (minimal projection for the JobCreate selector) ───────
// dep-cruiser forbids L2 cross-domain imports (no-L2-cross-domain), so the
// workflow selector reads a minimal workflow projection via the jobs domain
// rather than importing from a (not-yet-existing) workflows domain. Only the
// fields needed to label a <Select> option are fetched.

export interface WorkflowInputSchemaEntry {
  name: string;
  resource_type: "MATERIAL" | "ASSET" | "PRODUCT" | "VIRTUAL_IP";
  asset_kind?: string;
  required: boolean;
}

export interface WorkflowOption {
  id: string;
  name: string;
  scope: string;
  version: number;
  status: number | string;
  input_schema?: { entries?: WorkflowInputSchemaEntry[] };
}

export interface WorkflowsListResponse {
  items: WorkflowOption[];
}
