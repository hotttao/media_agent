import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import type { CatalogKind } from "@/domains/agent-catalog/types";
import {
  listMyTeams,
  listOrganizations,
} from "@/foundation/lib/hierarchy-client";
import { useAuth } from "@/foundation/providers/auth-provider";
import { MCPCatalog } from "./mcp-catalog";
import { ResourceLibrary } from "./resource-library";
import { CatalogError, inputClass } from "./shared";

export function CatalogManager({
  kind,
  organizationId,
  onOrganizationChange,
}: {
  kind: CatalogKind;
  organizationId?: string;
  onOrganizationChange(value: string): void;
}) {
  const { t } = useTranslation();
  const { user } = useAuth();
  const load = useCallback(async () => {
    const [organizations, teams] = await Promise.all([
      listOrganizations(),
      listMyTeams(),
    ]);
    return { organizations, teams };
  }, []);
  const request = useCatalogLoad(load);
  const organization =
    request.data?.organizations.find((item) => item.id === organizationId) ??
    (!organizationId ? request.data?.organizations[0] : undefined);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-stone-200 px-5 py-3">
        <label className="flex items-center gap-3 text-sm text-stone-600">
          {t("catalog.organization")}
          <select
            aria-label={t("catalog.organization")}
            className={`${inputClass} max-w-52`}
            value={organization?.id ?? ""}
            onChange={(e) => onOrganizationChange(e.target.value)}
          >
            <option value="" disabled>
              {t("catalog.chooseOrganization")}
            </option>
            {request.data?.organizations.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
        </label>
        <span className="text-xs text-stone-500">
          {t("settingsNavigation.organizationScope")}
        </span>
      </div>
      {request.loading ? (
        <p className="p-6 text-sm text-stone-500">{t("catalog.loading")}</p>
      ) : request.error ? (
        <div className="p-6">
          <CatalogError error={request.error} retry={request.reload} />
        </div>
      ) : organization ? (
        kind === "mcp" ? (
          <MCPCatalog
            key={organization.id}
            organization={organization.id}
            admin={organization.my_role === "admin"}
            platformAdmin={user?.role === "admin"}
          />
        ) : (
          <ResourceLibrary
            key={`${organization.id}:${kind}`}
            kind={kind}
            organization={organization.id}
            organizationName={organization.name}
            teams={request.data?.teams ?? []}
          />
        )
      ) : (
        <p className="p-6 text-sm text-stone-500">
          {t(organizationId ? "catalog.forbidden" : "catalog.noOrganization")}
        </p>
      )}
    </div>
  );
}
