import { api } from "@/foundation/lib/api-client";
import type { Capability } from "../types";

export async function listCapabilities(): Promise<Capability[]> {
  const items: Capability[] = [];
  let page = 1;
  while (true) {
    const result = await api.request<{ items?: Capability[]; total: number }>(
      `/capabilities?page=${page}&page_size=200`,
    );
    if (!Number.isSafeInteger(result.total) || result.total < 0)
      throw new Error("Invalid capability catalog");
    items.push(...(result.items ?? []));
    if (items.length >= result.total) return items;
    if (
      !result.items?.length ||
      !Number.isSafeInteger(result.total) ||
      result.total < 0
    )
      throw new Error("Invalid capability catalog");
    page++;
  }
}

export function getCapability(
  key: string,
  version: number,
): Promise<Capability> {
  return api.request<Capability>(
    `/capabilities/${encodeURIComponent(key)}/versions/${version}`,
  );
}
