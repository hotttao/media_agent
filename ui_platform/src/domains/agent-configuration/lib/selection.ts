import type {
  MetadataSelection,
  PromptSelection,
  ResourceReference,
} from "@/domains/agent-configuration/types";
export function selectedPrompt(selection: MetadataSelection): PromptSelection {
  if (selection.prompt) return selection.prompt;
  const ref = selection.resources?.find(
    (item) => item.kind === "media_control.configured_prompt",
  );
  return ref
    ? { mode: "library", resource_id: ref.id, resource_version: ref.version }
    : { mode: "none" };
}
export function selectionBody(
  prompt: PromptSelection,
  refs: ResourceReference[],
): { prompt: PromptSelection; resources: ResourceReference[] } {
  const normalized: PromptSelection =
    prompt.mode === "none"
      ? { mode: "none" }
      : prompt.mode === "custom"
        ? { mode: "custom", content: prompt.content ?? "" }
        : {
            mode: "library",
            resource_id: prompt.resource_id,
            resource_version: prompt.resource_version,
          };
  return {
    prompt: normalized,
    resources: refs
      .filter((item) => item.kind !== "media_control.configured_prompt")
      .map(({ kind, id, version }) => ({ kind, id, version }))
      .sort((a, b) => a.kind.localeCompare(b.kind) || a.id.localeCompare(b.id)),
  };
}
export function resourceKind(choice: "prompts" | "skills" | "mcp"): string {
  return choice === "mcp"
    ? "media_plugin.mcp_binding"
    : choice === "skills"
      ? "media_control.skill"
      : "media_control.configured_prompt";
}
