import { useQueries } from "@tanstack/react-query";
import { ArrowUpRight, Ban } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { JobStatusBadge } from "@/domains/jobs/components/status-badge";
import { useCancelJob } from "@/domains/jobs/hooks/use-job-mutations";
import { useJobsList } from "@/domains/jobs/hooks/use-jobs-list";
import { getJob } from "@/domains/jobs/lib/api";
import {
  CANCELABLE_JOB_STATUS,
  type GetJobResponse,
  type Job,
} from "@/domains/jobs/types";
import { useWorkflowsList } from "@/domains/workflows/hooks/use-workflows-list";
import { formatDetailDate } from "@/foundation/components/detail-layout";

function currentPosition(detail?: GetJobResponse) {
  if (!detail) return "正在获取节点状态…";
  const steps = [...detail.step_runs].sort(
    (a, b) => a.step_order - b.step_order,
  );
  const approval = steps.filter((step) => step.status === "need_confirm");
  if (approval.length)
    return `待确认：${approval.map((step) => step.step_key).join("、")}`;
  const running = steps.filter(
    (step) => step.status === "running" || step.status === "claimed",
  );
  if (running.length)
    return `正在执行：${running.map((step) => step.step_key).join("、")}`;
  const pending = steps.find((step) => step.status === "pending");
  if (pending) return `等待执行：${pending.step_key}`;
  const failed = steps.find((step) => step.status === "failed");
  if (failed) return `失败于：${failed.step_key}`;
  if (detail.job.status === "succeeded") return `${steps.length} 个节点已完成`;
  return steps.length ? "等待前序节点" : "尚未开始";
}

function jobPriority(job: Job) {
  if (job.status === "need_confirm") return 0;
  if (job.status === "running") return 1;
  if (job.status === "pending") return 2;
  return 3;
}

export function JobList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading, isError, refetch } = useJobsList({ page_size: 100 });
  const { data: workflows } = useWorkflowsList({ page_size: 200 });
  const cancelJob = useCancelJob();
  const items = data?.items ?? [];
  const activeJobs = items.filter((job) =>
    CANCELABLE_JOB_STATUS.has(job.status),
  );
  const activeDetails = useQueries({
    queries: activeJobs.map((job) => ({
      queryKey: ["job", job.id],
      queryFn: () => getJob(job.id),
      refetchInterval: 2000,
    })),
  });
  const details = new Map(
    activeJobs.map((job, index) => [job.id, activeDetails[index]?.data]),
  );
  const workflowNames = new Map(
    (workflows?.items ?? []).map((workflow) => [workflow.id, workflow.name]),
  );
  const sortedItems = [...items].sort(
    (a, b) => jobPriority(a) - jobPriority(b),
  );

  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (isError)
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-3">
        <p>执行记录加载失败。</p>
        <Button variant="outline" onClick={() => void refetch()}>
          重试
        </Button>
      </div>
    );
  if (items.length === 0)
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        还没有执行记录。运行一个工作流后，可以在这里追踪进度。
      </div>
    );

  return (
    <ScrollArea className="flex-1">
      <div className="mx-auto max-w-5xl space-y-4 p-4 sm:p-6">
        <div>
          <h1 className="text-2xl font-semibold">执行中的工作流</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            先看哪些流程需要处理，再打开节点查看结果。
          </p>
        </div>
        <div className="space-y-3">
          {sortedItems.map((job) => {
            const detail = details.get(job.id);
            const done = detail?.step_runs.filter(
              (step) => step.status === "succeeded",
            ).length;
            const total = detail?.step_runs.length;
            return (
              <article
                key={job.id}
                className="rounded-2xl border border-border/70 bg-card p-4 shadow-sm sm:p-5"
              >
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="mb-2 flex items-center gap-2">
                      <JobStatusBadge status={job.status} />
                      <span className="text-xs text-muted-foreground">
                        v{job.workflow_version}
                      </span>
                    </div>
                    <button
                      type="button"
                      onClick={() => onSelect(job.id)}
                      className="group flex items-center gap-2 text-left text-lg font-semibold hover:text-primary"
                    >
                      <span className="truncate">
                        {workflowNames.get(job.workflow_id) ?? job.workflow_id}
                      </span>
                      <ArrowUpRight className="h-4 w-4 shrink-0 opacity-50 group-hover:opacity-100" />
                    </button>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {CANCELABLE_JOB_STATUS.has(job.status)
                        ? currentPosition(detail)
                        : job.status === "succeeded"
                          ? "全部节点已完成"
                          : job.status === "failed"
                            ? "执行失败，打开查看失败节点"
                            : "已取消"}
                      {done !== undefined ? ` · ${done}/${total} 步完成` : ""}
                    </p>
                  </div>
                  {CANCELABLE_JOB_STATUS.has(job.status) ? (
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="取消执行"
                      disabled={cancelJob.isPending}
                      onClick={() => cancelJob.mutate({ jobId: job.id })}
                    >
                      <Ban className="h-4 w-4" />
                    </Button>
                  ) : null}
                </div>
                <div className="mt-3 text-xs text-muted-foreground">
                  {formatDetailDate(job.submitted_at)}
                </div>
              </article>
            );
          })}
        </div>
      </div>
    </ScrollArea>
  );
}
