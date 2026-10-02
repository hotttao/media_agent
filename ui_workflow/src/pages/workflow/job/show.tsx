import {
  ArrowLeft,
  CalendarClock,
  Package2,
  UserRound,
  WandSparkles,
} from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { JobStatusBadge } from "@/domains/jobs/components/status-badge";
import { StepRunCard } from "@/domains/jobs/components/step-run-card";
import { useJob } from "@/domains/jobs/hooks/use-job";
import {
  useCancelJob,
  useRetryJob,
} from "@/domains/jobs/hooks/use-job-mutations";
import type { StepRun } from "@/domains/jobs/types";
import {
  CANCELABLE_JOB_STATUS,
  type JobInput,
  RETRYABLE_JOB_STATUS,
} from "@/domains/jobs/types";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { WorkflowGraph } from "@/domains/workflows/components/workflow-graph";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import { formatDetailDate } from "@/foundation/components/detail-layout";

const stepStatusLabel: Record<StepRun["status"], string> = {
  blocked: "等待前序",
  pending: "等待执行",
  claimed: "执行中",
  running: "执行中",
  need_confirm: "待确认",
  succeeded: "已完成",
  failed: "失败",
  skipped: "已跳过",
};

function InputSummary({ input }: { input: JobInput }) {
  const resources = [...(input.material ?? []), ...(input.asset ?? [])];
  if (!input.product && !input.virtual_ip && resources.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">这次执行没有外部输入。</p>
    );
  }
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {input.product ? (
        <div className="flex min-w-0 items-center gap-3 rounded-xl bg-muted/55 p-3">
          {input.product.image_url ? (
            <img
              src={input.product.image_url}
              alt={input.product.name || "商品"}
              className="h-16 w-16 shrink-0 rounded-lg bg-background object-contain"
            />
          ) : (
            <Package2 className="h-6 w-6 shrink-0 text-muted-foreground" />
          )}
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">商品</p>
            <p className="truncate text-sm font-medium">
              {input.product.name || input.product.id || "未命名商品"}
            </p>
          </div>
        </div>
      ) : null}
      {input.virtual_ip ? (
        <div className="flex min-w-0 items-center gap-3 rounded-xl bg-muted/55 p-3">
          {input.virtual_ip.avatar_url ? (
            <img
              src={input.virtual_ip.avatar_url}
              alt={input.virtual_ip.name || "虚拟形象"}
              className="h-16 w-16 shrink-0 rounded-lg bg-background object-cover"
            />
          ) : (
            <UserRound className="h-6 w-6 shrink-0 text-muted-foreground" />
          )}
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">虚拟形象</p>
            <p className="truncate text-sm font-medium">
              {input.virtual_ip.name || input.virtual_ip.id || "未命名形象"}
            </p>
          </div>
        </div>
      ) : null}
      {resources.map((item, index) => (
        <div
          key={`${item.asset_kind}-${item.name}-${index}`}
          className="flex min-w-0 items-center gap-3 rounded-xl bg-muted/55 p-3"
        >
          <WandSparkles className="h-5 w-5 shrink-0 text-muted-foreground" />
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">
              {item.asset_kind || "素材"}
            </p>
            <p className="truncate text-sm font-medium">
              {item.name || item.content || "未命名素材"}
            </p>
          </div>
        </div>
      ))}
    </div>
  );
}

