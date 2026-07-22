/**
 * media_materials/generated_asset API client. Wraps foundation/lib/api-client
 * (cookie auth, onUnauthorized). Base path /api/v1 is prepended by the ApiClient.
 *
 * Routes per idl/media_materials/generated_asset.proto. GeneratedAsset is
 * read-only on the frontend (created by media_workflow execution via the
 * internal `UpsertGeneratedAsset` RPC), so only list/get/delete are exposed.
 */
import { api } from "@/foundation/lib/api-client";
import type {
  GeneratedAsset,
  GeneratedAssetListParams,
  GeneratedAssetsPage,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

// ── GeneratedAsset (read-only + delete) ─────────────────────────────────

export async function listGeneratedAssets(
  params: GeneratedAssetListParams = {},
): Promise<GeneratedAssetsPage> {
  return api.request<GeneratedAssetsPage>(
    `/generated-assets${buildQuery(params)}`,
  );
}

export async function getGeneratedAsset(id: string): Promise<GeneratedAsset> {
  return api.request<GeneratedAsset>(
    `/generated-assets/${encodeURIComponent(id)}`,
  );
}

export async function deleteGeneratedAsset(id: string): Promise<void> {
  await api.request<void>(`/generated-assets/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
