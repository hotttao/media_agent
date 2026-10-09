import { useQueries, type Query } from "@tanstack/react-query";
import { ArrowUpRight, Ban, Play } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { JobStatusBadge } from "@/domains/jobs/components/status-badge";
import { useCancelJob } from "@/domains/jobs/hooks/use-job-mutations";
import { useJobsList } from "@/domains/jobs/hooks/use-jobs-list";
import { getJob } from "@/domains/jobs/lib/api";
import {
  executionPosition,
  jobPriority,
} from "@/domains/jobs/lib/execution-summary";
import { CANCELABLE_JOB_STATUS, type JobStatus } from "@/domains/jobs/types";
import { useWorkflowsList } from "@/domains/workflows/hooks/use-workflows-list";
import { formatDetailDate } from "@/foundation/components/detail-layout";
import {
  ListPagination,
  ListState,
  ResourceListLayout,
} from "@/foundation/components/resource-list-layout";
import { hostShim } from "@/foundation/lib/host-shim";

const PAGE_SIZE = 20;
const STATUSES: JobStatus[] = [
  "need_confirm",
  "running",
  "pending",
  "awaiting_resume",
  "paused",
  "succeeded",
  "failed",
  "cancelled",
];
export function JobList({
  onSelect,
  onCreate,
  instanceId,
}: {
  onSelect: (id: string) => void;
  onCreate?: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const [status, setStatus] = hostShim.useInstanceState<JobStatus | undefined>(
    instanceId,
    "workflow_jobs_status",
    () => undefined,
  );
  const [page, setPage] = hostShim.useInstanceState(
    instanceId,
    "workflow_jobs_page",
    () => 1,
  );
  const [search, setSearch] = hostShim.useInstanceState(
    instanceId,
    "workflow_jobs_search",
    () => "",
  );
  const [cancelId, setCancelId] = useState<string | null>(null);
  const query = useJobsList({ status, page, page_size: PAGE_SIZE });
  const { data: workflows } = useWorkflowsList({ page_size: 200 });
  const cancelJob = useCancelJob();
  const pageItems = query.data?.items ?? [];
  const workflowNames = new Map(
    (workflows?.items ?? []).map((workflow) => [workflow.id, workflow.name]),
  );
  const items = [...pageItems]
    .filter((job) =>
      `${workflowNames.get(job.workflow_id) ?? job.workflow_id} ${job.id}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    )
    .sort((a, b) => jobPriority(a.status) - jobPriority(b.status));
  const snapshots = useQueries({
    queries: items
      .filter((job) => CANCELABLE_JOB_STATUS.has(job.status))
      .map((job) => ({
        queryKey: ["job", job.id],
        queryFn: () => getJob(job.id),
        staleTime: 10000,
        refetchInterval: (query: Query<Awaited<ReturnType<typeof getJob>>>) =>
          query.state.data &&
          !CANCELABLE_JOB_STATUS.has(query.state.data.job.status)
            ? false
            : 10000,
      })),
  });
  const observed = new Map(
    items
      .filter((job) => CANCELABLE_JOB_STATUS.has(job.status))
      .map((job, index) => [job.id, snapshots[index]]),
  );
  return (
    <>
      <ResourceListLayout
        title={t("v12.jobsTitle")}
        description={t("v12.jobsHint")}
        search={search}
        onSearch={setSearch}
        filterLabel={t("v12.filterStatus")}
        actions={
          onCreate ? (
            <Button onClick={onCreate}>
              <Play className="h-4 w-4" />
              {t("v12.newJob")}
            </Button>
          ) : null
        }
        filters={[undefined, ...STATUSES].map((item) => (
          <button
            key={item ?? "all"}
            type="button"
            className="media-filter-option"
            aria-pressed={status === item}
            onClick={() => {
              setStatus(item);
              setPage(1);
            }}
          >
            {item ? t(`v12.jobStatus.${item}`) : t("v12.all")}
          </button>
        ))}
      >
        <p className="mb-4 text-xs text-muted-foreground">
          {t("v12.pageSearch")}
        </p>
        {query.isLoading ? (
          <ListState loading />
        ) : query.isError ? (
          <ListState
            error={query.error.message || t("v12.loadError")}
            onRetry={() => void query.refetch()}
          />
        ) : items.length === 0 ? (
          <ListState
            empty={search || status ? t("v12.noMatches") : t("v12.emptyJobs")}
          />
        ) : (
          <div className="space-y-3">
            {items.map((job) => {
              const snapshot = observed.get(job.id);
              const currentStatus = snapshot?.data?.job.status || job.status;
              const position = executionPosition(currentStatus, snapshot?.data);
              const done = snapshot?.data?.step_runs.filter(
                (step) => step.status === "succeeded",
              ).length;
              return (
                <article
                  key={job.id}
                  data-testid={`execution-row-${job.id}`}
                  className="media-resource-row"
                >
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="min-w-0 flex-1">
                      <div className="mb-2 flex items-center gap-2">
                        <JobStatusBadge status={currentStatus} />
                        <span className="text-xs text-muted-foreground">
                          v{job.workflow_version}
                        </span>
                      </div>
                      <button
                        type="button"
                        className="flex items-center gap-2 text-left hover:underline"
                        onClick={() => onSelect(job.id)}
                      >
                        <h2 className="text-base font-semibold">
                          {workflowNames.get(job.workflow_id) ??
                            job.workflow_id}
                        </h2>
                        <ArrowUpRight className="h-4 w-4 shrink-0" />
                      </button>
                      <p className="mt-2 text-sm text-muted-foreground">
                        {snapshot?.isError
                          ? t("v12.jobPosition.unavailable")
                          : t(position.key, position.values)}
                        {done !== undefined
                          ? ` · ${t("v12.jobPosition.progress", { done, total: snapshot?.data?.step_runs.length })}`
                          : ""}
                      </p>
                      <p className="mt-3 text-xs text-muted-foreground">
                        {formatDetailDate(job.submitted_at)}
                      </p>
                    </div>
                    {CANCELABLE_JOB_STATUS.has(currentStatus) ? (
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t("v12.cancelJob")}
                        onClick={() => {
                          cancelJob.reset();
                          setCancelId(job.id);
                        }}
                      >
                        <Ban className="h-4 w-4" />
                      </Button>
                    ) : null}
                  </div>
                </article>
              );
            })}
          </div>
        )}
        {!query.isLoading && !query.isError ? (
          <ListPagination
            page={page}
            pageSize={PAGE_SIZE}
            total={query.data?.total ?? 0}
            onPage={(next) => {
              setSearch("");
              setPage(next);
            }}
          />
        ) : null}
      </ResourceListLayout>
      <Dialog
        open={!!cancelId}
        onOpenChange={(open) => {
          if (!open && !cancelJob.isPending) setCancelId(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("v12.cancelJob")}</DialogTitle>
            <DialogDescription>{t("v12.cancelConfirm")}</DialogDescription>
          </DialogHeader>
          {cancelJob.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {cancelJob.error.message || t("v12.cancelError")}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={cancelJob.isPending}
              onClick={() => setCancelId(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              disabled={cancelJob.isPending}
              onClick={() =>
                cancelId &&
                cancelJob.mutate(
                  { jobId: cancelId },
                  { onSuccess: () => setCancelId(null) },
                )
              }
            >
              {t("v12.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
