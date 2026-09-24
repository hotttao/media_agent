/**
 * media_materials Product domain types.
 *
 * Field shapes from docs/ddd/media_materials/domain/0004-product/product.md
 * and idl/media_materials/product.proto. Stable business fields remain
 * first-class columns; metadata is only the extension bag. Like Material,
 * product is a flat row - no Metadata/Spec/Status k8s-style wrapping.
 *
 * product has no user ownership / visibility / grants (scope-level shared
 * resource); created_by / updated_by are audit-only.
 */

// ── Product (business product) ──────────────────────────────────────────

export type ProductStatus = "active" | "archived";

export interface Product {
  id: string;
  organization_id?: string;
  scope: string;
  name: string;
  description?: string;
  target_audience?: string;
  product_details?: string;
  display_actions?: unknown[] | string;
  tags?: string[] | string;
  metadata?: Record<string, unknown> | string;
  external_commerce?: Record<string, unknown> | string;
  images?: Array<{
    id: string;
    object_path: string;
    is_main?: boolean;
    sort_order?: number;
  }>;
  status: ProductStatus;
  created_by: string;
  updated_by?: string;
  created_at: string | number;
  updated_at: string | number;
}

export interface ProductImageInput {
  object_path: string;
  is_main: boolean;
  sort_order: number;
}

/** Create/update payload. status is intentionally absent - it is only
 * mutated via the dedicated Archive endpoint. scope is required on create
 * and immutable on update (enforced server-side). */
export interface ProductInput {
  scope: string;
  name: string;
  description?: string;
  target_audience?: string;
  product_details?: string;
  display_actions?: unknown[];
  tags?: string[];
  external_commerce?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  images?: ProductImageInput[];
}

export interface ProductListParams {
  scope?: string;
  name?: string;
  target_audience?: string;
  tag?: string;
  status?: ProductStatus;
  page?: number;
  page_size?: number;
}

/** Assumed paged response shape - confirm against handler during integration.
 * proto's ListProductsResp only carries items + total (no page/page_size). */
export interface ProductsPage {
  items: Product[];
  total: number;
}
