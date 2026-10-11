import type { WorkflowInputSchemaEntry } from "@/domains/jobs/types";
import type { WorkflowStep } from "@/domains/workflows/types";

export interface RunInputGroup {
  stepKey: string;
  entries: WorkflowInputSchemaEntry[];
}

/** Preparation order only; Conductor owns actual execution scheduling. */
export function runInputGroups(
  steps: WorkflowStep[],
  entries: WorkflowInputSchemaEntry[],
) {
  const byKey = new Map(steps.map((step) => [step.key, step]));
  const byName = new Map(entries.map((entry) => [entry.name, entry]));
  if (byKey.size !== steps.length || byName.size !== entries.length)
    throw new Error("Duplicate input or node");
  const pending = [...steps];
  const ordered: WorkflowStep[] = [];
  const visited = new Set<string>();
  while (pending.length) {
    const index = pending.findIndex((step) =>
      step.slot_bindings.every((binding) => {
        if (binding.source !== "step_output") return true;
        if (!binding.ref || !byKey.has(binding.ref))
          throw new Error("Unknown predecessor");
        return visited.has(binding.ref);
      }),
    );
    if (index < 0) throw new Error("Cyclic workflow");
    const [step] = pending.splice(index, 1);
    visited.add(step.key);
    ordered.push(step);
  }
  const selected = new Set<string>();
  const groups: RunInputGroup[] = [];
  for (const step of ordered) {
    const inputs: WorkflowInputSchemaEntry[] = [];
    for (const binding of step.slot_bindings) {
      if (binding.source !== "workflow_input") continue;
      const entry = binding.ref && byName.get(binding.ref);
      if (!entry) throw new Error("Unknown workflow input");
      if (!selected.has(entry.name)) {
        inputs.push(entry);
        selected.add(entry.name);
      }
    }
    if (inputs.length) groups.push({ stepKey: step.key, entries: inputs });
  }
  if (entries.some((entry) => !selected.has(entry.name)))
    throw new Error("Unbound workflow input");
  return { groups, steps: ordered };
}
