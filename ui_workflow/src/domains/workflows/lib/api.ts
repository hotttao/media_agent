import { api } from "@/foundation/lib/api-client";
import type {
  ValidationResult,
  Visibility,
  Workflow,
  WorkflowInput,
  WorkflowListParams,
  WorkflowsPage,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

export async function listWorkflows(params: WorkflowListParams = {}): Promise<WorkflowsPage> {
  return api.request<WorkflowsPage>(`/workflows${buildQuery(params)}`);
}

export async function getWorkflow(id: string): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}`);
}

export async function createWorkflow(input: WorkflowInput): Promise<Workflow> {
  return api.request<Workflow>("/workflows", { method: "POST", body: JSON.stringify(input) });
}

export async function updateWorkflow(
  id: string,
  patch: Partial<WorkflowInput>,
): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

export async function validateWorkflow(input: WorkflowInput): Promise<ValidationResult> {
  return api.request<ValidationResult>("/workflows/validate", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function archiveWorkflow(id: string): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}/archive`, {
    method: "POST",
  });
}

export async function grantWorkflow(id: string, teamId: string): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}/grants`, {
    method: "POST",
    body: JSON.stringify({ team_id: teamId }),
  });
}

export async function revokeWorkflow(id: string, teamId: string): Promise<Workflow> {
  return api.request<Workflow>(
    `/workflows/${encodeURIComponent(id)}/grants/${encodeURIComponent(teamId)}`,
    { method: "DELETE" },
  );
}

export async function changeWorkflowVisibility(
  id: string,
  visibility: Visibility,
): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}/visibility`, {
    method: "POST",
    body: JSON.stringify({ visibility }),
  });
}