export function JobShow({ id, onBack }: { id: string; onBack: () => void }) {
  const { data, isLoading, isError, refetch } = useJob(id);
  const { data: workflow } = useWorkflow(data?.job.workflow_id);
  const { data: templates } = useTemplatesList({
    scope: workflow?.scope,
    page_size: 200,
  });
  const [selectedStepKey, setSelectedStepKey] = useState<string>();
  const cancelJob = useCancelJob();
  const retryJob = useRetryJob();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (isError)
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-sm">
        <p>执行记录加载失败。</p>
        <Button variant="outline" onClick={() => void refetch()}>
          重试
        </Button>
      </div>
    );
  if (!data)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        没有找到这次执行。
      </div>
    );

  const { job, step_runs: stepRuns } = data;
  const sortedStepRuns = [...stepRuns].sort(
    (left, right) => left.step_order - right.step_order,
  );
  const completeCount = sortedStepRuns.filter(
    (stepRun) => stepRun.status === "succeeded",
  ).length;
  const progress = sortedStepRuns.length
    ? Math.round((completeCount / sortedStepRuns.length) * 100)
    : 0;
  const highlightedStep =
    sortedStepRuns.find((stepRun) => stepRun.status === "need_confirm") ??
    sortedStepRuns.find(
      (stepRun) => stepRun.status === "running" || stepRun.status === "claimed",
    ) ??
    sortedStepRuns.find((stepRun) => stepRun.status === "pending") ??
    sortedStepRuns.find((stepRun) => stepRun.status === "failed") ??
    sortedStepRuns.at(-1);
  const selectedStep =
    sortedStepRuns.find((stepRun) => stepRun.step_key === selectedStepKey) ??
    highlightedStep;
  const canShowGraph = workflow && workflow.version === job.workflow_version;
  const executionStatus = Object.fromEntries(
    sortedStepRuns.map((stepRun) => [stepRun.step_key, stepRun.status]),
  );

  return (
    <main className="h-full overflow-y-auto bg-gradient-to-b from-slate-50/80 to-background dark:from-slate-950/80">
      <div className="mx-auto flex max-w-5xl flex-col gap-6 p-4 sm:p-6 lg:p-8">
        <header className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <Button
              variant="outline"
              size="icon"
              onClick={onBack}
              aria-label="返回执行列表"
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="min-w-0">
              <div className="mb-2 flex items-center gap-2">
                <JobStatusBadge status={job.status} />
                <span className="text-xs text-muted-foreground">
                  版本 {job.workflow_version}
                </span>
              </div>
              <h1 className="text-balance text-2xl font-semibold tracking-tight sm:text-3xl">
                {workflow?.name ?? "工作流执行"}
              </h1>
              <p
                className="mt-1 max-w-xl truncate text-xs text-muted-foreground"
                title={job.id}
              >
                {job.id}
              </p>
            </div>
          </div>
          <div className="flex gap-2">
            {CANCELABLE_JOB_STATUS.has(job.status) ? (
              <Button
                variant="outline"
                disabled={cancelJob.isPending}
                onClick={() => cancelJob.mutate({ jobId: id })}
              >
                取消执行
              </Button>
            ) : null}
            {RETRYABLE_JOB_STATUS.has(job.status) ? (
              <Button
                disabled={retryJob.isPending}
                onClick={() => retryJob.mutate({ jobId: id })}
              >
                重新执行
              </Button>
            ) : null}
          </div>
        </header>
        <section
          aria-label="执行进度"
          className="rounded-2xl border border-border/70 bg-card/80 p-5 sm:p-6"
        >
          <div className="flex items-end justify-between gap-4">
            <div>
              <p className="text-xs font-medium text-muted-foreground">
                执行进度
              </p>
              <p className="mt-1 text-xl font-semibold tabular-nums">
                {completeCount} / {sortedStepRuns.length} 步完成
              </p>
            </div>
            <span className="text-sm font-medium tabular-nums text-muted-foreground">
              {progress}%
            </span>
          </div>
          <div
            className="mt-4 h-1.5 overflow-hidden rounded-full bg-muted"
            role="progressbar"
            aria-valuenow={completeCount}
            aria-valuemin={0}
            aria-valuemax={sortedStepRuns.length}
            aria-label="已完成步骤"
          >
            <div
              className="h-full rounded-full bg-primary transition-[width]"
              style={{ width: `${progress}%` }}
            />
          </div>
          <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <CalendarClock className="h-3.5 w-3.5" />
              提交于 {formatDetailDate(job.submitted_at)}
            </span>
            {job.finished_at ? (
              <span>结束于 {formatDetailDate(job.finished_at)}</span>
            ) : null}
            {job.retry_count > 0 ? (
              <span>已重试 {job.retry_count} 次</span>
            ) : null}
          </div>
          {job.failure_reason ? (
            <p
              className="mt-4 rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
              role="alert"
            >
              {job.failure_reason}
            </p>
          ) : null}
          {job.status === "need_confirm" ? (
            <p className="mt-4 rounded-lg bg-warning/10 p-3 text-sm text-warning">
              有节点等待你确认结果。选择标记为“待确认”的节点继续。
            </p>
          ) : null}
        </section>
        <section aria-labelledby="job-input-title" className="space-y-3">
          <div className="flex items-baseline justify-between gap-3">
            <h2 id="job-input-title" className="text-lg font-semibold">
              本次输入
            </h2>
            <span className="text-xs text-muted-foreground">{job.scope}</span>
          </div>
          <InputSummary input={job.input} />
        </section>
        <section aria-labelledby="job-steps-title" className="space-y-3">
          <h2 id="job-steps-title" className="text-lg font-semibold">
            执行位置
          </h2>
          {sortedStepRuns.length === 0 ? (
            <p className="rounded-xl bg-muted/50 p-6 text-sm text-muted-foreground">
              步骤尚未开始。
            </p>
          ) : (
            <div className="overflow-hidden rounded-2xl border border-border/70 bg-card">
              {canShowGraph ? (
                <WorkflowGraph
                  model={{
                    steps: workflow.steps,
                    outputStepKey: workflow.output_step_key,
                    templateNameByID: Object.fromEntries(
                      (templates?.items ?? []).map((template) => [
                        template.id,
                        template.name,
                      ]),
                    ),
                  }}
                  className="h-[390px] border-b"
                  selectedNodeId={selectedStep?.step_key}
                  onNodeSelect={setSelectedStepKey}
                  executionStatus={executionStatus}
                />
              ) : (
                <p className="border-b p-4 text-sm text-muted-foreground">
                  {workflow
                    ? "这次执行使用了旧版本工作流；当前定义与执行版本不同，下面按实际执行记录显示。"
                    : "工作流定义暂不可用，下面按实际执行记录显示。"}
                </p>
              )}
              <nav
                className="flex gap-2 overflow-x-auto border-b p-3"
                aria-label="选择执行节点"
              >
                {sortedStepRuns.map((stepRun) => (
                  <button
                    key={stepRun.id}
                    type="button"
                    onClick={() => setSelectedStepKey(stepRun.step_key)}
                    aria-current={
                      selectedStep?.id === stepRun.id ? "step" : undefined
                    }
                    className={`shrink-0 rounded-lg border px-3 py-2 text-left text-xs transition-colors ${selectedStep?.id === stepRun.id ? "border-primary bg-primary/10 text-primary" : "border-border hover:bg-muted"}`}
                  >
                    <span className="block font-medium">
                      {stepRun.step_key}
                    </span>
                    <span className="mt-1 block opacity-70">
                      {stepStatusLabel[stepRun.status]}
                    </span>
                  </button>
                ))}
              </nav>
              {selectedStep ? (
                <div className="p-4">
                  <StepRunCard
                    key={selectedStep.id}
                    jobId={id}
                    stepRun={selectedStep}
                  />
                </div>
              ) : null}
            </div>
          )}
        </section>
      </div>
    </main>
  );
}
