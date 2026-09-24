/**
 * media_materials domain types.
 *
 * Field shapes from docs/ddd/media_materials/domain/* (confirm against
 * idl/media_materials/*.proto during integration - some response wrappers may
 * differ). Metadata/Spec/Status pattern is not forced: materials entities are
 * flat rows, not k8s-style objects.
 */

// ── AssetKind (global registry) ─────────────────────────────────────────
// Canonical definitions live in @/domains/asset-kinds/types (extracted to its
// own domain). Re-exported here so existing materials imports keep working.
export type {
  AssetKind,
  AssetKindInput,
  AssetKindMediaType,
  AssetKindResourceType,
  AssetKindStatus,
} from "@/domains/asset-kinds/types";

// ── Material (raw input asset) ───────────────────────────────────────────

export type Visibility = "private" | "team" | "public";

export interface Material {
  id: string;
  organization_id?: string;
  scope: string;
  asset_kind: string;
  name: string;
  object_path?: string;
  description?: string;
  prompt?: string;
  content?: string;
  owner_user_id: string;
  visibility: Visibility;
  team_id?: string;
  metadata?: Record<string, unknown> | string;
  created_at: string | number;
  updated_at: string | number;
}

export interface MaterialInput {
  scope: string;
  asset_kind: string;
  name: string;
  description?: string;
  prompt?: string;
  object_path?: string;
  content?: string;
  visibility?: Visibility;
  team_id?: string;
  metadata?: Record<string, unknown>;
}

export interface MaterialListParams {
  scope?: string;
  asset_kind?: string;
  only_mine?: boolean;
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
