/**
 * media_materials.asset_kind domain types.
 *
 * Field shapes from idl/media_materials/asset_kind.proto +
 * idl/common/media_type.proto. Flat row (no Metadata/Spec/Status wrapping):
 * asset_kind is a global registry, not a k8s-style object.
 *
 * created_at / updated_at are int64 in proto; serialized as string over JSON
 * to avoid JS number precision loss (matches materials convention).
 */

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

/** RegisterAssetKindReq body (proto: name + resource_type + media_type +
 * source_table + description). source_table is server-derived from
 * resource_type, so the form omits it. */
export interface AssetKindInput {
  name: string;
  resource_type: AssetKindResourceType;
  media_type: AssetKindMediaType;
  description?: string;
  status?: AssetKindStatus;
}

export interface AssetKindUpdateInput {
  description?: string;
  status: AssetKindStatus;
}

export interface AssetKindsPage {
  items: AssetKind[];
  total: number;
}

/** ListAssetKindsReq query params. */
export interface ListAssetKindsParams {
  resource_type?: AssetKindResourceType;
  media_type?: AssetKindMediaType;
  status?: AssetKindStatus;
  page?: number;
  page_size?: number;
}
