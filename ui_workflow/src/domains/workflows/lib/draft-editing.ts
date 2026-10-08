import type { WorkflowInput, WorkflowStep } from "@/domains/workflows/types";

export function updateDraftStep(
  value: WorkflowInput,
  index: number,
  patch: Partial<WorkflowStep>,
): WorkflowInput {
  const previous = value.steps[index];
  if (!previous) return value;
  const renamed = patch.key !== undefined && patch.key !== previous.key;
  const steps = value.steps.map((step, position) => {
    const next = position === index ? { ...step, ...patch } : step;
    return renamed
      ? {
          ...next,
          slot_bindings: next.slot_bindings.map((binding) =>
            binding.source === "step_output" && binding.ref === previous.key
              ? { ...binding, ref: patch.key }
              : binding,
          ),
        }
      : next;
  });
  return {
    ...value,
    steps,
    output_step_key:
      renamed && value.output_step_key === previous.key
        ? (patch.key ?? value.output_step_key)
        : value.output_step_key,
  };
}

export function removalImpact(value: WorkflowInput, key: string): string[] {
  return value.steps.flatMap((step) =>
    step.slot_bindings
      .filter(
        (binding) => binding.source === "step_output" && binding.ref === key,
      )
      .map((binding) => `${step.key} → ${binding.to_input}`),
  );
}

/** Keep dangling bindings visible for repair; never silently change semantics. */
export function removeDraftStep(
  value: WorkflowInput,
  key: string,
): WorkflowInput {
  return {
    ...value,
    steps: value.steps.filter((step) => step.key !== key),
    output_step_key: value.output_step_key === key ? "" : value.output_step_key,
  };
}
