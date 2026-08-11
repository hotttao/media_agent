import type { AppDefinition } from "@/foundation/slots/types";

export function createAppRegistry(
  definitions: AppDefinition[],
): AppDefinition[] {
  const ids = new Set<string>();
  for (const definition of definitions) {
    if (!definition.id.trim()) throw new Error("App id is required");
    if (ids.has(definition.id))
      throw new Error(`Duplicate app id: ${definition.id}`);
    ids.add(definition.id);
  }
  return definitions;
}
