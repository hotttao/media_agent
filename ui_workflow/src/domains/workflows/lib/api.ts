import { api } from "@/foundation/lib/api-client";
import type {
  SlotBinding,
  ValidationResult,
  Visibility,
  Workflow,
  WorkflowInput,
  WorkflowListParams,
  WorkflowStep,
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
    ...(input.steps
      ? {
          steps: steps.map((step) => ({
            ...step,
            slot_bindings: Object.fromEntries(
              (step.slot_bindings ?? []).map((binding) => [
                binding.to_input,
                {
                  source:
                    binding.source === "workflow_input"
                      ? "WORKFLOW_INPUT"
                      : binding.source === "step_output"
                        ? "STEP_OUTPUT"
                        : "CONST",
                  ref: binding.ref ?? "",
                  const_value: binding.const_value ?? "",
                },
              ]),
            ),
          })),
        }
      : {}),
  };
}

function fromWire(workflow: Workflow): Workflow {
  const wireStatus = String(workflow.status ?? "").toLowerCase();
  const numericStatus = Number(workflow.status);
  return {
    ...workflow,
    status:
      numericStatus === 2 || wireStatus.includes("published")
        ? "published"
        : numericStatus === 3 || wireStatus.includes("archived")
          ? "archived"
          : "draft",
    steps: (workflow.steps ?? []).map((step: WorkflowStep) => {
      const rawBindings = step.slot_bindings as unknown as
        | SlotBinding[]
        | Record<
            string,
            { source?: unknown; ref?: string; const_value?: unknown }
          >;
      return {
        ...step,
        slot_bindings: Array.isArray(rawBindings)
          ? rawBindings
          : (
              Object.entries(rawBindings ?? {}) as [
                string,
                { source?: unknown; ref?: string; const_value?: unknown },
              ][]
            ).map(([to_input, binding]) => ({
              to_input,
              source:
                binding.source === "STEP_OUTPUT" ||
                binding.source === "SOURCE_STEP_OUTPUT" ||
                binding.source === "2" ||
                binding.source === 2
                  ? "step_output"
                  : binding.source === "CONST" ||
                      binding.source === "SOURCE_CONST" ||
                      binding.source === "3" ||
                      binding.source === 3
                    ? "const"
                    : "workflow_input",
              ref: binding.ref,
              const_value: binding.const_value,
            })),
      };
    }),
  };
}

export async function listWorkflows(
  params: WorkflowListParams = {},
): Promise<WorkflowsPage> {
  const page = await api.request<WorkflowsPage>(
    `/workflows${buildQuery(params)}`,
  );
  return { ...page, items: (page.items ?? []).map(fromWire) };
}

export async function getWorkflow(id: string): Promise<Workflow> {
  return fromWire(
    await api.request<Workflow>(`/workflows/${encodeURIComponent(id)}`),
  );
}

export async function createWorkflow(input: WorkflowInput): Promise<Workflow> {
  return fromWire(
    await api.request<Workflow>("/workflows", {
      method: "POST",
      body: JSON.stringify(toWire(input)),
    }),
  );
}

export async function updateWorkflow(
  id: string,
  patch: Partial<WorkflowInput>,
): Promise<Workflow> {
  return fromWire(
    await api.request<Workflow>(`/workflows/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(toWire(patch)),
    }),
  );
}

export async function validateWorkflow(
  input: WorkflowInput,
): Promise<ValidationResult> {
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

export async function publishWorkflow(id: string): Promise<Workflow> {
  return fromWire(
    await api.request<Workflow>(
      `/workflows/${encodeURIComponent(id)}/publish`,
      {
        method: "POST",
      },
    ),
  );
}

export async function grantWorkflow(
  id: string,
  teamId: string,
): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}/grants`, {
    method: "POST",
    body: JSON.stringify({ team_id: teamId }),
  });
}

export async function revokeWorkflow(
  id: string,
  teamId: string,
): Promise<Workflow> {
  return api.request<Workflow>(
    `/workflows/${encodeURIComponent(id)}/grants/${encodeURIComponent(teamId)}`,
    { method: "DELETE" },
  );
}

export async function changeWorkflowVisibility(
  id: string,
  visibility: Visibility,
): Promise<Workflow> {
  return api.request<Workflow>(
    `/workflows/${encodeURIComponent(id)}/visibility`,
    {
      method: "POST",
      body: JSON.stringify({ visibility }),
    },
  );
}
