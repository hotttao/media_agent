import { useQueries, type Query } from "@tanstack/react-query";
import {
  ArrowLeft,
  Boxes,
  CalendarClock,
  Eye,
  GitBranch,
  Layers3,
  Pencil,
  Play,
  Send,
  Trash2,
  Settings2,
} from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { JobStatusBadge } from "@/domains/jobs/components/status-badge";
import { useJobsList } from "@/domains/jobs/hooks/use-jobs-list";
import { getJob } from "@/domains/jobs/lib/api";
import { CANCELABLE_JOB_STATUS } from "@/domains/jobs/types";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { WorkflowGraph } from "@/domains/workflows/components/workflow-graph";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import {
  useArchiveWorkflow,
  usePublishWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import type { WorkflowStep } from "@/domains/workflows/types";
import { cn } from "@/foundation/lib/utils";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import { ListState } from "@/foundation/components/resource-list-layout";
import { ResourceSharing } from "@/foundation/components/resource-sharing";
import { hostShim } from "@/foundation/lib/host-shim";
import {
  changeWorkflowVisibility,
  grantWorkflow,
  revokeWorkflow,
} from "@/domains/workflows/lib/api";

export function WorkflowShow({
  id,
  onBack,
  onEdit,
  onOpenJob,
  onRun,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
  onOpenJob: (jobId: string) => void;
  onRun: () => void;
}) {
  const { t } = useTranslation();
  const { user } = hostShim.useCurrentUser();
  const shell = hostShim.useShellContext();
  const {
    data: workflow,
    isLoading,
    isError,
    error,
    refetch,
  } = useWorkflow(id);
  const [executionsOpen, setExecutionsOpen] = useState(false);
  const { data: templates } = useTemplatesList({
    scope: workflow?.scope,
    page_size: 200,
  });
  const executions = useJobsList(
    { workflow_id: id, page_size: 100 },
    executionsOpen,
  );
  const jobsPage = executions.data;
  const activeJobs = (jobsPage?.items ?? []).filter((job) =>
    CANCELABLE_JOB_STATUS.has(job.status),
  );
  const activeDetails = useQueries({
    queries: activeJobs.map((job) => ({
      queryKey: ["job", job.id],
      queryFn: () => getJob(job.id),
      enabled: executionsOpen,
      staleTime: 10000,
      refetchInterval: (query: Query<Awaited<ReturnType<typeof getJob>>>) =>
        query.state.data &&
        !CANCELABLE_JOB_STATUS.has(query.state.data.job.status)
          ? false
          : 10000,
    })),
  });
  const archiveWorkflow = useArchiveWorkflow();
  const publishWorkflow = usePublishWorkflow();
  const [selectedStepKey, setSelectedStepKey] = useState<string>();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);

  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!workflow)
    return (
      <div className="p-8 text-sm text-muted-foreground">
        {t("workflowView.notFound")}
      </div>
    );

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-background">
      <div className="flex min-h-0 flex-1 flex-col">
        <AppStudioHeader>
          <div className="flex min-w-0 items-start gap-3">
            <Button
              variant="outline"
              size="icon"
              onClick={onBack}
              aria-label={t("workflowView.back")}
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="max-w-[30vw] truncate text-base font-semibold">
                  {workflow.name}
                </h1>
                <span
                  className={cn(
                    "rounded-full px-2.5 py-1 text-[11px] font-semibold",
                    workflow.status === "published"
                      ? "bg-emerald-100 text-emerald-700"
                      : workflow.status === "archived"
                        ? "bg-slate-200 text-slate-600"
                        : "bg-amber-100 text-amber-700",
                  )}
                >
                  {t(`v12.definitionStatus.${workflow.status}`)}
                </span>
                <span className="rounded-full bg-primary/10 px-2.5 py-1 text-[11px] font-semibold text-primary">
                  v{workflow.version}
                </span>
              </div>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setSettingsOpen(true)}
            >
              <Settings2 className="mr-1.5 h-4 w-4" />
              {t("studio.settings")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setExecutionsOpen(true)}
            >
              {t("studio.viewExecutions")}
            </Button>
            {workflow.status === "published" &&
            workflow.publication_status === "registered" ? (
              <Button onClick={onRun}>
                <Play className="mr-2 h-4 w-4" />
                {t("workflowView.run")}
              </Button>
            ) : null}
            {workflow.status === "draft" && workflow.created_by === user?.id ? (
              <Button
                onClick={() => publishWorkflow.mutate(id)}
                disabled={publishWorkflow.isPending}
              >
                <Send className="mr-2 h-4 w-4" />
                {t("workflowView.publish")}
              </Button>
            ) : null}
            <Button variant="outline" onClick={onEdit}>
              <Pencil className="mr-2 h-4 w-4" />
              {t("workflowView.edit")}
            </Button>
          </div>
        </AppStudioHeader>

        <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("studio.settings")}</DialogTitle>
              <DialogDescription>
                {workflow.description || t("studio.noDescription")}
              </DialogDescription>
            </DialogHeader>
            <section className="grid gap-3 sm:grid-cols-2">
              <InfoCard
                icon={GitBranch}
                label={t("workflowView.steps")}
                value={t("v12.nodes", { count: workflow.steps.length })}
              />
              <InfoCard
                icon={Boxes}
                label={t("workflowView.output")}
                value={workflow.output_step_key}
              />
              <InfoCard
                icon={Eye}
                label={t("workflowView.visibility")}
                value={t(`common.visibility.${workflow.visibility}`)}
              />
              <InfoCard
                icon={CalendarClock}
                label={t("workflowView.scope")}
                value={workflow.scope}
              />
            </section>
            <ResourceSharing
              organizationId={workflow.organization_id || shell.organization.id}
              owner={Boolean(user?.id && user.id === workflow.created_by)}
              visibility={workflow.visibility}
              onVisibility={async (value) => {
                await changeWorkflowVisibility(id, value);
                await refetch();
              }}
              onGrant={(team, permission) =>
                grantWorkflow(id, team, permission)
              }
              onRevoke={(team) => revokeWorkflow(id, team)}
            />
            {workflow.status !== "archived" ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => {
                  setSettingsOpen(false);
                  setArchiveOpen(true);
                }}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                {t("studio.archive")}
              </Button>
            ) : null}
          </DialogContent>
        </Dialog>

        <Dialog open={executionsOpen} onOpenChange={setExecutionsOpen}>
          <DialogContent className="max-h-[80vh] max-w-2xl overflow-y-auto">
            <DialogHeader>
              <DialogTitle>
                {t("studio.activeExecutions", { count: activeJobs.length })}
              </DialogTitle>
              <DialogDescription>{t("studio.executionHint")}</DialogDescription>
            </DialogHeader>
            {executions.isLoading ? (
              <ListState loading />
            ) : executions.isError ? (
              <ListState
                error={executions.error.message}
                onRetry={() => void executions.refetch()}
              />
            ) : activeJobs.length > 0 ? (
              <section className="rounded-2xl border border-border/70 bg-card p-5">
                <h2 className="font-semibold">
                  {t("studio.activeExecutions", { count: activeJobs.length })}
                </h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("workflowView.executionHint")}
                </p>
                <div className="mt-4 grid gap-3 sm:grid-cols-2">
                  {activeJobs.map((job, index) => {
                    const stepRuns =
                      activeDetails[index]?.data?.step_runs ?? [];
                    const focus =
                      stepRuns.find((step) => step.status === "need_confirm") ??
                      stepRuns.find(
                        (step) =>
                          step.status === "running" ||
                          step.status === "claimed",
                      ) ??
                      stepRuns.find((step) => step.status === "pending");
                    return (
                      <button
                        key={job.id}
                        type="button"
                        onClick={() => onOpenJob(job.id)}
                        className="rounded-xl border border-border p-4 text-left transition-colors hover:border-primary hover:bg-primary/5"
                      >
                        <JobStatusBadge status={job.status} />
                        <span className="mt-2 block text-sm font-medium">
                          {focus
                            ? `${focus.step_key} · ${focus.status === "need_confirm" ? t("workflowView.approval") : focus.status === "pending" ? t("workflowView.pending") : t("workflowView.running")}`
                            : t("workflowView.loadingSteps")}
                        </span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {
                            stepRuns.filter(
                              (step) => step.status === "succeeded",
                            ).length
                          }
                          /{stepRuns.length} {t("workflowView.stepsCompleted")}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </section>
            ) : (
              <p className="py-6 text-sm text-muted-foreground">
                {t("studio.noActiveExecutions")}
              </p>
            )}
          </DialogContent>
        </Dialog>

        {publishWorkflow.error || archiveWorkflow.error ? (
          <p role="alert" className="px-4 py-2 text-sm text-destructive">
            {publishWorkflow.error?.message || archiveWorkflow.error?.message}
          </p>
        ) : null}
        <section
          className={cn(
            "workflow-definition-studio",
            selectedStepKey && "has-selection",
          )}
        >
          <div className="workflow-workbench-canvas">
            <WorkflowGraph
              model={{
                steps: workflow.steps,
                layout: "studio",
                outputStepKey: workflow.output_step_key,
                templateNameByID: Object.fromEntries(
                  (templates?.items ?? []).map((template) => [
                    template.id,
                    template.name,
                  ]),
                ),
              }}
              className="absolute inset-0"
              selectedNodeId={selectedStepKey}
              onNodeSelect={setSelectedStepKey}
            />
            <div className="pointer-events-none absolute bottom-4 left-4 max-w-[80%] rounded-full border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground">
              {t("studio.canvasLegend")}
            </div>
          </div>
          {selectedStepKey ? (
            <div className="workflow-workbench-panel">
              <Button
                size="sm"
                variant="ghost"
                className="m-2 self-end"
                onClick={() => setSelectedStepKey(undefined)}
              >
                {t("editor.closePanel")}
              </Button>
              <StepDetails
                step={workflow.steps.find(
                  (item) => item.key === selectedStepKey,
                )}
                isOutput={selectedStepKey === workflow.output_step_key}
                templateName={
                  templates?.items.find(
                    (template) =>
                      template.id ===
                      workflow.steps.find(
                        (step) => step.key === selectedStepKey,
                      )?.template_id,
                  )?.name
                }
              />
            </div>
          ) : null}
        </section>

        <Dialog
          open={archiveOpen}
          onOpenChange={(open) => {
            if (!archiveWorkflow.isPending) setArchiveOpen(open);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("studio.archive")}</DialogTitle>
              <DialogDescription>{t("studio.archiveHint")}</DialogDescription>
            </DialogHeader>
            {archiveWorkflow.error ? (
              <p role="alert" className="text-sm text-destructive">
                {archiveWorkflow.error.message}
              </p>
            ) : null}
            <DialogFooter>
              <Button
                variant="outline"
                disabled={archiveWorkflow.isPending}
                onClick={() => setArchiveOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="destructive"
                disabled={archiveWorkflow.isPending}
                onClick={() =>
                  archiveWorkflow.mutate(id, { onSuccess: onBack })
                }
              >
                {t("studio.archive")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}

function StepDetails({
  step,
  isOutput,
  templateName,
}: {
  step?: WorkflowStep;
  isOutput: boolean;
  templateName?: string;
}) {
  const { t } = useTranslation();
  return (
    <aside className="border-t bg-background/70 p-5 xl:border-l xl:border-t-0">
      {!step ? (
        <div className="flex h-full min-h-[180px] flex-col items-center justify-center text-center">
          <Layers3 className="h-7 w-7 text-muted-foreground/50" />
          <h3 className="mt-3 font-semibold">{t("workflowView.choose")}</h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t("workflowView.chooseHint")}
          </p>
        </div>
      ) : (
        <div>
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-primary">
                Step
              </p>
              <h3 className="mt-1 text-lg font-semibold">{step.key}</h3>
            </div>
            {isOutput ? (
              <span className="rounded-full bg-primary/10 px-2 py-1 text-[10px] font-semibold text-primary">
                {t("workflowView.output")}
              </span>
            ) : null}
          </div>
          <dl className="mt-6 space-y-4 text-sm">
            <div>
              <dt className="text-xs text-muted-foreground">Template</dt>
              <dd className="mt-1 break-all font-medium">
                {templateName || step.template_id} · v
                {step.template_version ?? "latest"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">
                {t("workflowView.approvalPolicy")}
              </dt>
              <dd className="mt-1 font-medium">
                {step.approval_policy === "always"
                  ? t("workflowView.always")
                  : t("workflowView.multiple")}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">
                {t("workflowView.bindings")}
              </dt>
              <dd className="mt-2 space-y-2">
                {step.slot_bindings.length === 0 ? (
                  <span className="text-muted-foreground">
                    {t("workflowView.emptyBindings")}
                  </span>
                ) : (
                  step.slot_bindings.map((binding) => (
                    <div
                      key={`${binding.to_input}-${binding.ref}`}
                      className="rounded-lg border bg-muted/40 px-3 py-2 text-xs"
                    >
                      <strong>{binding.to_input}</strong>
                      <span className="mx-1.5 text-muted-foreground">←</span>
                      <span className="text-muted-foreground">
                        {binding.source === "const"
                          ? String(
                              binding.const_value ?? t("workflowView.constant"),
                            )
                          : binding.ref}
                      </span>
                    </div>
                  ))
                )}
              </dd>
            </div>
          </dl>
        </div>
      )}
    </aside>
  );
}

function InfoCard({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof GitBranch;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-2xl border border-border/70 bg-card/90 p-4 shadow-sm">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="rounded-lg bg-primary/10 p-2 text-primary">
          <Icon className="h-4 w-4" />
        </span>
        {label}
      </div>
      <div className="mt-3 truncate font-semibold">{value}</div>
    </div>
  );
}
