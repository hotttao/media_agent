/**
 * media_workflow.template domain types.
 *
 * Field shapes from idl/media_workflow/template.proto +
 * docs/ddd/media_workflow/domain/0001-template/template.md. Flat row (no
 * Metadata/Spec/Status wrapping): template is a versioned prompt config, not a
 * k8s-style object.
 *
 * created_at / updated_at are int64 (UnixNano) in proto; typed as string here
 * to avoid JS number precision loss (matches materials convention). status is
 * a proto enum (DRAFT/PUBLISHED/ARCHIVED) but the domain layer uses lowercase
 * strings, so we align on lowercase here. visibility is a plain proto string
 * field (private/team/public).
 *
 * Slot's resource_type field name follows the proto wire format
 * (TemplateInputSlot.resource_type); the docs' `kind` column is the DB column
 * name, not the JSON field.
 */

// ── Enums ───────────────────────────────────────────────────────────────

export type TemplateStatus = "draft" | "published" | "archived" | "unknown";

export type Visibility = "private" | "team" | "public";

/** Slot resource_type - mirrors idl/common/media_type.proto ResourceType.
 * Kept as a string union (not re-exported from asset-kinds) so the templates
 * domain is self-contained. */
export type TemplateSlotResourceType =
  | "MATERIAL"
  | "ASSET"
  | "PRODUCT"
  | "VIRTUAL_IP"
  | "TEXT";

// ── Slot (input slot) ───────────────────────────────────────────────────

export interface TemplateSlot {
  name: string;
  asset_kind: string;
  resource_type: string;
  required: boolean;
  display_order?: number;
}

export interface TemplateSlotInput {
  name: string;
  asset_kind: string;
  resource_type: string;
  required: boolean;
  display_order?: number;
}

export type InvocationOperation = "render_template" | "asset_ref" | "list";

export interface InvocationExpression {
  op: InvocationOperation;
  slot?: string;
  items?: InvocationExpression[];
}

// ── Template ────────────────────────────────────────────────────────────

export interface Template {
  my_permission?: "owner" | "editor" | "viewer";
  allowed_actions?: string[];
  organization_id?: string;
  id: string;
  scope: string;
  name: string;
  description?: string;
  prompt_body: string;
  inputs: TemplateSlot[];
  output_asset_kind: string;
  platform: string;
  gen_way: string;
  gen_type?: string;
  status: TemplateStatus;
  visibility: Visibility;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
  needs_confirm: boolean;
  candidate_count: number;
  capability_key: string;
  capability_version: number;
  invocation_mapping: Record<string, InvocationExpression>;
}

/** Create/update payload. scope is required on create and immutable on update
 * (enforced server-side). status / version are mutated via dedicated
 * endpoints, not the update body. visibility is accepted by the form for UX
 * compatibility but draft forms display it read-only: Create/Update do not
 * carry it. The show page owns the separate owner-only visibility command. */
export interface TemplateInput {
  organization_id?: string;
  scope: string;
  name: string;
  description?: string;
  prompt_body: string;
  inputs: TemplateSlotInput[];
  output_asset_kind: string;
  platform: string;
  gen_way: string;
  gen_type?: string;
  needs_confirm?: boolean;
  candidate_count?: number;
  capability_key: string;
  capability_version: number;
  invocation_mapping: Record<string, InvocationExpression>;
  visibility?: Visibility;
}

export interface TemplateListParams {
  scope?: string;
  name?: string;
  visibility?: Visibility;
  page?: number;
  page_size?: number;
}

/** Paged response shape - proto's ListTemplatesResp carries items + total
 * (no page/page_size). */
export interface TemplatesPage {
  items: Template[];
  total: number;
}

/** Create response - proto's CreateTemplateResp { id, version }. The full
 * Template can be fetched via getTemplate(id) if needed. */
export interface CreateTemplateResponse {
  id: string;
  version: number;
}

/** Update response - proto's UpdateTemplateResp { version }. */
export interface UpdateTemplateResponse {
  version: number;
}

/** Grant request body - proto's GrantReq { id (path), team_id (body),
 * permission (body, optional), granted_by (query) }. permission defaults
 * server-side; granted_by is taken from the auth cookie server-side. */
export interface GrantTemplateInput {
  team_id: string;
  permission?: "viewer" | "editor";
}

/** Change visibility request body - proto's ChangeVisibilityReq
 * { id (path), visibility (body) }. */
export interface ChangeVisibilityInput {
  visibility: Visibility;
}
