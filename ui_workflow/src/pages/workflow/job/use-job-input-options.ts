import { useQuery } from "@tanstack/react-query";
import { listGeneratedAssets } from "@/domains/generated-assets/lib/api";
import type { GeneratedAsset } from "@/domains/generated-assets/types";
import type { WorkflowOption } from "@/domains/jobs/types";
import { listMaterials } from "@/domains/materials/lib/api";
import type { Material } from "@/domains/materials/types";
import { listProducts } from "@/domains/products/lib/api";
import type { Product } from "@/domains/products/types";
import { listVirtualIps } from "@/domains/virtual-ips/lib/api";
import type { VirtualIp } from "@/domains/virtual-ips/types";
import { compatibleInputOptions } from "./input-option-filter";

export type JobInputOption = Product | VirtualIp | Material | GeneratedAsset;
export type JobInputOptions = Record<string, JobInputOption[]>;

export function useJobInputOptions(
  workflow: WorkflowOption | undefined,
  organizationReady: boolean,
) {
  return useQuery({
    queryKey: ["jobs", "input-options", workflow?.id, workflow?.version],
    enabled: Boolean(workflow && organizationReady),
    queryFn: async (): Promise<JobInputOptions> => {
      if (!workflow) return {};
      const loaded = await Promise.all(
        (workflow.input_schema?.entries ?? []).map(
          async (entry): Promise<[string, JobInputOption[]]> => {
            switch (entry.resource_type) {
              case "PRODUCT": {
                const page = await listProducts({
                  scope: workflow.scope,
                  status: "active",
                  page: 1,
                  page_size: 200,
                });
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, page.items),
                ];
              }
              case "VIRTUAL_IP": {
                const page = await listVirtualIps({
                  scope: workflow.scope,
                  status: "active",
                  page: 1,
                  page_size: 200,
                });
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, page.items),
                ];
              }
              case "MATERIAL": {
                const page = await listMaterials({
                  scope: workflow.scope,
                  asset_kind: entry.asset_kind,
                  page: 1,
                  page_size: 200,
                });
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, page.items),
                ];
              }
              case "ASSET": {
                const page = await listGeneratedAssets({
                  scope: workflow.scope,
                  asset_kind: entry.asset_kind,
                  page: 1,
                  page_size: 200,
                });
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, page.items),
                ];
              }
            }
          },
        ),
      );
      return Object.fromEntries(loaded);
    },
  });
}
