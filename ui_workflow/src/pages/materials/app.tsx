/**
 * MaterialsApp - the reusable app unit for both standalone (Desktop slot) and
 * plugin (registerPanel) modes. One panel covers all media_materials resources;
 * a resource Tabs in the body switches between Material / VirtualIp / Product /
 * AssetKind / GeneratedAsset. Internal view switching (list/show/create/edit)
 * via instance state (NOT react-router) - see docs/plan/ui_workflow.md §3.1.
 */

import { Plus } from "lucide-react";
import { createPortal } from "react-dom";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hostShim } from "@/foundation/lib/host-shim";
import { AssetKindCreate } from "./asset-kind/create";
import { AssetKindEdit } from "./asset-kind/edit";
import { AssetKindList } from "./asset-kind/list";
import { AssetKindShow } from "./asset-kind/show";
import { GeneratedAssetList } from "./generated-asset/list";
import { GeneratedAssetShow } from "./generated-asset/show";
import { MaterialsCreate } from "./material/create";
import { MaterialsEdit } from "./material/edit";
import { MaterialsList } from "./material/list";
import { MaterialsShow } from "./material/show";
import { ProductCreate } from "./product/create";
import { ProductEdit } from "./product/edit";
import { ProductList } from "./product/list";
import { ProductShow } from "./product/show";
import { VirtualIpCreate } from "./virtual-ip/create";
import { VirtualIpEdit } from "./virtual-ip/edit";
import { VirtualIpList } from "./virtual-ip/list";
import { VirtualIpShow } from "./virtual-ip/show";

type Resource =
  | "material"
  | "virtual_ip"
  | "product"
  | "asset_kind"
  | "generated_asset";
type View = "list" | "show" | "create" | "edit";

const RESOURCES: { id: Resource; label: string; readonly?: boolean }[] = [
  { id: "material", label: "Materials" },
  { id: "virtual_ip", label: "Virtual IPs" },
  { id: "product", label: "Products" },
  { id: "asset_kind", label: "Asset Kinds" },
  { id: "generated_asset", label: "Generated", readonly: true },
];

export function MaterialsApp({ instanceId }: { instanceId: string }) {
  const {
    useAppHeaderSlot,
    AppHeaderButton,
    useInstancePersistentState,
    useInstanceState,
  } = hostShim;
  const headerSlot = useAppHeaderSlot();
  const [resource, setResource] = useInstancePersistentState<Resource>(
    instanceId,
    "materials_resource",
    () => "material",
  );
  const [selectedId, setSelectedId] = useInstancePersistentState<string | null>(
    instanceId,
    "materials_selectedId",
    () => null,
  );
  const [view, setView] = useInstanceState<View>(
    instanceId,
    "materials_view",
    () => "list",
  );

  const current = RESOURCES.find((r) => r.id === resource);
  const canCreate = !current?.readonly;

  function changeResource(r: Resource) {
    setResource(r);
    setSelectedId(null);
    setView("list");
  }

  function select(id: string) {
    setSelectedId(id);
    setView("show");
  }

  return (
    <>
      {headerSlot &&
        canCreate &&
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
          <Tabs
            value={resource}
            onValueChange={(v) => changeResource(v as Resource)}
          >
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
          {resource === "material" && view === "list" && (
            <MaterialsList onSelect={select} />
          )}
          {resource === "material" && view === "show" && selectedId && (
            <MaterialsShow
              id={selectedId}
              onBack={() => setView("list")}
              onEdit={() => setView("edit")}
            />
          )}
          {resource === "material" && view === "create" && (
            <MaterialsCreate onDone={() => setView("list")} />
          )}
          {resource === "material" && view === "edit" && selectedId && (
            <MaterialsEdit id={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "virtual_ip" && view === "list" && (
            <VirtualIpList onSelect={select} />
          )}
          {resource === "virtual_ip" && view === "show" && selectedId && (
            <VirtualIpShow
              id={selectedId}
              onBack={() => setView("list")}
              onEdit={() => setView("edit")}
            />
          )}
          {resource === "virtual_ip" && view === "create" && (
            <VirtualIpCreate onDone={() => setView("list")} />
          )}
          {resource === "virtual_ip" && view === "edit" && selectedId && (
            <VirtualIpEdit id={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "product" && view === "list" && (
            <ProductList onSelect={select} />
          )}
          {resource === "product" && view === "show" && selectedId && (
            <ProductShow
              id={selectedId}
              onBack={() => setView("list")}
              onEdit={() => setView("edit")}
            />
          )}
          {resource === "product" && view === "create" && (
            <ProductCreate onDone={() => setView("list")} />
          )}
          {resource === "product" && view === "edit" && selectedId && (
            <ProductEdit id={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "asset_kind" && view === "list" && (
            <AssetKindList onSelect={select} />
          )}
          {resource === "asset_kind" && view === "show" && selectedId && (
            <AssetKindShow
              name={selectedId}
              onBack={() => setView("list")}
              onEdit={() => setView("edit")}
            />
          )}
          {resource === "asset_kind" && view === "create" && (
            <AssetKindCreate onDone={() => setView("list")} />
          )}
          {resource === "asset_kind" && view === "edit" && selectedId && (
            <AssetKindEdit name={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "generated_asset" && view === "list" && (
            <GeneratedAssetList onSelect={select} />
          )}
          {resource === "generated_asset" && view === "show" && selectedId && (
            <GeneratedAssetShow
              id={selectedId}
              onBack={() => setView("list")}
            />
          )}
        </div>
      </div>
    </>
  );
}
