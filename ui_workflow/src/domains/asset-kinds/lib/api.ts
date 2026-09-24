/**
 * media_materials.asset_kind API client. Wraps foundation/lib/api-client
 * (cookie auth, onUnauthorized). Base path /api/v1 is prepended by the
 * ApiClient.
 *
 * Routes per idl/media_materials/asset_kind.proto.
 */
import { api } from "@/foundation/lib/api-client";
import type {
  AssetKind,
  AssetKindInput,
  AssetKindsPage,
  AssetKindUpdateInput,
  ListAssetKindsParams,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

export async function listAssetKinds(
  params: ListAssetKindsParams = {},
): Promise<AssetKindsPage> {
  return api.request<AssetKindsPage>(`/asset-kinds${buildQuery(params)}`);
}

export async function getAssetKind(name: string): Promise<AssetKind> {
  return api.request<AssetKind>(`/asset-kinds/${encodeURIComponent(name)}`);
}

export async function createAssetKind(
  input: AssetKindInput,
): Promise<AssetKind> {
  return api.request<AssetKind>("/asset-kinds", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateAssetKind(
  name: string,
  patch: AssetKindUpdateInput,
): Promise<AssetKind> {
  return api.request<AssetKind>(`/asset-kinds/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

export async function deleteAssetKind(name: string): Promise<void> {
  await api.request<void>(`/asset-kinds/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}
