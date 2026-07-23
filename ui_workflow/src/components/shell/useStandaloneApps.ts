import type { AppDefinition } from "@/foundation/types/app-registry";
import { MaterialsApp } from "@/pages/materials/app";
import { WorkflowApp } from "@/pages/workflow/app";

/**
 * App registry for standalone mode. Returns the AppDefinition[] shown in the
 * desktop's dock/picker. Plugin mode registers these same app components as
 * panels via window.tos.registerPanel (see src/plugin.tsx).
 */
export function useStandaloneApps(): AppDefinition[] {
  return [
    { id: "materials", label: "Materials", Component: MaterialsApp, group: "tool" },
    { id: "workflow", label: "Workflow", Component: WorkflowApp, group: "tool" },
  ];
}
