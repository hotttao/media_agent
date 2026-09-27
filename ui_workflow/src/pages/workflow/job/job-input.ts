import type { GeneratedAsset } from "@/domains/generated-assets/types";
import type {
  JobInput,
  WorkflowInputSchemaEntry,
  WorkflowOption,
} from "@/domains/jobs/types";
import type { Material } from "@/domains/materials/types";
import type { Product } from "@/domains/products/types";
import type { VirtualIp } from "@/domains/virtual-ips/types";
import type { JobInputOption, JobInputOptions } from "./use-job-input-options";

export function missingRequiredInputs(
  workflow: WorkflowOption | undefined,
  selectedIds: Record<string, string>,
): string[] {
  return (workflow?.input_schema?.entries ?? [])
    .filter((entry) => entry.required && !selectedIds[entry.name])
    .map((entry) => entry.name);
}

function findSelected(
  entry: WorkflowInputSchemaEntry,
  selectedIds: Record<string, string>,
  options: JobInputOptions,
): JobInputOption | undefined {
  return (options[entry.name] ?? []).find(
    (option) => option.id === selectedIds[entry.name],
  );
}

function asString(value: unknown): string {
  if (typeof value === "string") return value;
  return value === undefined ? "" : JSON.stringify(value);
}

export async function buildJobInput(
  workflow: WorkflowOption,
  selectedIds: Record<string, string>,
  options: JobInputOptions,
  resolveObject: (path?: string) => Promise<string>,
): Promise<JobInput> {
  const input: JobInput = { asset: [], material: [] };
  for (const entry of workflow.input_schema?.entries ?? []) {
    const selected = findSelected(entry, selectedIds, options);
    if (!selected) continue;
    switch (entry.resource_type) {
      case "PRODUCT": {
        const product = selected as Product;
        const image = product.images?.find((item) => item.is_main) ?? product.images?.[0];
        input.product = {
          id: product.id,
          name: product.name,
          target_audience: product.target_audience,
          product_details: product.product_details,
          display_actions: asString(product.display_actions),
          image_url: await resolveObject(image?.object_path),
        };
        break;
      }
      case "VIRTUAL_IP": {
        const virtualIp = selected as VirtualIp;
        input.virtual_ip = {
          id: virtualIp.id,
          name: virtualIp.name,
          avatar_url: await resolveObject(virtualIp.avatar_object_path),
          full_body_url: await resolveObject(virtualIp.full_body_object_path),
          three_view_url: await resolveObject(virtualIp.three_view_object_path),
          nine_view_url: await resolveObject(virtualIp.nine_view_object_path),
        };
        break;
      }
      case "MATERIAL": {
        const material = selected as Material;
        input.material?.push({
          name: entry.name,
          asset_kind: entry.asset_kind,
          url: await resolveObject(material.object_path),
          content:
            material.content ?? material.prompt ?? material.description ?? "",
        });
        break;
      }
      case "ASSET": {
        const asset = selected as GeneratedAsset;
        input.asset?.push({
          name: entry.name,
          asset_kind: entry.asset_kind,
          url: asset.url,
          content: asset.content ?? "",
        });
        break;
      }
    }
  }
  return input;
}
