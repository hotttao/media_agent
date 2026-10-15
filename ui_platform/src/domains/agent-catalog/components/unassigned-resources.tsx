import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import { catalogPath } from "@/domains/agent-catalog/lib/catalog-client";
import type { CatalogResource } from "@/domains/agent-catalog/types";
import {
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";
import { CatalogError, CatalogModal, ConfirmAction } from "./shared";
export function UnassignedResources({
  kind,
  organization,
  organizationName,
  close,
  assigned,
}: {
  kind: "prompts" | "skills";
  organization: string;
  organizationName: string;
  close(): void;
  assigned(value: CatalogResource): void;
}) {
  const { t } = useTranslation();
  const load = useCallback(async () => {
    const data = await managementRequest<Record<string, CatalogResource[]>>(
      `${catalogPath(kind)}/unassigned`,
    );
    return data[kind] ?? [];
  }, [kind]);
  const request = useCatalogLoad(load);
  const [error, setError] = useState<unknown>();
  return (
    <CatalogModal title={t("catalog.unassigned")} close={close}>
      <p className="mb-4 text-sm leading-6 text-stone-500">
        {t("catalog.unassignedHint")}
      </p>
      <CatalogError error={request.error || error} retry={request.reload} />
      {request.loading && (
        <p className="text-sm text-stone-500">{t("catalog.loading")}</p>
      )}
      {!request.loading && !request.data?.length && (
        <p className="py-6 text-center text-sm text-stone-500">
          {t("catalog.empty")}
        </p>
      )}
      <div className="grid gap-3">
        {request.data?.map((item) => (
          <div
            key={item.id}
            className="flex items-center justify-between gap-3 rounded-lg border p-3"
          >
            <span className="break-words text-sm font-medium">{item.name}</span>
            <ConfirmAction
              title={t("catalog.assign")}
              description={t("catalog.assignConfirm", {
                name: item.name,
                organization: organizationName,
              })}
              onError={setError}
              action={async () => {
                const value = await managementWrite<CatalogResource>(
                  `${catalogPath(kind, item.id)}/assign-organization`,
                  "POST",
                  {
                    organization_id: organization,
                    expected_version: item.version,
                  },
                );
                assigned(value);
                close();
              }}
            />
          </div>
        ))}
      </div>
    </CatalogModal>
  );
}
