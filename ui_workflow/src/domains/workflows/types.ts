/** media_workflow Workflow types. Fields per idl/media_workflow/*.proto. */

export type WorkflowStatus = "draft" | "published" | "archived";
export type Visibility = "private" | "team" | "public";
export type StepBindingSource = "workflow_input" | "step_output" | "const";

export interface StepBinding {
  source: StepBindingSource;
  to_input: string;
  ref?: string;
  const_value?: unknown;
}

export interface WorkflowStep {
  key: string;
  template_id: string;
  template_version?: number;
  bindings: StepBinding[];
  approval_policy?: "when_multiple_candidates" | "always";
  reuse_policy?: "reuse_if_available" | "force_new";
}

export interface WorkflowInput {
  scope: string;
  name: string;
  description?: string;
  output_step_key: string;
  steps: WorkflowStep[];
  visibility?: Visibility;
}

export interface Workflow {
  id: string;
  scope: string;
  name: string;
  description?: string;
  output_step_key: string;
  steps: WorkflowStep[];
  status: WorkflowStatus;
  visibility: Visibility;
  version: number;
  input_schema?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface WorkflowListParams {
  scope?: string;
  status?: WorkflowStatus;
  page?: number;
  page_size?: number;
}

export interface WorkflowsPage {
  items: Workflow[];
  total: number;
  page: number;
  page_size: number;
}

export interface ValidationEdge {
  from: string;
  to: string;
}

export interface ValidationError {
  step_key?: string;
  message: string;
}

export interface ValidationResult {
  roots?: string[];
  edges?: ValidationEdge[];
  errors: ValidationError[];
}
