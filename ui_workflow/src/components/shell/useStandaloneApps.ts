import type { AppDefinition } from "@/foundation/types/app-registry";
import { MaterialsApp } from "@/pages/materials/app";
import { WorkflowApp } from "@/pages/workflow/app";

/**
 * App registry for standalone mode. Returns the AppDefinition[] shown in the
 * desktop's dock/picker. Plugin mode registers these same app components as
 * panels via window.tos.registerPanel (see src/plugin.tsx).
 */
export function useStandaloneApps(): AppDefinition[] {
  const { t } = useTranslation();
  return [
    {
      id: "materials",
      label: t("nav.materials"),
      Component: MaterialsApp,
      group: "tool",
    },
    {
      id: "workflow",
      label: t("nav.workflow"),
      Component: WorkflowApp,
      group: "tool",
    },
  ];
}
import { useTranslation } from "react-i18next";
