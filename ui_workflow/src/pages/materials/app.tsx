/**
 * MaterialsApp - the reusable app unit for both standalone (Desktop slot) and
 * plugin (registerPanel) modes. One panel covers all media_materials resources;
 * a resource Tabs in the body switches between Material / VirtualIp / Product /
 * AssetKind / GeneratedAsset. Internal view switching (list/show/create/edit)
 * via instance state (NOT react-router) - see docs/plan/ui_workflow.md §3.1.
 */

import { Plus, Settings2 } from "lucide-react";
import { useEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hostShim } from "@/foundation/lib/host-shim";
import {
  PRODUCT_OPEN_KEY,
  type ProductOpenIntent,
} from "@/foundation/types/app-navigation";
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

const RESOURCES: { id: Resource; labelKey: string; readonly?: boolean }[] = [
  { id: "material", labelKey: "nav.materials" },
  { id: "virtual_ip", labelKey: "nav.virtualIps" },
  { id: "product", labelKey: "nav.products" },
  { id: "asset_kind", labelKey: "nav.assetKinds" },
  { id: "generated_asset", labelKey: "nav.generated", readonly: true },
];

export function MaterialsApp({ instanceId }: { instanceId: string }) {
  const { t } = useTranslation();
  const { user } = hostShim.useCurrentUser();
  const registryVisible =
    user?.role === "admin" || user?.role === "platform_admin";
  const {
    useAppHeaderSlot,
    AppHeaderButton,
    useInstancePersistentState,
    useInstanceState,
  } = hostShim;
  const headerSlot = useAppHeaderSlot();
  const shell = hostShim.useShellContext();
  const [incoming, consumeIntent] = useInstanceState<ProductOpenIntent | null>(
    instanceId,
    PRODUCT_OPEN_KEY,
    () => null,
  );
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

  useEffect(() => {
    if (!incoming || shell.isLoading) return;
    if (incoming.organizationId === shell.organization.id) {
      setResource("product");
      setSelectedId(incoming.productId);
      setView("show");
    }
    consumeIntent(null);
  }, [
    incoming,
    shell.isLoading,
    shell.organization.id,
    setResource,
    setSelectedId,
    setView,
    consumeIntent,
  ]);

  function changeResource(r: Resource) {
    setResource(r);
    setSelectedId(null);
    setView("list");
  }

  useEffect(() => {
    if (user && !registryVisible && resource === "asset_kind") {
      setResource("material");
      setSelectedId(null);
      setView("list");
    }
  }, [user, registryVisible, resource, setResource, setSelectedId, setView]);

  function select(id: string) {
    setSelectedId(id);
    setView("show");
  }

  return (
    <>
      {headerSlot &&
        canCreate &&
        view === "list" &&
        createPortal(
          <span className="media-workflow-ui">
            <span className="media-header-actions">
              <AppHeaderButton
                icon={Plus}
                label={t("common.new")}
                title={t("common.new")}
                onClick={() => {
                  setSelectedId(null);
                  setView("create");
                }}
              />
              {registryVisible ? (
                <AppHeaderButton
                  icon={Settings2}
                  label={t("nav.assetKinds")}
                  onClick={() => changeResource("asset_kind")}
                />
              ) : null}
            </span>
          </span>,
          headerSlot,
        )}
      <div className="media-app media-surface flex h-full min-h-0 flex-col">
        <div className="media-subnav">
          <Tabs
            value={resource}
            onValueChange={(v) => changeResource(v as Resource)}
          >
            <TabsList>
              {RESOURCES.filter((r) => r.id !== "asset_kind").map((r) => (
                <TabsTrigger key={r.id} value={r.id}>
                  {t(r.labelKey)}
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
            <MaterialsCreate
              instanceId={instanceId}
              onDone={() => setView("list")}
            />
          )}
          {resource === "material" && view === "edit" && selectedId && (
            <MaterialsEdit
              key={selectedId}
              instanceId={instanceId}
              id={selectedId}
              onDone={() => setView("show")}
            />
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
            <VirtualIpCreate
              instanceId={instanceId}
              onDone={() => setView("list")}
            />
          )}
          {resource === "virtual_ip" && view === "edit" && selectedId && (
            <VirtualIpEdit
              key={selectedId}
              instanceId={instanceId}
              id={selectedId}
              onDone={() => setView("show")}
            />
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
            <ProductCreate
              instanceId={instanceId}
              onDone={() => setView("list")}
            />
          )}
          {resource === "product" && view === "edit" && selectedId && (
            <ProductEdit
              key={selectedId}
              instanceId={instanceId}
              id={selectedId}
              onDone={() => setView("show")}
            />
          )}

          {registryVisible && resource === "asset_kind" && view === "list" && (
            <AssetKindList onSelect={select} />
          )}
          {registryVisible &&
            resource === "asset_kind" &&
            view === "show" &&
            selectedId && (
              <AssetKindShow
                name={selectedId}
                onBack={() => setView("list")}
                onEdit={() => setView("edit")}
              />
            )}
          {registryVisible &&
            resource === "asset_kind" &&
            view === "create" && (
              <AssetKindCreate
                instanceId={instanceId}
                onDone={() => setView("list")}
              />
            )}
          {registryVisible &&
            resource === "asset_kind" &&
            view === "edit" &&
            selectedId && (
              <AssetKindEdit
                key={selectedId}
                instanceId={instanceId}
                name={selectedId}
                onDone={() => setView("show")}
              />
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
