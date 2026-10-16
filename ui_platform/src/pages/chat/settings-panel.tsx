import {
  FileText,
  KeyRound,
  Library,
  Plug,
  Puzzle,
  Settings,
  SlidersHorizontal,
} from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CatalogManager } from "@/domains/agent-catalog/components/catalog-manager";
import { ModelConnectionManager } from "@/domains/model-connection/components/model-connection-manager";
import type { Workspace } from "@/domains/workspace/types";
import { AgentSettingsPanel } from "./agent-settings-panel";

export type SettingsSection =
  | "general"
  | "configuration"
  | "prompts"
  | "skills"
  | "mcp"
  | "models";
const sections = [
  {
    id: "general",
    group: "workspace",
    icon: Settings,
    label: "settingsNavigation.general",
  },
  {
    id: "configuration",
    group: "workspace",
    icon: SlidersHorizontal,
    label: "settingsNavigation.configuration",
  },
  {
    id: "prompts",
    group: "organization",
    icon: FileText,
    label: "catalog.prompts",
  },
  {
    id: "skills",
    group: "organization",
    icon: Puzzle,
    label: "catalog.skills",
  },
  {
    id: "mcp",
    group: "organization",
    icon: Plug,
    label: "settingsNavigation.mcpCatalog",
  },
  {
    id: "models",
    group: "personal",
    icon: KeyRound,
    label: "modelConnections.title",
  },
] as const;
export function settingsSection(
  value: string | undefined | null,
  fallback: SettingsSection,
): SettingsSection {
  return sections.some((s) => s.id === value)
    ? (value as SettingsSection)
    : fallback;
}
export function SettingsPanel({
  workspace,
  section,
  onSection,
  refresh,
  editWorkspace,
  organizationId,
}: {
  workspace?: Workspace;
  section: SettingsSection;
  onSection(value: SettingsSection): void;
  refresh(): Promise<void>;
  editWorkspace(): void;
  organizationId?: string;
}) {
  const { t } = useTranslation();
  const [selectedOrganization, setSelectedOrganization] = useState(
    workspace?.organization_id ?? organizationId,
  );
  const catalog =
    section === "prompts" || section === "skills" || section === "mcp";
  const available = sections.filter(
    (s) => workspace || s.group !== "workspace",
  );
  return (
    <div
      data-testid="workspace-settings"
      className="agent-settings agent-product flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden text-stone-900"
    >
      <div className="settings-compact-nav shrink-0 border-b p-3">
        <label className="flex items-center gap-3 text-sm">
          <Settings size={16} />
          <span className="sr-only">{t("settingsNavigation.navigation")}</span>
          <select
            aria-label={t("settingsNavigation.navigation")}
            value={section}
            onChange={(e) => onSection(e.target.value as SettingsSection)}
            className="min-w-0 flex-1 rounded-lg border bg-white px-3 py-2"
          >
            {available.map((s) => (
              <option key={s.id} value={s.id}>
                {t(s.label)}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="flex min-h-0 min-w-0 flex-1">
        <nav
          aria-label={t("settingsNavigation.navigation")}
          className="settings-sidebar w-40 shrink-0 overflow-y-auto border-r border-stone-200 bg-stone-50/70 px-2 py-4"
        >
          {["workspace", "organization", "personal"].map((group) => {
            const items = available.filter((s) => s.group === group);
            return (
              items.length > 0 && (
                <div key={group} className="mb-5 last:mb-0">
                  <p className="mb-2 px-2 text-xs font-medium text-stone-500">
                    {t(`settingsNavigation.${group}`)}
                  </p>
                  {items.map(({ id, icon: Icon, label }) => (
                    <button
                      key={id}
                      type="button"
                      aria-current={section === id ? "page" : undefined}
                      onClick={() => onSection(id)}
                      className={`mb-0.5 flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-stone-400 ${section === id ? "bg-stone-200/70 font-semibold text-stone-950" : "text-stone-600 hover:bg-stone-200/40"}`}
                    >
                      <Icon size={15} className="shrink-0" />
                      <span>{t(label)}</span>
                    </button>
                  ))}
                </div>
              )
            );
          })}
        </nav>
        <div
          key={`${workspace?.id ?? "global"}:${section}`}
          data-testid="settings-content"
          className={`settings-content flex min-h-0 min-w-0 flex-1 flex-col ${catalog ? "overflow-hidden" : "overflow-y-auto"}`}
        >
          {catalog ? (
            <CatalogManager
              kind={section}
              organizationId={selectedOrganization}
              onOrganizationChange={setSelectedOrganization}
            />
          ) : (
            <div className="mx-auto w-full max-w-4xl p-5 sm:p-6">
              {section === "models" ? (
                <ModelConnectionManager />
              ) : workspace ? (
                section === "general" ? (
                  <div className="space-y-5">
                    <div>
                      <h2 className="break-words text-xl font-semibold">
                        {workspace.name}
                      </h2>
                      <p className="mt-2 text-sm text-stone-500">
                        {t("settingsNavigation.generalDescription")}
                      </p>
                    </div>
                    <dl className="divide-y rounded-xl border border-stone-200 bg-stone-50/50 px-4 text-sm">
                      <div className="py-4">
                        <dt className="text-xs text-stone-500">
                          {t("settingsNavigation.agent")}
                        </dt>
                        <dd className="mt-1 font-medium capitalize">
                          {workspace.agent_type}
                        </dd>
                      </div>
                      <div className="py-4">
                        <dt className="text-xs text-stone-500">
                          {t("modelConnections.authentication")}
                        </dt>
                        <dd className="mt-1 font-medium">
                          {t(
                            workspace.model_connection_id
                              ? "modelConnections.configured"
                              : workspace.api_key_configured
                                ? "settingsNavigation.configured"
                                : "settingsNavigation.notConfigured",
                          )}
                        </dd>
                      </div>
                    </dl>
                    <AgentSettingsPanel
                      workspace={workspace}
                      section="general"
                      refresh={refresh}
                      editWorkspace={editWorkspace}
                      openLibrary={() => onSection("prompts")}
                    />
                  </div>
                ) : (
                  <AgentSettingsPanel
                    workspace={workspace}
                    refresh={refresh}
                    editWorkspace={editWorkspace}
                    openLibrary={() => onSection("prompts")}
                  />
                )
              ) : (
                <div className="space-y-3 text-sm text-stone-500">
                  <Library size={24} />
                  <p>{t("settingsNavigation.chooseWorkspace")}</p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
