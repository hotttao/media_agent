/**
 * media_materials Product domain types.
 *
 * Field shapes from docs/media_materials/domain/0004-product/product.md
 * (idl/media_materials/product.proto is stale - it still has flat
 * target_audience / product_details / display_actions strings; per the
 * 2026-07-03 change log those were consolidated into a single `metadata`
 * jsonb to avoid schema churn). Like Material, product is a flat row - no
 * Metadata/Spec/Status k8s-style wrapping.
 *
 * product has no user ownership / visibility / grants (scope-level shared
 * resource); created_by / updated_by are audit-only.
 */

// ── Product (business product) ──────────────────────────────────────────

export type ProductStatus = "active" | "archived";

export interface Product {
  id: string;
  scope: string;
  name: string;
  description?: string;
  metadata?: Record<string, unknown>;
  status: ProductStatus;
  created_by: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

/** Create/update payload. status is intentionally absent - it is only
 * mutated via the dedicated Archive endpoint. scope is required on create
 * and immutable on update (enforced server-side). */
export interface ProductInput {
  scope: string;
  name: string;
  description?: string;
  metadata?: Record<string, unknown>;
}

export interface ProductListParams {
  scope?: string;
  name?: string;
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
