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

export function moveInvocationItem(
  expression: InvocationExpression,
  index: number,
  direction: -1 | 1,
): InvocationExpression {
  const items = [...(expression.items ?? [])];
  const target = index + direction;
  if (
    index < 0 ||
    index >= items.length ||
    target < 0 ||
    target >= items.length
  )
    return expression;
  [items[index], items[target]] = [items[target], items[index]];
  return { ...expression, items };
}

export type TemplateInputIssue = {
  code:
    | "emptyName"
    | "duplicateName"
    | "missingKind"
    | "missingReference"
    | "textReference";
  subject: string;
};

/** Local structural checks only; the backend owns provider schema/type validation. */
export function checkTemplateInputs(
  value: Pick<TemplateInput, "inputs" | "invocation_mapping">,
): TemplateInputIssue[] {
  const issues: TemplateInputIssue[] = [];
  const slots = new Map<string, TemplateSlotInput>();
  value.inputs.forEach((slot, index) => {
    if (!slot.name.trim())
      issues.push({ code: "emptyName", subject: String(index + 1) });
    if (slots.has(slot.name))
      issues.push({ code: "duplicateName", subject: slot.name });
    slots.set(slot.name, slot);
    if (!slot.asset_kind)
      issues.push({
        code: "missingKind",
        subject: slot.name || String(index + 1),
      });
  });
  function check(expression: InvocationExpression, path: string) {
    if (expression.op === "asset_ref") {
      const slot = slots.get(expression.slot ?? "");
      if (!slot || !expression.slot)
        issues.push({
          code: "missingReference",
          subject: `${path} → ${expression.slot || "?"}`,
        });
      else if (slot.resource_type === "TEXT")
        issues.push({
          code: "textReference",
          subject: `${path} → ${slot.name}`,
        });
    }
    expression.items?.forEach((item, index) =>
      check(item, `${path}[${index}]`),
    );
  }
  Object.entries(value.invocation_mapping).forEach(([name, expression]) =>
    check(expression, name),
  );
  return issues;
}
