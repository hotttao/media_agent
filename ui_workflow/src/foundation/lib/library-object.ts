import { ApiClientError, api } from "@/foundation/lib/api-client";
import { requestKey } from "@/foundation/lib/request-key";

export type LibraryObjectPurpose =
  | "material"
  | "product_image"
  | "virtual_ip_avatar"
  | "virtual_ip_full_body"
  | "virtual_ip_three_view"
  | "virtual_ip_nine_view";

export interface UploadedLibraryObject {
  object_path: string;
  size_bytes: number;
  content_type: string;
  checksum: string;
}

export async function resolveLibraryObject(
  organizationId: string,
  objectPath?: string,
): Promise<string> {
  if (!objectPath) return "";
  return (await resolveLibraryMedia(organizationId, objectPath)).url;
}

export async function resolveLibraryMedia(
  organizationId: string,
  objectPath: string,
): Promise<{ url: string; content_type?: string }> {
  const query = new URLSearchParams({
    organization_id: organizationId,
    object_path: objectPath,
  });
  return api.request<{ url: string; content_type?: string }>(
    `/library-objects:resolve?${query.toString()}`,
  );
}

export async function uploadLibraryObject(input: {
  organizationId: string;
  scope: string;
  purpose: LibraryObjectPurpose;
  file: File;
  idempotencyKey?: string;
}): Promise<UploadedLibraryObject> {
  const query = new URLSearchParams({
    organization_id: input.organizationId,
    scope: input.scope,
    purpose: input.purpose,
  });
  const form = new FormData();
  form.set("file", input.file);
  const response = await fetch(`/api/v1/library-objects:upload?${query}`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Idempotency-Key": input.idempotencyKey ?? requestKey(),
    },
    body: form,
  });
  if (!response.ok) {
    const text = await response.text();
    throw new ApiClientError(
      text || `Upload failed: ${response.status}`,
      response.status,
      {},
    );
  }
  return response.json() as Promise<UploadedLibraryObject>;
}
