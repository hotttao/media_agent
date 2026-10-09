import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
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

export function ProductWorkflowLaunch({
  product,
  organizationId,
}: {
  product: Product;
  organizationId: string;
}) {
  const { t } = useTranslation();
  const navigation = hostShim.useNavigationPort();
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState("");
  const candidates = useQuery({
    queryKey: ["product-workflows", organizationId, product.scope],
    enabled: open && Boolean(organizationId),
    queryFn: () =>
      listWorkflows({
        scope: product.scope,
        status: "published",
        page_size: 200,
      }),
  });
  const eligible = (candidates.data?.items || []).filter(
    (item) =>
      item.organization_id === organizationId &&
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
      detail.data.organization_id === organizationId &&
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
            <select
              className="h-10 w-full rounded-lg border bg-background px-3"
              aria-label={t("productWorkflow.available")}
              value={selected}
              onChange={(event) => setSelected(event.target.value)}
            >
              <option value="">{t("productWorkflow.select")}</option>
              {eligible.map((item) => (
                <option value={item.id} key={item.id}>
                  {item.name} · v{item.version}
                </option>
              ))}
            </select>
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
