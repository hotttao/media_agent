/**
 * media_materials Product API client. Wraps foundation/lib/api-client (cookie
 * auth, onUnauthorized). Base path /api/v1 is prepended by the ApiClient.
 *
 * Routes per docs/media_materials/domain/0004-product + media_materials/biz/router:
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

export async function listProducts(params: ProductListParams = {}): Promise<ProductsPage> {
  return api.request<ProductsPage>(`/products${buildQuery(params)}`);
}

export async function getProduct(id: string): Promise<Product> {
  return api.request<Product>(`/products/${encodeURIComponent(id)}`);
}

export async function createProduct(input: ProductInput): Promise<Product> {
  return api.request<Product>("/products", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateProduct(
  id: string,
  patch: Partial<ProductInput>,
): Promise<Product> {
  return api.request<Product>(`/products/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

export async function deleteProduct(id: string): Promise<void> {
  await api.request<void>(`/products/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function archiveProduct(id: string): Promise<Product> {
  return api.request<Product>(`/products/${encodeURIComponent(id)}/archive`, {
    method: "POST",
  });
}
