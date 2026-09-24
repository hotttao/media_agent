/**
 * media_materials Product API client. Wraps foundation/lib/api-client (cookie
 * auth, onUnauthorized). Base path /api/v1 is prepended by the ApiClient.
 *
 * Routes per docs/ddd/media_materials/domain/0004-product + media_materials/biz/router:
 *   POST   /products            -> Product
 *   GET    /products/:id        -> Product
 *   GET    /products            -> ProductsPage
 *   PUT    /products/:id        -> Product (Partial<ProductInput>)
 *   DELETE /products/:id        -> void
 *   POST   /products/:id/archive -> Product (active -> archived)
 */
import { api } from "@/foundation/lib/api-client";
import type {
  Product,
  ProductInput,
  ProductListParams,
  ProductsPage,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

// ── Product ─────────────────────────────────────────────────────────────

export async function listProducts(
  params: ProductListParams = {},
): Promise<ProductsPage> {
  return api.request<ProductsPage>(`/products${buildQuery(params)}`);
}

export async function getProduct(id: string): Promise<Product> {
  return api.request<Product>(`/products/${encodeURIComponent(id)}`);
}

export async function createProduct(input: ProductInput): Promise<Product> {
  return api.request<Product>(
    `/products${buildQuery({ scope: input.scope })}`,
    {
      method: "POST",
      body: JSON.stringify(productPayload(input)),
    },
  );
}

export async function updateProduct(
  id: string,
  patch: Partial<ProductInput>,
): Promise<Product> {
  return api.request<Product>(`/products/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(productPayload(patch)),
  });
}

function productPayload(input: Partial<ProductInput>) {
  return {
    name: input.name,
    description: input.description ?? "",
    target_audience: input.target_audience ?? "",
    product_details: input.product_details ?? "",
    display_actions: JSON.stringify(input.display_actions ?? []),
    tags: JSON.stringify(input.tags ?? []),
    external_commerce: JSON.stringify(input.external_commerce ?? {}),
    metadata: JSON.stringify(input.metadata ?? {}),
    images: input.images ?? [],
  };
}

export async function deleteProduct(id: string): Promise<void> {
  await api.request<void>(`/products/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function archiveProduct(id: string): Promise<Product> {
  return api.request<Product>(`/products/${encodeURIComponent(id)}/archive`, {
    method: "POST",
  });
}
