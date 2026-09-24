/**
 * media_materials API client. Wraps foundation/lib/api-client (cookie auth,
 * onUnauthorized). Base path /api/v1 is prepended by the ApiClient.
 *
 * Routes per docs/ddd/media_materials/domain/* + media_materials/biz/router.
 */
import { api } from "@/foundation/lib/api-client";
import type {
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
// Canonical definitions live in @/domains/asset-kinds/lib/api (extracted to
// its own domain). Re-exported here so existing materials imports keep
// working.
export {
  createAssetKind,
  deleteAssetKind,
  getAssetKind,
  listAssetKinds,
  updateAssetKind,
} from "@/domains/asset-kinds/lib/api";

// ── Material ────────────────────────────────────────────────────────────

export async function listMaterials(
  params: MaterialListParams = {},
): Promise<MaterialsPage> {
  return api.request<MaterialsPage>(`/materials${buildQuery(params)}`);
}

export async function getMaterial(id: string): Promise<Material> {
  return api.request<Material>(`/materials/${encodeURIComponent(id)}`);
}

export async function createMaterial(input: MaterialInput): Promise<Material> {
  return api.request<Material>(
    `/materials${buildQuery({ scope: input.scope })}`,
    {
      method: "POST",
      body: JSON.stringify(materialPayload(input)),
    },
  );
}

export async function updateMaterial(
  id: string,
  patch: Partial<MaterialInput>,
): Promise<Material> {
  return api.request<Material>(`/materials/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(materialPayload(patch)),
  });
}

function materialPayload(input: Partial<MaterialInput>) {
  return {
    name: input.name,
    description: input.description ?? "",
    prompt: input.prompt ?? "",
    object_path: input.object_path ?? "",
    content: input.content ?? "",
    metadata: JSON.stringify(input.metadata ?? {}),
    visibility: input.visibility ?? "private",
    team_id: input.team_id ?? "",
    ...(input.asset_kind ? { asset_kind: input.asset_kind } : {}),
  };
}

export async function deleteMaterial(id: string): Promise<void> {
  await api.request<void>(`/materials/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function addMaterialBinding(
  materialId: string,
  binding: MaterialBindingInput,
): Promise<void> {
  await api.request<void>(
    `/materials/${encodeURIComponent(materialId)}/bindings`,
    {
      method: "POST",
      body: JSON.stringify(binding),
    },
  );
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
