import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Info, RefreshCw, X } from "lucide-react";
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
  const definition = useWorkflow(
    query.data?.job.workflow_id,
    query.data?.job.workflow_version,
  );
  const { data: workflow } = definition;
  const [selectedKey, setSelectedKey] = hostShim.useInstanceState<
    string | null
  >(instanceId, `workflow_job_selected_${id}`, () => null);
  const [panelOpen, setPanelOpen] = hostShim.useInstanceState<boolean>(
    instanceId,
    `workflow_job_panel_open_${id}`,
    () => false,
  );
  const closeButton = useRef<HTMLButtonElement>(null);
  const canvas = useRef<HTMLDivElement>(null);
  const selectionTrigger = useRef<HTMLElement | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [action, setAction] = useState<"cancel" | "retry" | null>(null);
  const cancel = useCancelJob();
  const retry = useRetryJob();
  const steps = [...(query.data?.step_runs ?? [])].sort(
    (a, b) => a.step_order - b.step_order,
  );
  const focused = steps.find((step) => step.step_key === selectedKey);
  const asset = useGeneratedAsset(focused?.output_asset_id);
  const shell = hostShim.useShellContext();
  const organizationId =
    query.data?.job.organization_id || shell.organization.id;
  useEffect(() => {
    if (panelOpen && selectedKey) closeButton.current?.focus();
  }, [panelOpen, selectedKey]);
  function selectStep(key: string) {
    selectionTrigger.current =
      document.activeElement instanceof HTMLButtonElement
        ? document.activeElement
        : null;
    setSelectedKey(key);
    setPanelOpen(true);
  }
  function closePanel() {
    setPanelOpen(false);
    if (selectionTrigger.current?.isConnected) selectionTrigger.current.focus();
    else canvas.current?.focus();
  }
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
  const reviewStep =
    job.status === "need_confirm"
      ? steps.find((step) => step.status === "need_confirm")
      : undefined;
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
          {job.status === "failed" &&
          job.origin !== "legacy_import" ? (
            <Button size="sm" variant="outline" onClick={onRerun}>
              {t("runForm.rerun")}
            </Button>
          ) : null}
        </div>
      </AppStudioHeader>
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 text-xs">
        <p className="font-medium">{t(position.key, position.values)}</p>
        {reviewStep ? (
          <Button
            size="sm"
            variant="outline"
            onClick={() => selectStep(reviewStep.step_key)}
          >
            {t("execution.inspectApproval")}
          </Button>
        ) : null}
        <span className="text-muted-foreground">
          v{job.workflow_version} ·{" "}
          {t("execution.progress", {
            completed,
            total: hasGraph && workflow ? workflow.steps.length : steps.length,
          })}
        </span>
      </div>
      {query.isRefetchError ? (
        <p role="alert" className="px-4 py-2 text-xs text-destructive">
          {t("execution.refreshError")}
        </p>
      ) : null}
      {job.failure_reason ? (
        <p
          role="alert"
          className="border-b bg-destructive/5 px-4 py-2 text-sm text-destructive"
        >
          {job.failure_reason}
        </p>
      ) : null}
      <div
        className={`execution-workbench${panelOpen ? " has-inspector" : ""}`}
      >
        <div
          ref={canvas}
          role="region"
          tabIndex={-1}
          className="execution-main"
          aria-label={t("execution.graph")}
        >
          {definition.isLoading ? <ListState loading /> : null}
          {definition.isError ? (
            <ListState
              error={definition.error.message}
              onRetry={() => void definition.refetch()}
            />
          ) : null}
          {hasGraph && workflow ? (
            <WorkflowGraph
              model={{
                steps: workflow.steps,
                outputStepKey: workflow.output_step_key,
                layout: "studio",
              }}
              className="absolute inset-0"
              selectedNodeId={selectedKey || undefined}
              onNodeSelect={selectStep}
              executionStatus={Object.fromEntries(
                steps.map((step) => [step.step_key, step.status]),
              )}
            />
          ) : (
            <div className="flex h-full min-h-0 flex-col gap-4 overflow-auto p-4">
              <p
                role="status"
                className="text-xs leading-5 text-muted-foreground"
              >
                {t("execution.graphFallback")}
              </p>
              <nav
                aria-label={t("execution.nodes")}
                className="execution-steps"
              >
                {steps.map((step) => (
                  <button
                    type="button"
                    key={step.id}
                    aria-current={
                      selectedKey === step.step_key ? "step" : undefined
                    }
                    className="execution-step"
                    onClick={() => selectStep(step.step_key)}
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
              </nav>
            </div>
          )}
        </div>
        {panelOpen ? (
          <aside
            className="execution-inspector"
            aria-label={t("execution.nodeResults")}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.stopPropagation();
                closePanel();
              }
            }}
          >
            <div className="flex shrink-0 items-center justify-between gap-2 border-b p-3">
              <h2 className="truncate text-sm font-semibold">
                {selectedKey || t("execution.nodeResults")}
              </h2>
              <Button
                ref={closeButton}
                size="icon"
                variant="ghost"
                aria-label={t("execution.closeResults")}
                onClick={closePanel}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="min-h-0 overflow-y-auto px-2 py-3">
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
          </aside>
        ) : null}
      </div>
      <Dialog open={detailsOpen} onOpenChange={setDetailsOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("execution.details")}</DialogTitle>
            <DialogDescription>
              {t("execution.inputSnapshot")}
            </DialogDescription>
          </DialogHeader>
          {job.origin === "legacy_import" ? (
            <p className="text-xs text-muted-foreground">
              {t("execution.imported")}
            </p>
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t("execution.stateAuthority")}
          </p>
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
