import {
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";
import type { ModelConnectionChoice } from "@/foundation/types/model-connection";

const path = "/api/v1/model-connections";
export async function listModelConnections(): Promise<ModelConnectionChoice[]> {
  return (
    (await managementRequest<{ connections?: ModelConnectionChoice[] }>(path))
      .connections ?? []
  );
}
export function saveModelConnection(
  name: string,
  auth: string,
  existing?: ModelConnectionChoice,
): Promise<ModelConnectionChoice> {
  return managementWrite(
    existing ? `${path}/${encodeURIComponent(existing.id)}` : path,
    existing ? "PUT" : "POST",
    {
      name,
      ...(auth ? { auth_json: auth } : {}),
      ...(existing ? { expected_version: existing.version } : {}),
    },
  );
}
export function revokeModelConnection(
  existing: ModelConnectionChoice,
): Promise<ModelConnectionChoice> {
  return managementWrite(
    `${path}/${encodeURIComponent(existing.id)}/revoke`,
    "POST",
    { expected_version: existing.version },
  );
}
