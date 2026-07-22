/**
 * media_materials API client. Wraps foundation/lib/api-client (cookie auth,
 * onUnauthorized). Base path /api/v1 is prepended by the ApiClient.
 *
 * Routes per docs/media_materials/domain/* + media_materials/biz/router.
 */
import { api } from "@/foundation/lib/api-client";
import type {
  AssetKind,
  AssetKindInput,
  Material,
  MaterialBindingInput,
  MaterialInput,
  MaterialListParams,
  MaterialsPage,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

// ── AssetKind ───────────────────────────────────────────────────────────

export async function listAssetKinds(): Promise<AssetKind[]> {
  return api.request<AssetKind[]>("/asset-kinds");
}

export async function getAssetKind(name: string): Promise<AssetKind> {
  return api.request<AssetKind>(`/asset-kinds/${encodeURIComponent(name)}`);
}

export async function createAssetKind(input: AssetKindInput): Promise<AssetKind> {
  return api.request<AssetKind>("/asset-kinds", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateAssetKind(
  name: string,
  patch: Partial<AssetKindInput>,
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

// ── Material ────────────────────────────────────────────────────────────

export async function listMaterials(params: MaterialListParams = {}): Promise<MaterialsPage> {
  return api.request<MaterialsPage>(`/materials${buildQuery(params)}`);
}

export async function getMaterial(id: string): Promise<Material> {
  return api.request<Material>(`/materials/${encodeURIComponent(id)}`);
}

export async function createMaterial(input: MaterialInput): Promise<Material> {
  return api.request<Material>("/materials", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateMaterial(id: string, patch: Partial<MaterialInput>): Promise<Material> {
  return api.request<Material>(`/materials/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(patch),
  });
}

export async function deleteMaterial(id: string): Promise<void> {
  await api.request<void>(`/materials/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function addMaterialBinding(
  materialId: string,
  binding: MaterialBindingInput,
): Promise<void> {
  await api.request<void>(`/materials/${encodeURIComponent(materialId)}/bindings`, {
    method: "POST",
    body: JSON.stringify(binding),
  });
}

export async function removeMaterialBinding(
  materialId: string,
  ownerKind: string,
  ownerId: string,
): Promise<void> {
  await api.request<void>(
    `/materials/${encodeURIComponent(materialId)}/bindings/${encodeURIComponent(ownerKind)}/${encodeURIComponent(ownerId)}`,
    { method: "DELETE" },
  );
}
