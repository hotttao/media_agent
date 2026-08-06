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
import i18n from "@/foundation/lib/i18n";
import {
  type PluginPanelProps,
  registerPanel,
} from "@/foundation/lib/plugin-host";
import { QueryProvider } from "@/foundation/providers/query-provider";
import { MaterialsApp } from "@/pages/materials/app";
import { WorkflowApp } from "@/pages/workflow/app";
import pluginCss from "./index.css?inline";

type PanelComp = ComponentType<PluginPanelProps>;
const STYLE_ID = "media-workflow-plugin-styles";

function installStyles(): void {
  if (document.getElementById(STYLE_ID)) return;
  const style = document.createElement("style");
  style.id = STYLE_ID;
  style.textContent = pluginCss;
  document.head.appendChild(style);
}

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

installStyles();

registerPanel({
  id: "media_materials",
  label: () => i18n.t("pages.materials.title", { defaultValue: "Materials" }),
  component: withQueryProvider(MaterialsApp),
});

registerPanel({
  id: "media_workflow",
  label: () => i18n.t("pages.workflow.title", { defaultValue: "Workflow" }),
  component: withQueryProvider(WorkflowApp),
});
