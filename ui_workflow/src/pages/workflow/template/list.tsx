import { ArrowUpRight, Layers3, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import type { TemplateStatus } from "@/domains/templates/types";
import { formatDetailDate } from "@/foundation/components/detail-layout";
import {
  ListPagination,
  ListState,
  ResourceListLayout,
} from "@/foundation/components/resource-list-layout";
import { hostShim } from "@/foundation/lib/host-shim";

const PAGE_SIZE = 20;
export function TemplateList({
  onSelect,
  onCreate,
  instanceId,
}: {
  onSelect: (id: string) => void;
  onCreate?: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const [name, setName] = hostShim.useInstanceState(
    instanceId,
    "workflow_templates_name",
    () => "",
  );
  const [page, setPage] = hostShim.useInstanceState(
    instanceId,
    "workflow_templates_page",
    () => 1,
  );
  const [status, setStatus] = hostShim.useInstanceState<
    TemplateStatus | undefined
  >(instanceId, "workflow_templates_status", () => undefined);
  const query = useTemplatesList({
    name: name || undefined,
    page,
    page_size: PAGE_SIZE,
  });
  // The API has no status filter: never imply these counts cover the full library.
  const items = (query.data?.items ?? []).filter(
    (template) => !status || template.status === status,
  );
  return (
    <ResourceListLayout
      title={t("nav.templates")}
      description={t("v12.templatesHint")}
      search={name}
      onSearch={(value) => {
        setName(value);
        setPage(1);
      }}
      filterLabel={`${t("v12.filterStatus")} · ${t("v12.pageOnly")}`}
      actions={
        onCreate ? (
          <Button onClick={onCreate}>
            <Plus className="h-4 w-4" />
            {t("v12.newTemplate")}
          </Button>
        ) : null
      }
      filters={([undefined, "draft", "published", "archived"] as const).map(
        (item) => (
          <button
            type="button"
            key={item ?? "all"}
            className="media-filter-option"
            aria-pressed={status === item}
            onClick={() => setStatus(item)}
          >
            {item ? t(`v12.definitionStatus.${item}`) : t("v12.all")}
          </button>
        ),
      )}
    >
      {query.isLoading ? (
        <ListState loading />
      ) : query.isError ? (
        <ListState
          error={query.error.message || t("v12.loadError")}
          onRetry={() => void query.refetch()}
        />
      ) : items.length === 0 ? (
        <ListState
          empty={name || status ? t("v12.noMatches") : t("v12.emptyTemplates")}
        />
      ) : (
        <div className="space-y-3">
          {items.map((template) => (
            <article key={template.id} className="media-resource-row">
              <div className="flex flex-wrap items-center justify-between gap-4">
                <div className="flex min-w-0 flex-1 items-center gap-4">
                  <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-muted">
                    <Layers3 className="h-5 w-5" />
                  </span>
                  <div className="min-w-0">
                    <button
                      type="button"
                      className="text-left hover:underline"
                      onClick={() => onSelect(template.id)}
                    >
                      <h2 className="text-base font-semibold">
                        {template.name}
                      </h2>
                    </button>
                    <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">
                      {template.description || t("v12.noDescription")}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className="rounded-full bg-muted px-2.5 py-1 text-xs">
                    v{template.version}
                  </span>
                  <span
                    className={`rounded-full px-2.5 py-1 text-xs ${template.status === "published" ? "bg-success/10 text-success" : "bg-accent/20 text-warning"}`}
                  >
                    {t(`v12.definitionStatus.${template.status}`)}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`${t("v12.view")} ${template.name}`}
                    onClick={() => onSelect(template.id)}
                  >
                    <ArrowUpRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              <div className="mt-4 flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground">
                <span>
                  {t("v12.source")}:{" "}
                  {template.capability_key
                    ? `${template.capability_key}@${template.capability_version}`
                    : template.platform}
                </span>
                <span>
                  {t("v12.inputs")}: {template.inputs.length}
                </span>
                <span>
                  {t("v12.scope")}: {template.scope}
                </span>
                <span>{formatDetailDate(template.updated_at)}</span>
              </div>
            </article>
          ))}
        </div>
      )}
      {!query.isLoading && !query.isError ? (
        <ListPagination
          page={page}
          pageSize={PAGE_SIZE}
          total={query.data?.total ?? 0}
          onPage={setPage}
        />
      ) : null}
    </ResourceListLayout>
  );
}
