/**
 * media_materials domain types.
 *
 * Field shapes from docs/media_materials/domain/* (confirm against
 * idl/media_materials/*.proto during integration - some response wrappers may
 * differ). Metadata/Spec/Status pattern is not forced: materials entities are
 * flat rows, not k8s-style objects.
 */

// ── AssetKind (global registry) ─────────────────────────────────────────

export type AssetKindResourceType =
  | "MATERIAL"
  | "ASSET"
  | "PRODUCT"
  | "VIRTUAL_IP"
  | "TEXT";
export type AssetKindMediaType = "IMAGE" | "TEXT" | "VIDEO";
export type AssetKindStatus = "SYSTEM" | "ENABLED" | "DISABLED";

export interface AssetKind {
  name: string;
  resource_type: AssetKindResourceType;
  media_type: AssetKindMediaType;
  source_table?: string;
  status: AssetKindStatus;
  description?: string;
  created_at: string;
  updated_at: string;
}

export interface AssetKindInput {
  name: string;
  resource_type: AssetKindResourceType;
  media_type: AssetKindMediaType;
  description?: string;
}

// ── Material (raw input asset) ───────────────────────────────────────────

export type Visibility = "private" | "team" | "public";

export interface Material {
  id: string;
  scope: string;
  asset_kind: string;
  name: string;
  url: string;
  content?: string;
  owner_user_id: string;
  visibility: Visibility;
  team_id?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface MaterialInput {
  scope: string;
  asset_kind: string;
  name: string;
  url: string;
  content?: string;
  visibility?: Visibility;
  team_id?: string;
  metadata?: Record<string, unknown>;
}

export interface MaterialListParams {
  scope?: string;
  asset_kind?: string;
  owner_user_id?: string;
  visibility?: Visibility;
  page?: number;
  page_size?: number;
}

/** Assumed paged response shape - confirm against handler during integration. */
export interface MaterialsPage {
  items: Material[];
  total: number;
  page: number;
  page_size: number;
}

// ── Material binding (N:N to VirtualIp / Product) ────────────────────────

export type BindingOwnerKind = "virtual_ip" | "product";

export interface MaterialBindingInput {
  owner_kind: BindingOwnerKind;
  owner_id: string;
}
