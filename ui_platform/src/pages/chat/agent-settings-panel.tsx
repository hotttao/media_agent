import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { CatalogError } from "@/domains/agent-catalog/components/shared";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import {
  listCatalog,
  listDefinitions,
} from "@/domains/agent-catalog/lib/catalog-client";
import { BindingManager } from "@/domains/agent-configuration/components/binding-manager";
import { MetadataEditor } from "@/domains/agent-configuration/components/metadata-editor";
import {
  listBindings,
  loadSelection,
} from "@/domains/agent-configuration/lib/configuration-client";
import type { ResourceChoice } from "@/domains/agent-configuration/types";
import type { Workspace } from "@/domains/workspace/types";
import { managementRequest } from "@/foundation/lib/management-http";
export function AgentSettingsPanel({
  workspace,
  openLibrary,
  refresh,
  editWorkspace,
}: {
  workspace: Workspace;
  openLibrary(): void;
  refresh(): Promise<void>;
  editWorkspace(): void;
}) {
  const { t } = useTranslation();
  const [success, setSuccess] = useState(false);
  const loadPermissions = useCallback(
    () =>
      managementRequest<{ can_manage?: boolean }>(
        `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/permissions`,
      ),
    [workspace.id],
  );
  const permissions = useCatalogLoad(loadPermissions);
  const load = useCallback(
    () =>
      loadSelection(
        workspace.id,
        workspace.agent_configuration_id ?? "",
        workspace.configuration_version,
      ),
    [
      workspace.id,
      workspace.agent_configuration_id,
      workspace.configuration_version,
    ],
  );
  const selection = useCatalogLoad(load);
  const loadChoices = useCallback(async () => {
    if (!workspace.organization_id) throw new Error("missing_organization");
    const [prompts, skills, definitions, bindings] = await Promise.all([
      listCatalog("prompts", workspace.organization_id),
      listCatalog("skills", workspace.organization_id),
      listDefinitions(workspace.organization_id),
      listBindings(workspace.id),
    ]);
    const choices: ResourceChoice[] = [
      ...prompts.map((item) => ({
        kind: "prompts" as const,
        id: item.id,
        name: item.name,
        version: item.version,
      })),
      ...skills.map((item) => ({
        kind: "skills" as const,
        id: item.id,
        name: item.name,
        version: item.published_version ?? 0,
        disabled: item.status !== "active",
      })),
      ...bindings.map((item) => ({
        kind: "mcp" as const,
        id: item.binding_id,
        name:
          definitions.find(
            (definition) => definition.definition_id === item.definition_id,
          )?.service_name ?? item.definition_id,
        version: item.version,
        disabled: !item.enabled,
      })),
    ];
    return { choices, definitions };
  }, [workspace.id, workspace.organization_id]);
  const options = useCatalogLoad(loadChoices);
  return (
    <div className="space-y-5">
      {permissions.data?.can_manage && (
        <Button size="sm" variant="outline" onClick={editWorkspace}>
          {t("configuration.editWorkspace")}
        </Button>
      )}
      <CatalogError error={permissions.error} retry={permissions.reload} />
      <CatalogError error={options.error} retry={options.reload} />
      {selection.loading ? (
        <p className="text-sm text-stone-500">{t("configuration.loading")}</p>
      ) : selection.data ? (
        <MetadataEditor
          key={`${workspace.id}:${selection.data.version}`}
          selection={selection.data}
          choices={options.data?.choices ?? []}
          workspace={workspace.id}
          canManage={permissions.data?.can_manage === true}
          openLibrary={openLibrary}
          saved={async () => {
            setSuccess(true);
            selection.reload();
            await refresh();
          }}
        />
      ) : (
        <CatalogError error={selection.error} retry={selection.reload} />
      )}{" "}
      {success && (
        <p
          role="status"
          className="rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800"
        >
          {t("configuration.saved")}
        </p>
      )}
      {options.loading && (
        <p className="text-xs text-stone-500">
          {t("configuration.loadingChoices")}
        </p>
      )}
      <BindingManager
        key={workspace.id}
        workspace={workspace.id}
        definitions={options.data?.definitions ?? []}
        canManage={permissions.data?.can_manage === true}
        changed={options.reload}
      />
      <Button
        size="sm"
        variant="ghost"
        onClick={() => {
          selection.reload();
          options.reload();
          permissions.reload();
        }}
      >
        {t("configuration.reload")}
      </Button>
    </div>
  );
}
