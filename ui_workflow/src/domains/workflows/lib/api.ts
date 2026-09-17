import { api } from "@/foundation/lib/api-client";
import type {
  ValidationResult,
  Visibility,
  Workflow,
  WorkflowInput,
  WorkflowStep,
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

function toWire(input: Partial<WorkflowInput>): Record<string, unknown> {
  const steps = input.steps ?? [];
  return {
    ...input,
    ...(input.steps ? { steps: steps.map((step) => ({
      ...step,
      input_bindings: Object.fromEntries(
        (step.bindings ?? []).map((binding) => [binding.to_input, {
          source: binding.source === "workflow_input" ? "WORKFLOW_INPUT" : binding.source === "step_output" ? "STEP_OUTPUT" : "CONST",
          ref: binding.ref ?? "",
          const_value: binding.const_value ?? "",
        }]),
      ),
      bindings: undefined,
    })) } : {}),
  };
}

function fromWire(workflow: Workflow): Workflow {
  const wireStatus = String(workflow.status ?? "").toLowerCase();
  return {
    ...workflow,
    status: wireStatus.includes("published") ? "published" : wireStatus.includes("archived") ? "archived" : "draft",
    steps: (workflow.steps ?? []).map((step: WorkflowStep & { input_bindings?: Record<string, { source?: string; ref?: string; const_value?: unknown }> }) => ({
      ...step,
      bindings: step.bindings?.length ? step.bindings : (Object.entries(step.input_bindings ?? {}) as [string, { source?: string; ref?: string; const_value?: unknown }][]).map(([to_input, binding]) => ({
        to_input,
        source: binding.source === "STEP_OUTPUT" ? "step_output" : binding.source === "CONST" ? "const" : "workflow_input",
        ref: binding.ref,
        const_value: binding.const_value,
      })),
    })),
  };
}

export async function listWorkflows(params: WorkflowListParams = {}): Promise<WorkflowsPage> {
  const page = await api.request<WorkflowsPage>(`/workflows${buildQuery(params)}`);
  return { ...page, items: page.items.map(fromWire) };
}

export async function getWorkflow(id: string): Promise<Workflow> {
  return fromWire(await api.request<Workflow>(`/workflows/${encodeURIComponent(id)}`));
}

export async function createWorkflow(input: WorkflowInput): Promise<Workflow> {
  return fromWire(await api.request<Workflow>("/workflows", { method: "POST", body: JSON.stringify(toWire(input)) }));
}

export async function updateWorkflow(
  id: string,
  patch: Partial<WorkflowInput>,
): Promise<Workflow> {
  return fromWire(await api.request<Workflow>(`/workflows/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(toWire(patch)),
  }));
}

export async function validateWorkflow(input: WorkflowInput): Promise<ValidationResult> {
  return api.request<ValidationResult>("/workflows/validate", {
    method: "POST",
    body: JSON.stringify(toWire(input)),
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
