/**
 * media_materials/generated_asset domain types.
 *
 * GeneratedAsset is read-only on the frontend: created by media_workflow
 * execution via the internal `UpsertGeneratedAsset` RPC; UI only list/show/delete.
 *
 * Field shapes from idl/media_materials/generated_asset.proto. `updated_at` is
 * The current response exposes created_at but not updated_at; this frontend
 * follows the HTTP contract rather than assuming every database column is
 * serialized.
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
  object_path?: string;
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
