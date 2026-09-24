/**
 * media_materials VirtualIp API client. Wraps foundation/lib/api-client
 * (cookie auth, onUnauthorized). Base path /api/v1 is prepended by ApiClient.
 *
 * Routes per idl/media_materials/virtual_ip.proto +
 * media_materials/biz/handler/virtual_ip/virtual_ip_service.go.
 */
import { api } from "@/foundation/lib/api-client";
import type {
  ChangeVisibilityInput,
  GrantInput,
  VirtualIp,
  VirtualIpInput,
  VirtualIpListParams,
  VirtualIpsPage,
} from "../types";

function buildQuery(params: object): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

// ── VirtualIp CRUD ────────────────────────────────────────────────────

export async function listVirtualIps(
  params: VirtualIpListParams = {},
): Promise<VirtualIpsPage> {
  return api.request<VirtualIpsPage>(`/virtual-ips${buildQuery(params)}`);
}

export async function getVirtualIp(id: string): Promise<VirtualIp> {
  return api.request<VirtualIp>(`/virtual-ips/${encodeURIComponent(id)}`);
}

export async function createVirtualIp(
  input: VirtualIpInput,
): Promise<VirtualIp> {
  return api.request<VirtualIp>(
    `/virtual-ips${buildQuery({ scope: input.scope })}`,
    {
      method: "POST",
      body: JSON.stringify(virtualIpPayload(input, true)),
    },
  );
}

export async function updateVirtualIp(
  id: string,
  patch: Partial<VirtualIpInput>,
): Promise<VirtualIp> {
  return api.request<VirtualIp>(`/virtual-ips/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(virtualIpPayload(patch, false)),
  });
}

function virtualIpPayload(
  input: Partial<VirtualIpInput>,
  includeVisibility: boolean,
) {
  return {
    name: input.name,
    description: input.description ?? "",
    avatar_object_path: input.avatar_object_path ?? "",
    full_body_object_path: input.full_body_object_path ?? "",
    three_view_object_path: input.three_view_object_path ?? "",
    nine_view_object_path: input.nine_view_object_path ?? "",
    persona_profile: JSON.stringify(input.persona_profile ?? {}),
    ...(includeVisibility ? { visibility: input.visibility ?? "private" } : {}),
  };
}

export async function deleteVirtualIp(id: string): Promise<void> {
  await api.request<void>(`/virtual-ips/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

// ── VirtualIp ACL actions (archive / grant / revoke / visibility) ─────

export async function archiveVirtualIp(id: string): Promise<VirtualIp> {
  return api.request<VirtualIp>(
    `/virtual-ips/${encodeURIComponent(id)}/archive`,
    {
      method: "POST",
    },
  );
}

export async function grantVirtualIp(
  id: string,
  grant: GrantInput,
): Promise<VirtualIp> {
  return api.request<VirtualIp>(
    `/virtual-ips/${encodeURIComponent(id)}/grants`,
    {
      method: "POST",
      body: JSON.stringify(grant),
    },
  );
}

export async function revokeVirtualIp(
  id: string,
  teamId: string,
): Promise<VirtualIp> {
  return api.request<VirtualIp>(
    `/virtual-ips/${encodeURIComponent(id)}/grants/${encodeURIComponent(teamId)}`,
    { method: "DELETE" },
  );
}

export async function changeVirtualIpVisibility(
  id: string,
  input: ChangeVisibilityInput,
): Promise<VirtualIp> {
  return api.request<VirtualIp>(
    `/virtual-ips/${encodeURIComponent(id)}/visibility`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}
