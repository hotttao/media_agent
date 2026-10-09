import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Info, Network, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  JobStatusBadge,
  StepRunStatusBadge,
} from "@/domains/jobs/components/status-badge";
import { StepRunCard } from "@/domains/jobs/components/step-run-card";
import { useJob } from "@/domains/jobs/hooks/use-job";
import {
  useCancelJob,
  useRetryJob,
} from "@/domains/jobs/hooks/use-job-mutations";
import { executionPosition } from "@/domains/jobs/lib/execution-summary";
import {
  CANCELABLE_JOB_STATUS,
  RETRYABLE_JOB_STATUS,
} from "@/domains/jobs/types";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import { WorkflowGraph } from "@/domains/workflows/components/workflow-graph";
import { useGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import { LibraryMedia } from "@/foundation/components/library-media";
import { ListState } from "@/foundation/components/resource-list-layout";
import { formatDetailDate } from "@/foundation/components/detail-layout";
import { hostShim } from "@/foundation/lib/host-shim";

export function JobShow({
  id,
  onBack,
  instanceId,
  onResume,
  onRerun,
}: {
  id: string;
  onBack: () => void;
  instanceId: string;
  onResume: () => void;
  onRerun: () => void;
}) {
  const { t } = useTranslation();
  const query = useJob(id);
  const { data: workflow } = useWorkflow(query.data?.job.workflow_id);
  const [selectedKey, setSelectedKey] = hostShim.useInstanceState<
    string | null
  >(instanceId, `workflow_job_selected_${id}`, () => null);
  const [graphOpen, setGraphOpen] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [action, setAction] = useState<"cancel" | "retry" | null>(null);
  const cancel = useCancelJob();
  const retry = useRetryJob();
  const steps = [...(query.data?.step_runs ?? [])].sort(
    (a, b) => a.step_order - b.step_order,
  );
  const focused =
    steps.find((step) => step.step_key === selectedKey) ??
    steps.find((step) => step.status === "need_confirm") ??
    steps.find((step) =>
      ["running", "claimed", "failed", "pending"].includes(step.status),
    ) ??
    steps.at(-1);
  const asset = useGeneratedAsset(focused?.output_asset_id);
  const shell = hostShim.useShellContext();
  const organizationId =
    query.data?.job.organization_id || shell.organization.id;
  if (query.isLoading) return <ListState loading />;
  if (query.isError)
    return (
      <ListState
        error={query.error.message}
        onRetry={() => void query.refetch()}
      />
    );
  if (!query.data) return <ListState empty={t("execution.notFound")} />;
  const { job } = query.data;
  const completed = steps.filter((step) => step.status === "succeeded").length;
  const position = executionPosition(job.status, query.data);
  const hasGraph = Boolean(
    workflow && workflow.version === job.workflow_version,
  );
  const operationPending = cancel.isPending || retry.isPending;
  return (
    <main className="flex h-full min-h-0 flex-col overflow-hidden">
      <AppStudioHeader>
        <div className="flex min-w-0 items-center gap-3">
          <Button
            size="icon"
            variant="ghost"
            aria-label={t("execution.back")}
            onClick={onBack}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <h1 className="truncate text-base font-semibold">
            {workflow?.name || t("execution.title")}
          </h1>
          <JobStatusBadge status={job.status} />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setDetailsOpen(true)}
          >
            <Info className="mr-1 h-4 w-4" />
            {t("execution.details")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void query.refetch()}
            aria-label={t("execution.refresh")}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          {RETRYABLE_JOB_STATUS.has(job.status) ? (
            <Button
              size="sm"
              disabled={operationPending}
              onClick={() => setAction("retry")}
            >
              {t("execution.retry")}
            </Button>
          ) : null}
          {CANCELABLE_JOB_STATUS.has(job.status) ? (
            <Button
              size="sm"
              variant="outline"
              disabled={operationPending}
              onClick={() => setAction("cancel")}
            >
              {t("execution.cancel")}
            </Button>
          ) : null}
          {job.status === "awaiting_resume" ? (
            <Button size="sm" onClick={onResume}>
              {t("runForm.resume")}
            </Button>
          ) : null}
          {["failed", "cancelled", "succeeded"].includes(job.status) &&
          job.origin !== "legacy_import" ? (
            <Button size="sm" variant="outline" onClick={onRerun}>
              {t("runForm.rerun")}
            </Button>
          ) : null}
        </div>
      </AppStudioHeader>
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 text-xs">
        <p className="font-medium">{t(position.key, position.values)}</p>
        <span className="text-muted-foreground">
          v{job.workflow_version} ·{" "}
          {t("execution.progress", { completed, total: steps.length })}
        </span>
      </div>
      {query.isRefetchError ? (
        <p role="alert" className="px-4 py-2 text-xs text-destructive">
          {t("execution.refreshError")}
        </p>
      ) : null}
      <div className="execution-studio">
        <nav aria-label={t("execution.nodes")} className="execution-steps">
          {steps.map((step) => (
            <button
              type="button"
              key={step.id}
              aria-current={focused?.id === step.id ? "step" : undefined}
              className="execution-step"
              onClick={() => setSelectedKey(step.step_key)}
            >
              <span className="block text-sm font-semibold">
                {step.step_key}
              </span>
              <span className="mt-2 block">
                <StepRunStatusBadge status={step.status} />
              </span>
              {step.output_asset_id ? (
                <span className="mt-2 block text-xs text-muted-foreground">
                  {t("execution.resultReady")}
                </span>
              ) : null}
            </button>
          ))}
          {!steps.length ? (
            <p className="p-4 text-xs text-muted-foreground">
              {t("execution.notStarted")}
            </p>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            className="mt-4"
            onClick={() => setGraphOpen(true)}
          >
            <Network className="mr-1 h-4 w-4" />
            {t("execution.graph")}
          </Button>
        </nav>
        <div className="min-w-0 overflow-y-auto px-2 py-3">
          {job.failure_reason ? (
            <p
              role="alert"
              className="m-4 rounded-xl bg-destructive/10 p-4 text-sm text-destructive"
            >
              {job.failure_reason}
            </p>
          ) : null}
          {focused ? (
            <>
              {(asset.data?.object_path || asset.data?.url) &&
              !focused.candidates.length ? (
                <div className="p-4">
                  <LibraryMedia
                    organizationId={organizationId}
                    objectPath={asset.data?.object_path || asset.data?.url}
                    alt={t("execution.accepted")}
                  />
                </div>
              ) : null}
              <StepRunCard
                key={focused.id}
                jobId={id}
                stepRun={focused}
                instanceId={instanceId}
                reviewAllowed={job.status === "need_confirm"}
              />
            </>
          ) : (
            <ListState empty={t("execution.notStarted")} />
          )}
        </div>
        <aside className="execution-context">
          <h2 className="text-sm font-semibold">{t("execution.inputs")}</h2>
          {[
            job.input.product?.name,
            job.input.virtual_ip?.name,
            ...(job.input.material ?? []).map(
              (item) => item.name || item.content,
            ),
            ...(job.input.asset ?? []).map((item) => item.name),
          ]
            .filter(Boolean)
            .map((name, index) => (
              <p
                key={index}
                className="rounded-xl bg-background/70 p-3 text-xs leading-5"
              >
                {name}
              </p>
            ))}
          <h2 className="pt-4 text-sm font-semibold">
            {t("execution.submitted")}
          </h2>
          <p className="text-xs text-muted-foreground">
            {formatDetailDate(job.submitted_at)}
          </p>
          {job.origin === "legacy_import" ? (
            <p className="text-xs leading-5 text-muted-foreground">
              {t("execution.imported")}
            </p>
          ) : null}
          <p className="pt-4 text-xs leading-6 text-muted-foreground">
            {t("execution.stateAuthority")}
          </p>
        </aside>
      </div>
      <Dialog open={graphOpen} onOpenChange={setGraphOpen}>
        <DialogContent className="flex h-[80vh] max-w-5xl flex-col">
          <DialogHeader>
            <DialogTitle>{t("execution.graph")}</DialogTitle>
            <DialogDescription>
              {hasGraph
                ? t("execution.graphHint")
                : t("execution.graphUnavailable")}
            </DialogDescription>
          </DialogHeader>
          {hasGraph && workflow ? (
            <div className="workflow-workbench-canvas flex-1">
              <WorkflowGraph
                model={{
                  steps: workflow.steps,
                  outputStepKey: workflow.output_step_key,
                  layout: "studio",
                }}
                className="absolute inset-0"
                selectedNodeId={focused?.step_key}
                onNodeSelect={(key) => {
                  setSelectedKey(key);
                  setGraphOpen(false);
                }}
                executionStatus={Object.fromEntries(
                  steps.map((step) => [step.step_key, step.status]),
                )}
              />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t("execution.graphFallback")}
            </p>
          )}
        </DialogContent>
      </Dialog>
      <Dialog open={detailsOpen} onOpenChange={setDetailsOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("execution.details")}</DialogTitle>
            <DialogDescription>
              {t("execution.inputSnapshot")}
            </DialogDescription>
          </DialogHeader>
          <dl className="grid gap-3 text-sm">
            <dt>{t("execution.submitted")}</dt>
            <dd>{formatDetailDate(job.submitted_at)}</dd>
            <dt>{t("v12.scope")}</dt>
            <dd>{job.scope}</dd>
            <dt>{t("v12.version")}</dt>
            <dd>v{job.workflow_version}</dd>
            <dt>{t("execution.reference")}</dt>
            <dd className="break-all text-xs text-muted-foreground">
              {job.id}
            </dd>
          </dl>
          <section className="space-y-4" aria-label={t("execution.inputs")}>
            <h2 className="text-sm font-semibold">{t("execution.inputs")}</h2>
            {[
              {
                key: "product",
                name: job.input.product?.name,
                content: job.input.product?.product_details,
                urls: [job.input.product?.image_url],
              },
              {
                key: "virtual_ip",
                name: job.input.virtual_ip?.name,
                urls: [
                  job.input.virtual_ip?.avatar_url,
                  job.input.virtual_ip?.full_body_url,
                  job.input.virtual_ip?.three_view_url,
                  job.input.virtual_ip?.nine_view_url,
                ],
              },
              ...(job.input.material || []).map((item, index) => ({
                key: `material-${index}`,
                name: item.name,
                content: item.content,
                urls: [item.url],
              })),
              ...(job.input.asset || []).map((item, index) => ({
                key: `asset-${index}`,
                name: item.name,
                content: item.content,
                urls: [item.url],
              })),
            ]
              .filter(
                (item) => item.name || item.content || item.urls.some(Boolean),
              )
              .map((item) => (
                <article
                  key={item.key}
                  className="space-y-2 rounded-xl bg-muted/40 p-3"
                >
                  <h3 className="text-sm font-medium">
                    {item.name || item.key}
                  </h3>
                  {item.content ? (
                    <p className="whitespace-pre-wrap text-xs leading-6">
                      {item.content}
                    </p>
                  ) : null}
                  {item.urls.filter(Boolean).map((url, index) => (
                    <LibraryMedia
                      key={index}
                      organizationId={
                        job.organization_id || shell.organization.id
                      }
                      objectPath={url}
                      alt={item.name || item.key}
                    />
                  ))}
                </article>
              ))}
          </section>
        </DialogContent>
      </Dialog>
      <Dialog
        open={action !== null}
        onOpenChange={(open) => {
          if (!open && !operationPending) setAction(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t(action === "retry" ? "execution.retry" : "execution.cancel")}
            </DialogTitle>
            <DialogDescription>
              {t(
                action === "retry"
                  ? "execution.retryHint"
                  : "execution.cancelHint",
              )}
            </DialogDescription>
          </DialogHeader>
          {cancel.error || retry.error ? (
            <p role="alert" className="text-sm text-destructive">
              {cancel.error?.message || retry.error?.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              disabled={operationPending}
              onClick={() => setAction(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              disabled={operationPending}
              onClick={() => {
                const callbacks = { onSuccess: () => setAction(null) };
                if (action === "retry") retry.mutate({ jobId: id }, callbacks);
                else cancel.mutate({ jobId: id }, callbacks);
              }}
            >
              {t("execution.proceed")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </main>
  );
}
