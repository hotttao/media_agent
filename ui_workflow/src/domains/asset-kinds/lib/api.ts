/**
 * media_materials.asset_kind API client. Wraps foundation/lib/api-client
 * (cookie auth, onUnauthorized). Base path /api/v1 is prepended by the
 * ApiClient.
 *
 * Routes per idl/media_materials/asset_kind.proto.
 */
import { api } from "@/foundation/lib/api-client";
import type {
  AssetKindResourceType,
  AssetKindMediaType,
  AssetKindStatus,
  AssetKind,
  AssetKindInput,
  AssetKindsPage,
  AssetKindUpdateInput,
  ListAssetKindsParams,
} from "../types";

const resourceTypes: Record<AssetKindResourceType, number> = {
  MATERIAL: 1,
  ASSET: 2,
  PRODUCT: 3,
  VIRTUAL_IP: 4,
  TEXT: 5,
};
const mediaTypes: Record<AssetKindMediaType, number> = {
  IMAGE: 1,
  TEXT: 2,
  VIDEO: 3,
};
const statuses: Record<AssetKindStatus, number> = {
  SYSTEM: 1,
  ENABLED: 2,
  DISABLED: 3,
};
type WireAssetKind = Omit<
  AssetKind,
  "resource_type" | "media_type" | "status"
> & {
  resource_type: string | number;
  media_type: string | number;
  status: string | number;
};
function enumName<T extends string>(
  values: Record<T, number>,
  value: string | number,
): T {
  const entry = Object.entries(values).find(
    ([name, number]) => name === value || String(number) === String(value),
  );
  if (!entry) throw new Error(`Unsupported asset kind enum: ${String(value)}`);
  return entry[0] as T;
}
function fromWire(item: WireAssetKind): AssetKind {
  return {
    ...item,
    resource_type: enumName(resourceTypes, item.resource_type),
    media_type: enumName(mediaTypes, item.media_type),
    status: enumName(statuses, item.status),
  };
}

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
  const page = await api.request<{ items?: WireAssetKind[]; total?: number }>(
    `/asset-kinds${buildQuery(params)}`,
  );
  return { items: (page.items || []).map(fromWire), total: page.total || 0 };
}

export async function getAssetKind(name: string): Promise<AssetKind> {
  return fromWire(
    await api.request<WireAssetKind>(
      `/asset-kinds/${encodeURIComponent(name)}`,
    ),
  );
}

export async function createAssetKind(
  input: AssetKindInput,
): Promise<AssetKind> {
  return fromWire(
    await api.request<WireAssetKind>("/asset-kinds", {
      method: "POST",
      body: JSON.stringify({
        ...input,
        resource_type: resourceTypes[input.resource_type],
        media_type: mediaTypes[input.media_type],
      }),
    }),
  );
}

export async function updateAssetKind(
  name: string,
  patch: AssetKindUpdateInput,
): Promise<AssetKind> {
  return fromWire(
    await api.request<WireAssetKind>(
      `/asset-kinds/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        body: JSON.stringify(patch),
      },
    ),
  );
}

export async function deleteAssetKind(name: string): Promise<void> {
  await api.request<void>(`/asset-kinds/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}
