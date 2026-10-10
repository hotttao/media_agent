import type { GeneratedAsset } from "@/domains/generated-assets/types";
import type {
  JobInput,
  WorkflowInputSchemaEntry,
  WorkflowOption,
  Job,
  StepRun,
} from "@/domains/jobs/types";
import type { WorkflowStep } from "@/domains/workflows/types";
import type { Material } from "@/domains/materials/types";
import type { Product } from "@/domains/products/types";
import type { VirtualIp } from "@/domains/virtual-ips/types";
import type { JobInputOption, JobInputOptions } from "./use-job-input-options";

export function isWorkflowReady(
  workflow?: Pick<
    WorkflowOption,
    "status" | "publication_status" | "allowed_actions"
  >,
): boolean {
  return Boolean(
    workflow &&
      (workflow.status === "published" || workflow.status === 2) &&
      workflow.allowed_actions?.includes("run") &&
      (!workflow.publication_status ||
        workflow.publication_status === "registered"),
  );
}

export function missingRequiredInputs(
  workflow: WorkflowOption | undefined,
  selectedIds: Record<string, string>,
): string[] {
  return (workflow?.input_schema?.entries ?? [])
    .filter((entry) => entry.required && !selectedIds[entry.name])
    .map((entry) => entry.name);
}

/** Display projection only. Server Resume remains authoritative. Missing
 * frozen/prefix evidence must not silently request inputs from restored steps. */
export function resumeInputContract(
  workflow: WorkflowOption,
  steps: WorkflowStep[] | undefined,
  runs: StepRun[] | undefined,
  job: Job,
): WorkflowOption | undefined {
  if (
    !steps?.length ||
    !runs?.length ||
    job.origin !== "legacy_import" ||
    workflow.version !== job.workflow_version ||
    workflow.id !== job.workflow_id ||
    steps.length !== runs.length
  )
    return undefined;
  const byKey = new Map(runs.map((r) => [r.step_key, r]));
  if (byKey.size !== runs.length || runs.some((r) => r.job_id !== job.id))
    return undefined;
  const restored = new Set<string>();
  for (const step of steps) {
    const run = byKey.get(step.key);
    if (
      !run ||
      run.template_id !== step.template_id ||
      run.template_version !== step.template_version
    )
      return undefined;
    if (run.origin === "legacy_import") {
      if (run.status !== "succeeded" || !run.output_asset_id) return undefined;
      restored.add(step.key);
    }
  }
  if (!restored.size) return undefined;
  const needed = new Set(
    steps
      .filter((s) => !restored.has(s.key))
      .flatMap((s) =>
        s.slot_bindings
          .filter((b) => b.source === "workflow_input")
          .map((b) => b.ref),
      ),
  );
  const entries = workflow.input_schema?.entries ?? [];
  if (
    [...needed].some((name) => !name || !entries.some((e) => e.name === name))
  )
    return undefined;
  return {
    ...workflow,
    input_schema: {
      entries: entries.filter(
        (entry) =>
          needed.has(entry.name) &&
          !(entry.resource_type === "PRODUCT"
            ? job.input.product
            : entry.resource_type === "VIRTUAL_IP"
              ? job.input.virtual_ip
              : (entry.resource_type === "ASSET"
                  ? job.input.asset
                  : job.input.material
                )?.some((item) => item.name === entry.name)),
      ),
    },
  };
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

export function selectedInputPreview(
  type: WorkflowInputSchemaEntry["resource_type"],
  selected?: JobInputOption,
): { paths: string[]; content?: string } | undefined {
  if (!selected) return undefined;
  if (type === "PRODUCT") {
    const product = selected as Product;
    const image =
      product.images?.find((item) => item.is_main) || product.images?.[0];
    return {
      paths: image?.object_path ? [image.object_path] : [],
      content: product.product_details || product.description,
    };
  }
  if (type === "VIRTUAL_IP") {
    const character = selected as VirtualIp;
    const path =
      character.full_body_object_path ||
      character.avatar_object_path ||
      character.three_view_object_path ||
      character.nine_view_object_path;
    return { paths: path ? [path] : [], content: character.description };
  }
  const resource = selected as Material & GeneratedAsset;
  const path =
    resource.object_path || (type === "ASSET" ? resource.url : undefined);
  return {
    paths: path ? [path] : [],
    content:
      resource.content ||
      (type === "MATERIAL"
        ? resource.description || resource.prompt
        : undefined),
  };
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
        const image =
          product.images?.find((item) => item.is_main) ?? product.images?.[0];
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
          url: await resolveObject(asset.object_path || asset.url),
          content: asset.content ?? "",
        });
        break;
      }
    }
  }
  return input;
}
