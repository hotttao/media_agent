import type { WorkflowInputSchemaEntry } from "@/domains/jobs/types";
import type { JobInputOption } from "./use-job-input-options";

/** Defensive UI compatibility check; server authorization remains authoritative. */
export function compatibleInputOptions(
  scope: string,
  entry: WorkflowInputSchemaEntry,
  options: JobInputOption[],
): JobInputOption[] {
  return options.filter(
    (option) =>
      option.scope === scope &&
      ((entry.resource_type !== "MATERIAL" &&
        entry.resource_type !== "ASSET") ||
        ("asset_kind" in option && option.asset_kind === entry.asset_kind)),
  );
}
