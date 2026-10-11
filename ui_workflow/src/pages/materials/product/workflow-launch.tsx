import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ListState } from "@/foundation/components/resource-list-layout";
import { hostShim } from "@/foundation/lib/host-shim";
import { listWorkflows } from "@/domains/workflows/lib/api";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import type { Product } from "@/domains/products/types";
import { allResourcePages as allInputPages } from "@/foundation/lib/all-resource-pages";

export function ProductWorkflowLaunch({
  product,
  organizationId,
}: {
  product: Product;
  organizationId: string;
}) {
  const { t } = useTranslation();
  const navigation = hostShim.useNavigationPort();
  const { user } = hostShim.useCurrentUser();
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const belongs = (item: {
    organization_id?: string;
    my_permission?: string;
    created_by?: string;
  }) =>
    item.organization_id === organizationId ||
    (!item.organization_id &&
      (!item.my_permission || item.my_permission === "owner") &&
      Boolean(user?.id) &&
      item.created_by === user?.id);
  const candidates = useQuery({
    queryKey: ["product-workflows", organizationId, product.scope],
    enabled: open && Boolean(organizationId),
    queryFn: () =>
      allInputPages((page) =>
        listWorkflows({
          scope: product.scope,
          status: "published",
          page_size: 200,
          page,
        }),
      ),
  });
  const eligible = (candidates.data || []).filter(
    (item) =>
      belongs(item) &&
      (!item.allowed_actions || item.allowed_actions.includes("run")) &&
      item.status === "published" &&
      (
        item.input_schema?.entries as { resource_type: string }[] | undefined
      )?.some((entry) => entry.resource_type === "PRODUCT"),
  );
  const detail = useWorkflow(open ? selected : undefined);
  const ready = Boolean(
    detail.data &&
      detail.data.id === selected &&
      detail.data.status === "published" &&
      detail.data.publication_status === "registered" &&
      belongs(detail.data) &&
      detail.data.allowed_actions?.includes("run") &&
      detail.data.scope === product.scope &&
      (
        detail.data.input_schema?.entries as
          | { resource_type: string }[]
          | undefined
      )?.some((entry) => entry.resource_type === "PRODUCT") &&
      eligible.some((item) => item.id === selected),
  );
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={product.status === "archived"}
        onClick={() => setOpen(true)}
      >
        {t("productWorkflow.choose")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("productWorkflow.choose")}</DialogTitle>
            <DialogDescription>{t("productWorkflow.hint")}</DialogDescription>
          </DialogHeader>
          {candidates.isLoading ? (
            <ListState loading />
          ) : candidates.isError ? (
            <ListState
              error={candidates.error.message}
              onRetry={() => void candidates.refetch()}
            />
          ) : !eligible.length ? (
            <ListState empty={t("productWorkflow.empty")} />
          ) : (
            <div className="space-y-3">
              <Input
                aria-label={t("productWorkflow.search")}
                placeholder={t("productWorkflow.search")}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
              <div
                className="max-h-72 space-y-2 overflow-auto"
                aria-label={t("productWorkflow.available")}
              >
                {eligible
                  .filter((item) =>
                    item.name
                      .toLocaleLowerCase()
                      .includes(search.trim().toLocaleLowerCase()),
                  )
                  .map((item) => (
                    <button
                      type="button"
                      key={item.id}
                      aria-pressed={selected === item.id}
                      className={`w-full rounded-xl border p-4 text-left ${selected === item.id ? "border-primary bg-primary/5" : "hover:bg-muted"}`}
                      onClick={() => setSelected(item.id)}
                    >
                      <p className="font-medium">
                        {item.name} · v{item.version}
                      </p>
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {item.description || item.scope}
                      </p>
                    </button>
                  ))}
              </div>
              {!eligible.some((item) =>
                item.name
                  .toLocaleLowerCase()
                  .includes(search.trim().toLocaleLowerCase()),
              ) ? (
                <p className="text-sm text-muted-foreground">
                  {t("runWizard.noMatches")}
                </p>
              ) : null}
            </div>
          )}
          {detail.isError ? (
            <ListState
              error={detail.error.message}
              onRetry={() => void detail.refetch()}
            />
          ) : null}
          {!navigation ? (
            <p className="text-xs text-muted-foreground">
              {t("productWorkflow.hostUnavailable")}
            </p>
          ) : null}
          <Button
            disabled={!navigation || !ready}
            onClick={() => {
              if (!ready || !navigation) return;
              navigation.openWorkflowRun({
                organizationId,
                workflowId: selected,
                productId: product.id,
              });
              setOpen(false);
            }}
          >
            {t("productWorkflow.openRun")}
          </Button>
        </DialogContent>
      </Dialog>
    </>
  );
}
