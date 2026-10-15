import type {
  CatalogKind,
  CatalogResource,
  ContentVersion,
  MCPDefinition,
  SkillFile,
} from "@/domains/agent-catalog/types";
import {
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";
export function catalogPath(kind: CatalogKind, id = ""): string {
  return `/api/v1/${kind === "mcp" ? "mcp-definitions" : kind}${id ? `/${encodeURIComponent(id)}` : ""}`;
}
export async function listCatalog(
  kind: "prompts" | "skills",
  organization: string,
): Promise<CatalogResource[]> {
  const body = await managementRequest<Record<string, CatalogResource[]>>(
    `${catalogPath(kind)}?organization_id=${encodeURIComponent(organization)}`,
  );
  return body[kind] ?? [];
}
export async function listDefinitions(
  organization: string,
): Promise<MCPDefinition[]> {
  const results: MCPDefinition[] = [];
  let after = "";
  do {
    const body = await managementRequest<{
      definitions?: MCPDefinition[];
      next_after_id?: string;
    }>(
      `${catalogPath("mcp")}?organization_id=${encodeURIComponent(organization)}&limit=100&after_id=${encodeURIComponent(after)}`,
    );
    results.push(...(body.definitions ?? []));
    if (!body.next_after_id || body.next_after_id === after) break;
    after = body.next_after_id;
  } while (results.length < 2000);
  return results;
}
export async function versions(
  kind: "prompts" | "skills",
  id: string,
): Promise<ContentVersion[]> {
  const body = await managementRequest<{ versions?: ContentVersion[] }>(
    `${catalogPath(kind, id)}/versions`,
  );
  return body.versions ?? [];
}
export async function skillFiles(
  id: string,
  version = 0,
): Promise<SkillFile[]> {
  const body = await managementRequest<{ files?: SkillFile[] }>(
    `${catalogPath("skills", id)}/files?version=${version}`,
  );
  return body.files ?? [];
}
export function skillFile(
  id: string,
  path: string,
  version = 0,
): Promise<SkillFile> {
  return managementRequest(
    `${catalogPath("skills", id)}/file?${new URLSearchParams({ path, version: String(version) })}`,
  );
}
export function bytesToBase64(bytes: Uint8Array): string {
  let value = "";
  for (let start = 0; start < bytes.length; start += 8192)
    value += String.fromCharCode(...bytes.subarray(start, start + 8192));
  return btoa(value);
}
export function textToBase64(value: string): string {
  return bytesToBase64(new TextEncoder().encode(value));
}
export function base64Bytes(value: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}
export function base64Text(value: string): string {
  return new TextDecoder().decode(base64Bytes(value));
}
export function downloadBase64(content: string, filename: string): void {
  const url = URL.createObjectURL(new Blob([base64Bytes(content)]));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export async function downloadSkill(
  id: string,
  version: number,
): Promise<void> {
  const result = await managementRequest<{ content: string; filename: string }>(
    `${catalogPath("skills", id)}/versions/${version}/package`,
  );
  downloadBase64(result.content, result.filename);
}
export function saveResource(
  kind: "prompts" | "skills",
  resource: CatalogResource,
  content: Record<string, unknown>,
): Promise<CatalogResource> {
  return managementWrite(catalogPath(kind, resource.id), "PUT", {
    ...content,
    expected_version: resource.version,
  });
}
