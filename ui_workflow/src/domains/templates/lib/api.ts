/**
 * media_workflow.template API client. Wraps foundation/lib/api-client (cookie
 * auth, onUnauthorized). Base path /api/v1 is prepended by the ApiClient.
 *
 * Routes per idl/media_workflow/template.proto +
 * media_workflow/biz/router/template:
 *   POST   /templates                       -> CreateTemplateResponse
 *   GET    /templates/:id                   -> Template
 *   GET    /templates                       -> TemplatesPage
 *   PUT    /templates/:id                   -> UpdateTemplateResponse
 *   POST   /templates/:id/archive           -> Template
 *   POST   /templates/:id/grants            -> Template
 *   DELETE /templates/:id/grants/:team_id   -> Template
 *   POST   /templates/:id/visibility        -> Template
 *   GET    /templates/:id/versions/:version -> Template (historical snapshot)
 *
 * Note: create/update return {id,version}/{version} per proto (not the full
 * Template). Callers refetch via getTemplate/listTemplates as needed.
 */
import { api } from "@/foundation/lib/api-client";
import { wireStatus } from "@/foundation/lib/wire-status";
import type {
  ChangeVisibilityInput,
  CreateTemplateResponse,
  GrantTemplateInput,
  Template,
  TemplateInput,
  TemplateListParams,
  TemplatesPage,
  UpdateTemplateResponse,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

// ── Template ────────────────────────────────────────────────────────────

export async function listTemplates(
  params: TemplateListParams = {},
): Promise<TemplatesPage> {
  const page = await api.request<TemplatesPage>(
    `/templates${buildQuery(params)}`,
  );
  return { ...page, items: (page.items ?? []).map(fromWire) };
}

export async function getTemplate(id: string): Promise<Template> {
  return fromWire(
    await api.request<Template>(`/templates/${encodeURIComponent(id)}`),
  );
}

function fromWire(template: Template): Template {
  return {
    ...template,
    status: wireStatus(
      template.status,
      ["unknown", "draft", "published", "archived"],
      ["tpl_", "template_status_"],
    ),
    inputs: template.inputs ?? [],
    invocation_mapping: template.invocation_mapping ?? {},
  };
}

export async function createTemplate(
  input: TemplateInput,
): Promise<CreateTemplateResponse> {
  return api.request<CreateTemplateResponse>(
    `/templates${buildQuery({ scope: input.scope })}`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}

export async function updateTemplate(
  id: string,
  patch: Partial<TemplateInput>,
): Promise<UpdateTemplateResponse> {
  return api.request<UpdateTemplateResponse>(
    `/templates/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      body: JSON.stringify(patch),
    },
  );
}

export async function archiveTemplate(id: string): Promise<Template> {
  return api.request<Template>(`/templates/${encodeURIComponent(id)}/archive`, {
    method: "POST",
  });
}

export async function publishTemplate(id: string): Promise<Template> {
  return fromWire(
    await api.request<Template>(
      `/templates/${encodeURIComponent(id)}/publish`,
      { method: "POST" },
    ),
  );
}

export async function grantTemplate(
  id: string,
  input: GrantTemplateInput,
): Promise<Template> {
  return api.request<Template>(`/templates/${encodeURIComponent(id)}/grants`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function revokeTemplate(
  id: string,
  teamId: string,
): Promise<Template> {
  return api.request<Template>(
    `/templates/${encodeURIComponent(id)}/grants/${encodeURIComponent(teamId)}`,
    { method: "DELETE" },
  );
}

export async function changeTemplateVisibility(
  id: string,
  input: ChangeVisibilityInput,
): Promise<Template> {
  return api.request<Template>(
    `/templates/${encodeURIComponent(id)}/visibility`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}

export async function getTemplateVersion(
  id: string,
  version: number,
): Promise<Template> {
  return fromWire(
    await api.request<Template>(
      `/templates/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}`,
    ),
  );
}
