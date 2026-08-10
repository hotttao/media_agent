/**
 * media_materials/generated_asset domain types.
 *
 * GeneratedAsset is read-only on the frontend: created by media_workflow
 * execution via the internal `UpsertGeneratedAsset` RPC; UI only list/show/delete.
 *
 * Field shapes from idl/media_materials/generated_asset.proto. `updated_at` is
 * present in the generated_assets table (see docs/ddd/media_materials/domain/0002-asset/asset.md)
 * and is exposed by the API even though the proto message omits it; typed as
 * string (int64 serialized as string to preserve precision, matching the
 * Material convention).
 */

// Visibility is structurally identical to materials' Visibility; duplicated
// here because dep-cruiser forbids L2 cross-domain imports
// (no-L2-cross-domain in .dependency-cruiser.cjs).
export type Visibility = "private" | "team" | "public";

export interface GeneratedAsset {
  id: string;
  scope: string;
  asset_kind: string;
  name: string;
  url: string;
  content?: string;
  owner_user_id: string;
  visibility: Visibility;
  team_id?: string;
  // ── generation provenance (set by workflow execution, immutable) ──
  job_id: string;
  step_run_id: string;
  template_id: string;
  template_version: number;
  source_virtual_ip_id?: string;
  source_product_id?: string;
  created_at: string;
  updated_at: string;
}

/** Query params for GET /generated-assets (see ListGeneratedAssetsReq). */
export interface GeneratedAssetListParams {
  scope?: string;
  asset_kind?: string;
  owner?: string;
  job_id?: string;
  source_virtual_ip_id?: string;
  source_product_id?: string;
  page?: number;
  page_size?: number;
}

/** Paged response shape per ListGeneratedAssetsResp. */
export interface GeneratedAssetsPage {
  items: GeneratedAsset[];
  total: number;
}
