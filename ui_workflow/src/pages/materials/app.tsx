/**
 * MaterialsApp - the reusable app unit for both standalone (Desktop slot) and
 * plugin (registerPanel) modes. One panel covers all media_materials resources;
 * a resource Tabs in the body switches between Material / VirtualIp / Product /
 * AssetKind / GeneratedAsset. Internal view switching (list/show/create/edit)
 * via instance state (NOT react-router) - see docs/plan/ui_workflow.md §3.1.
 *
 * Only Material is implemented this phase; other resources render a placeholder.
 */
import { createPortal } from "react-dom";
import { Plus } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hostShim } from "@/foundation/lib/host-shim";
import { MaterialsCreate } from "./create";
import { MaterialsEdit } from "./edit";
import { MaterialsList } from "./list";
import { MaterialsShow } from "./show";

type Resource = "material" | "virtual_ip" | "product" | "asset_kind" | "generated_asset";
type View = "list" | "show" | "create" | "edit";

const RESOURCES: { id: Resource; label: string }[] = [
  { id: "material", label: "Materials" },
  { id: "virtual_ip", label: "Virtual IPs" },
  { id: "product", label: "Products" },
  { id: "asset_kind", label: "Asset Kinds" },
  { id: "generated_asset", label: "Generated" },
];

export function MaterialsApp({ instanceId }: { instanceId: string }) {
  const { useAppHeaderSlot, AppHeaderButton, useInstancePersistentState, useInstanceState } =
    hostShim;
  const headerSlot = useAppHeaderSlot();
  const [resource, setResource] = useInstancePersistentState<Resource>(
    instanceId,
    "resource",
    () => "material",
  );
  const [selectedId, setSelectedId] = useInstancePersistentState<string | null>(
    instanceId,
    "selectedId",
    () => null,
  );
  const [view, setView] = useInstanceState<View>(instanceId, "view", () => "list");

  function changeResource(r: Resource) {
    setResource(r);
    setSelectedId(null);
    setView("list");
  }

  return (
    <>
      {headerSlot &&
        createPortal(
          <AppHeaderButton
            icon={Plus}
            label="New"
            title="New"
            onClick={() => {
              setSelectedId(null);
              setView("create");
            }}
          />,
          headerSlot,
        )}
      <div className="flex h-full flex-col">
        <div className="border-b border-foreground/[0.06] px-3 py-2">
          <Tabs value={resource} onValueChange={(v) => changeResource(v as Resource)}>
            <TabsList>
              {RESOURCES.map((r) => (
                <TabsTrigger key={r.id} value={r.id}>
                  {r.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
        <div className="min-h-0 flex-1">
          {resource === "material" ? (
            <>
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
            </>
          ) : (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {RESOURCES.find((r) => r.id === resource)?.label} - coming soon
            </div>
          )}
        </div>
      </div>
    </>
  );
}
