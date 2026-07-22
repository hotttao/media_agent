/**
 * MaterialsApp - the reusable app unit for both standalone (Desktop slot) and
 * plugin (registerPanel) modes. Internal view switching via instance state
 * (NOT react-router) - see docs/plan/ui_workflow.md §3.1.
 */
import { createPortal } from "react-dom";
import { Plus } from "lucide-react";
import { hostShim } from "@/foundation/lib/host-shim";
import { MaterialsCreate } from "./create";
import { MaterialsEdit } from "./edit";
import { MaterialsList } from "./list";
import { MaterialsShow } from "./show";

type View = "list" | "show" | "create" | "edit";

/**
 * MaterialsApp takes only `instanceId` (the slot/panel also passes
 * `workspaceId`, which we ignore). This makes it assignable to both
 * AppComponentProps (standalone, instanceId only) and PluginPanelProps
 * (plugin, workspaceId + instanceId) via contravariance.
 */
export function MaterialsApp({ instanceId }: { instanceId: string }) {
  const { useAppHeaderSlot, AppHeaderButton, useInstancePersistentState, useInstanceState } =
    hostShim;
  const headerSlot = useAppHeaderSlot();
  const [selectedId, setSelectedId] = useInstancePersistentState<string | null>(
    instanceId,
    "selectedId",
    () => null,
  );
  const [view, setView] = useInstanceState<View>(instanceId, "view", () => "list");

  return (
    <>
      {headerSlot &&
        createPortal(
          <AppHeaderButton
            icon={Plus}
            label="New"
            title="New material"
            onClick={() => {
              setSelectedId(null);
              setView("create");
            }}
          />,
          headerSlot,
        )}
      <div className="flex h-full flex-col">
        {view === "list" && (
          <MaterialsList
            onSelect={(id) => {
              setSelectedId(id);
              setView("show");
            }}
          />
        )}
        {view === "show" && selectedId && (
          <MaterialsShow
            id={selectedId}
            onBack={() => setView("list")}
            onEdit={() => setView("edit")}
          />
        )}
        {view === "create" && <MaterialsCreate onDone={() => setView("list")} />}
        {view === "edit" && selectedId && (
          <MaterialsEdit id={selectedId} onDone={() => setView("show")} />
        )}
      </div>
    </>
  );
}
