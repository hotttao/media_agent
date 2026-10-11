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
import { allResourcePages as allInputPages } from "@/foundation/lib/all-resource-pages";

export type JobInputOption = Product | VirtualIp | Material | GeneratedAsset;
export type JobInputOptions = Record<string, JobInputOption[]>;

export function useJobInputOptions(
  workflow: WorkflowOption | undefined,
  organizationReady: boolean,
  organizationId: string,
) {
  return useQuery({
    queryKey: [
      "jobs",
      "input-options",
      organizationId,
      workflow?.id,
      workflow?.version,
      workflow?.scope,
      workflow?.input_schema?.entries,
    ],
    enabled: Boolean(workflow && organizationReady),
    queryFn: async (): Promise<JobInputOptions> => {
      if (!workflow) return {};
      const loaded = await Promise.all(
        (workflow.input_schema?.entries ?? []).map(
          async (entry): Promise<[string, JobInputOption[]]> => {
            switch (entry.resource_type) {
              case "PRODUCT": {
                const items = await allInputPages((page) =>
                  listProducts({
                    scope: workflow.scope,
                    status: "active",
                    page,
                    page_size: 200,
                  }),
                );
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, items),
                ];
              }
              case "VIRTUAL_IP": {
                const items = await allInputPages((page) =>
                  listVirtualIps({
                    scope: workflow.scope,
                    status: "active",
                    page,
                    page_size: 200,
                  }),
                );
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, items),
                ];
              }
              case "MATERIAL": {
                const items = await allInputPages((page) =>
                  listMaterials({
                    scope: workflow.scope,
                    asset_kind: entry.asset_kind,
                    page,
                    page_size: 200,
                  }),
                );
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, items),
                ];
              }
              case "ASSET": {
                const items = await allInputPages((page) =>
                  listGeneratedAssets({
                    scope: workflow.scope,
                    asset_kind: entry.asset_kind,
                    page,
                    page_size: 200,
                  }),
                );
                return [
                  entry.name,
                  compatibleInputOptions(workflow.scope, entry, items),
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
