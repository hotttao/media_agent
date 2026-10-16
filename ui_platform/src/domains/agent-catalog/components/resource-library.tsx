import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useCatalogLoad } from "@/domains/agent-catalog/hooks/use-catalog-load";
import {
  catalogPath,
  listCatalog,
} from "@/domains/agent-catalog/lib/catalog-client";
import type { CatalogResource } from "@/domains/agent-catalog/types";
import {
  managementRequest,
  managementWrite,
} from "@/foundation/lib/management-http";
import type { Team } from "@/foundation/types/hierarchy";
import { AccessEditor } from "./access-editor";
import { CreateResource } from "./create-resource";
import { PromptEditor } from "./prompt-editor";
import { CatalogError, ConfirmAction, inputClass } from "./shared";
import { SkillEditor } from "./skill-editor";
import { UnassignedResources } from "./unassigned-resources";

function ResourceDetail({
  kind,
  id,
  teams,
  changed,
  back,
}: {
  kind: "prompts" | "skills";
  id: string;
  teams: Team[];
  changed(): void;
  back(): void;
}) {
  const { t } = useTranslation();
  const load = useCallback(
    () => managementRequest<CatalogResource>(catalogPath(kind, id)),
    [kind, id],
  );
  const request = useCatalogLoad(load);
  const [current, setCurrent] = useState<CatalogResource>();
  const [access, setAccess] = useState(false);
  const [error, setError] = useState<unknown>();
  const resource = current ?? request.data;
  function saved(value: CatalogResource) {
    setCurrent(value);
    changed();
  }
  if (request.loading && !resource)
    return <p className="p-6 text-sm text-stone-500">{t("catalog.loading")}</p>;
  if (!resource)
    return (
      <div className="p-6">
        <CatalogError error={request.error} retry={request.reload} />
      </div>
    );
  return (
    <section className="min-w-0 space-y-5 p-5 sm:p-7">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <Button
            className="settings-resource-back mb-3"
            variant="ghost"
            size="sm"
            onClick={back}
          >
            {t("catalog.backToList")}
          </Button>
          <h2 className="break-words text-xl font-semibold tracking-tight">
            {resource.name}
          </h2>
          <p className="mt-1 text-xs text-stone-500">
            {t(`catalog.${resource.my_permission}`)} ·{" "}
            {t(`catalog.visibility_${resource.visibility}`)}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {resource.my_permission === "owner" && (
            <>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setAccess(true)}
              >
                {t("catalog.access")}
              </Button>
              {kind === "skills" && (
                <ConfirmAction
                  title={t(
                    resource.status === "active"
                      ? "catalog.disableSkill"
                      : "catalog.enableSkill",
                  )}
                  description={t("catalog.skillStatusConfirm", {
                    name: resource.name,
                  })}
                  onError={setError}
                  action={async () => {
                    saved(
                      await managementWrite<CatalogResource>(
                        `${catalogPath(kind, resource.id)}/status`,
                        "POST",
                        {
                          expected_version: resource.version,
                          status:
                            resource.status === "active"
                              ? "disabled"
                              : "active",
                        },
                      ),
                    );
                  }}
                />
              )}
              <ConfirmAction
                title={t("catalog.archiveAction")}
                description={t("catalog.archiveConfirm", {
                  name: resource.name,
                })}
                onError={setError}
                action={async () => {
                  await managementWrite(
                    `${catalogPath(kind, resource.id)}/archive`,
                    "POST",
                    { expected_version: resource.version },
                  );
                  changed();
                  back();
                }}
              />
            </>
          )}
        </div>
      </header>
      <CatalogError error={error} />
      {kind === "prompts" ? (
        <PromptEditor resource={resource} saved={saved} />
      ) : (
        <SkillEditor resource={resource} saved={saved} />
      )}{" "}
      {access && (
        <AccessEditor
          resource={resource}
          kind={kind}
          teams={teams}
          saved={saved}
          close={() => setAccess(false)}
        />
      )}
    </section>
  );
}
export function ResourceLibrary({
  kind,
  organization,
  organizationName,
  teams,
}: {
  kind: "prompts" | "skills";
  organization: string;
  organizationName: string;
  teams: Team[];
}) {
  const { t } = useTranslation();
  const load = useCallback(
    () => listCatalog(kind, organization),
    [kind, organization],
  );
  const request = useCatalogLoad(load);
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const [create, setCreate] = useState(false);
  const [unassigned, setUnassigned] = useState(false);
  const visible = (request.data ?? []).filter((item) =>
    item.name.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <div className="settings-resource-library flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b bg-white px-5 py-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">
            {t(`catalog.${kind}`)}
          </h1>
          <p className="mt-1 text-sm text-stone-500">
            {t("catalog.libraryHint")}
          </p>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="outline" onClick={() => setUnassigned(true)}>
            {t("catalog.unassigned")}
          </Button>
          <Button onClick={() => setCreate(true)}>
            {t(kind === "prompts" ? "catalog.newPrompt" : "catalog.newSkill")}
          </Button>
        </div>
      </div>
      <div className="settings-resource-grid grid min-h-0 flex-1">
        <aside
          className={`settings-resource-list ${selected ? "resource-selected" : ""} overflow-y-auto border-r bg-stone-50/70 p-4`}
        >
          <input
            aria-label={t("catalog.search")}
            className={`${inputClass} mb-4`}
            placeholder={t("catalog.search")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <CatalogError error={request.error} retry={request.reload} />
          {request.loading && (
            <p className="py-6 text-center text-sm text-stone-500">
              {t("catalog.loading")}
            </p>
          )}
          {!request.loading && !request.error && visible.length === 0 && (
            <div className="py-10 text-center text-sm text-stone-500">
              {t("catalog.empty")}
            </div>
          )}
          <div className="grid gap-2">
            {visible.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => setSelected(item.id)}
                className={`rounded-lg border p-3 text-left transition-colors ${selected === item.id ? "border-amber-300 bg-amber-50" : "border-stone-200 bg-white hover:border-stone-400"}`}
              >
                <span className="block break-words text-sm font-semibold">
                  {item.name}
                </span>
                <span className="mt-2 block text-xs text-stone-500">
                  {kind === "skills"
                    ? (item.published_version ?? 0) > 0
                      ? t("catalog.publishedVersion", {
                          version: item.published_version,
                        })
                      : t("catalog.unpublished")
                    : t("catalog.version", { version: item.version })}{" "}
                  · {t(`catalog.visibility_${item.visibility}`)}
                </span>
                {kind === "skills" && item.has_draft && (
                  <span className="mt-2 inline-block rounded bg-amber-100 px-1.5 py-0.5 text-[11px] text-amber-900">
                    {t("catalog.hasDraft")}
                  </span>
                )}
              </button>
            ))}
          </div>
        </aside>
        <div
          className={`settings-resource-detail ${!selected ? "resource-empty" : ""} min-w-0 overflow-y-auto bg-white`}
        >
          {selected ? (
            <ResourceDetail
              key={`${kind}:${selected}`}
              kind={kind}
              id={selected}
              teams={teams}
              changed={request.reload}
              back={() => setSelected("")}
            />
          ) : (
            <div className="grid min-h-60 place-items-center p-8 text-sm text-stone-500">
              {t("catalog.selectResource")}
            </div>
          )}
        </div>
      </div>
      {unassigned && (
        <UnassignedResources
          kind={kind}
          organization={organization}
          organizationName={organizationName}
          close={() => setUnassigned(false)}
          assigned={(value) => {
            request.reload();
            setSelected(value.id);
          }}
        />
      )}
      {create && (
        <CreateResource
          kind={kind}
          organization={organization}
          close={() => setCreate(false)}
          created={(value) => {
            request.reload();
            setSelected(value.id);
          }}
        />
      )}
    </div>
  );
}
