import type {
  InvocationExpression,
  TemplateInput,
  TemplateSlotInput,
} from "@/domains/templates/types";

export function providerIdentity(key: string): Partial<TemplateInput> | null {
  const match =
    /^([a-z][a-z0-9_]*)\.(t2i|i2i|t2v|i2v)\.([a-z][a-z0-9_]*)(?:\.managed)?$/.exec(
      key,
    );
  return match
    ? { platform: match[1], gen_way: match[2], gen_type: match[3] }
    : null;
}

export function renameInvocationSlot(
  expression: InvocationExpression,
  previous: string,
  next: string,
): InvocationExpression {
  return {
    ...expression,
    ...(expression.slot === previous && previous ? { slot: next } : {}),
    ...(expression.items
      ? {
          items: expression.items.map((item) =>
            renameInvocationSlot(item, previous, next),
          ),
        }
      : {}),
  };
}
export function updateTemplateSlot(
  value: TemplateInput,
  index: number,
  patch: Partial<TemplateSlotInput>,
): TemplateInput {
  const previous = value.inputs[index];
  return {
    ...value,
    inputs: value.inputs.map((slot, position) =>
      position === index ? { ...slot, ...patch } : slot,
    ),
    invocation_mapping:
      patch.name !== undefined && previous
        ? Object.fromEntries(
            Object.entries(value.invocation_mapping).map(
              ([name, expression]) => [
                name,
                renameInvocationSlot(
                  expression,
                  previous.name,
                  patch.name ?? previous.name,
                ),
              ],
            ),
          )
        : value.invocation_mapping,
  };
}
export function defaultImageInvocation(
  slots: TemplateSlotInput[],
): Record<string, InvocationExpression> {
  return {
    prompt: { op: "render_template" },
    images: {
      op: "list",
      items: slots
        .filter((slot) => slot.resource_type !== "TEXT" && slot.name)
        .map((slot) => ({ op: "asset_ref", slot: slot.name })),
    },
  };
}
