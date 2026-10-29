import type { ApiToken, TokenDraft } from "@/domains/api-token/types";
import {
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";

const path = "/api/api-tokens";
export async function listTokens(): Promise<ApiToken[]> {
  return (
    (await managementRequest<{ api_tokens?: ApiToken[] }>(path)).api_tokens ??
    []
  );
}
export function createToken(
  draft: TokenDraft,
): Promise<{ api_token: ApiToken; secret: string }> {
  return managementWrite(path, "POST", draft);
}
export function revokeToken(id: string): Promise<{ success: boolean }> {
  return managementWrite(
    `${path}/${encodeURIComponent(id)}/revoke`,
    "POST",
    {},
  );
}
