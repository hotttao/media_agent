import { ArrowUpRight, GitBranch, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useWorkflowsList } from "@/domains/workflows/hooks/use-workflows-list";
import type { WorkflowStatus } from "@/domains/workflows/types";
import {
  ResourceListLayout,
  ListPagination,
  ListState,
} from "@/foundation/components/resource-list-layout";
import { formatDetailDate } from "@/foundation/components/detail-layout";
import { hostShim } from "@/foundation/lib/host-shim";

const PAGE_SIZE = 20;
const STATUSES: WorkflowStatus[] = [
  "draft",
  "publishing",
  "publish_failed",
  "published",
  "archived",
];

export function WorkflowList({
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
    "workflow_list_name",
    () => "",
  );
  const [status, setStatus] = hostShim.useInstanceState<
    WorkflowStatus | undefined
  >(instanceId, "workflow_list_status", () => undefined);
  const [page, setPage] = hostShim.useInstanceState(
    instanceId,
    "workflow_list_page",
    () => 1,
  );
  const query = useWorkflowsList({
    name: name || undefined,
    status,
    page,
    page_size: PAGE_SIZE,
  });
  const items = query.data?.items ?? [];
  return (
    <ResourceListLayout
      title={t("nav.workflows")}
      description={t("v12.workflowsHint")}
      search={name}
      onSearch={(value) => {
        setName(value);
        setPage(1);
      }}
      filterLabel={t("v12.filterStatus")}
      actions={
        onCreate ? (
          <Button onClick={onCreate}>
            <Plus className="h-4 w-4" />
            {t("v12.newWorkflow")}
          </Button>
        ) : null
      }
      filters={[undefined, ...STATUSES].map((item) => (
        <button
          type="button"
          key={item ?? "all"}
          className="media-filter-option"
          aria-pressed={status === item}
          onClick={() => {
            setStatus(item);
            setPage(1);
          }}
        >
          {item ? t(`v12.definitionStatus.${item}`) : t("v12.all")}
        </button>
      ))}
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
          empty={name || status ? t("v12.noMatches") : t("v12.emptyWorkflows")}
        />
      ) : (
        <div className="space-y-3">
          {items.map((workflow) => (
            <article key={workflow.id} className="media-resource-row">
              <div className="flex flex-wrap items-center justify-between gap-4">
                <div className="flex min-w-0 flex-1 items-center gap-4">
                  <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-accent/25">
                    <GitBranch className="h-5 w-5" />
                  </span>
                  <div className="min-w-0">
                    <button
                      type="button"
                      className="text-left hover:underline"
                      onClick={() => onSelect(workflow.id)}
                    >
                      <h2 className="text-base font-semibold">
                        {workflow.name}
                      </h2>
                    </button>
                    <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">
                      {workflow.description || t("v12.noDescription")}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className="rounded-full bg-muted px-2.5 py-1 text-xs">
                    v{workflow.version}
                  </span>
                  <span
                    className={`rounded-full px-2.5 py-1 text-xs ${workflow.status === "published" ? "bg-success/10 text-success" : workflow.status === "publish_failed" ? "bg-destructive/10 text-destructive" : "bg-accent/20 text-warning"}`}
                  >
                    {t(`v12.definitionStatus.${workflow.status}`)}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`${t("v12.view")} ${workflow.name}`}
                    onClick={() => onSelect(workflow.id)}
                  >
                    <ArrowUpRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
                {workflow.steps.length > 0 ? (
                  <span>
                    {t("v12.nodes", { count: workflow.steps.length })}
                  </span>
                ) : null}
                <span>
                  {t("v12.scope")}: {workflow.scope}
                </span>
                <span>
                  {t("v12.output")}: {workflow.output_step_key || "—"}
                </span>
                <span>{formatDetailDate(workflow.updated_at)}</span>
              </div>
            </article>
          ))}
        </div>
      )}
      {!query.isError && !query.isLoading ? (
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
