/**
 * Plugin entry (built as an IIFE bundle, VITE_TARGET=plugin).
 *
 * Registers the materials app as a workspace panel. Each panel is wrapped in
 * a plugin-local QueryProvider (per docs/plan/ui_workflow.md §4.3 方案 C -
 * window.tos does not expose react-query, so the plugin carries its own copy
 * + QueryClient; cache is intentionally isolated from the host's).
 * i18next is initialized via the side-effect import and kept in sync with the
 * host language via the `lang.change` event (see foundation/lib/i18n.ts).
 *
 * React/ReactDOM are externalized to window.tos (single React instance required
 * by the rules of hooks).
 */
import type { ComponentType } from "react";
import { QueryProvider } from "@/foundation/providers/query-provider";
import "@/foundation/lib/i18n"; // side-effect: initialize i18next + subscribe to lang.change
import { registerPanel, type PluginPanelProps } from "@/foundation/lib/plugin-host";
import { MaterialsApp } from "@/pages/materials/app";
import { WorkflowApp } from "@/pages/workflow/app";

type PanelComp = ComponentType<PluginPanelProps>;

/** Wrap a panel component with the plugin-local QueryProvider. */
function withQueryProvider(Comp: PanelComp): PanelComp {
  return function Panel({ workspaceId, instanceId }: PluginPanelProps) {
    return (
      <QueryProvider>
        <Comp workspaceId={workspaceId} instanceId={instanceId} />
      </QueryProvider>
    );
  };
}

registerPanel({
  id: "media_materials",
  label: () => "Materials",
  component: withQueryProvider(MaterialsApp),
});

registerPanel({
  id: "media_workflow",
  label: () => "Workflow",
  component: withQueryProvider(WorkflowApp),
});
