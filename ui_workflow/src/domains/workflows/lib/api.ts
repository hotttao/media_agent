import { api } from "@/foundation/lib/api-client";
import { wireStatus } from "@/foundation/lib/wire-status";
import type { ResourceGrant } from "@/foundation/types/resource-access";
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
                      ? 1
                      : binding.source === "step_output"
                        ? 2
                        : 3,
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
  return {
    ...workflow,
    status: wireStatus(
      workflow.status,
      [
        "unknown",
        "draft",
        "published",
        "archived",
        "publishing",
        "publish_failed",
      ],
      ["wf_", "workflow_status_"],
    ),
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

export async function listWorkflowGrants(id: string): Promise<ResourceGrant[]> {
  const result = await api.request<{items?: ResourceGrant[]}>(`/workflows/${encodeURIComponent(id)}/grants`);
  return result.items ?? [];
}

export async function getWorkflowVersion(
  id: string,
  version: number,
): Promise<Workflow> {
  if (!Number.isSafeInteger(version) || version <= 0)
    throw new Error("Invalid workflow version");
  return fromWire(
    await api.request<Workflow>(
      `/workflows/${encodeURIComponent(id)}/versions/${version}`,
    ),
  );
}

export async function createWorkflow(
  input: WorkflowInput,
): Promise<{ id: string; version: number }> {
  return api.request<{ id: string; version: number }>(
    `/workflows${buildQuery({ scope: input.scope })}`,
    {
      method: "POST",
      body: JSON.stringify(toWire(input)),
    },
  );
}

export async function updateWorkflow(
  id: string,
  patch: Partial<WorkflowInput>,
): Promise<{ version: number }> {
  return api.request<{ version: number }>(
    `/workflows/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      body: JSON.stringify(toWire(patch)),
    },
  );
}

export async function validateWorkflow(
  input: WorkflowInput,
): Promise<ValidationResult> {
  const response = await api.request<
    ValidationResult & {
      valid?: boolean;
      derived_edges?: ValidationResult["edges"];
      reject_reasons?: { subject?: string; message: string }[];
    }
  >("/workflows/validate", {
    method: "POST",
    body: JSON.stringify(toWire(input)),
  });
  const errors =
    response.reject_reasons?.map((reason) => ({
      step_key: reason.subject,
      message: reason.message,
    })) ??
    response.errors ??
    [];
  return {
    ...response,
    edges: response.derived_edges ?? response.edges ?? [],
    errors: errors.length
      ? errors
      : response.valid === false ||
          (response.valid !== true &&
            !response.errors &&
            !response.reject_reasons)
        ? [
            {
              message: "Workflow validation did not return a successful result",
            },
          ]
        : [],
  };
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
  permission: "viewer" | "editor" = "viewer",
): Promise<Workflow> {
  return api.request<Workflow>(`/workflows/${encodeURIComponent(id)}/grants`, {
    method: "POST",
    body: JSON.stringify({ team_id: teamId, permission }),
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
