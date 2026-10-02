import { Badge } from "@/components/ui/badge";
import type { JobStatus, StepRunStatus } from "@/domains/jobs/types";
import { cn } from "@/foundation/lib/utils";

// Soft-tint status chips (consistent with materials/products visibility/
// status badges). Picking distinct tints per semantic so that, e.g., running
// reads as info-blue and need_confirm reads as warning-amber without
// overwhelming the card chrome.

const JOB_STATUS_STYLE: Record<JobStatus, string> = {
  pending: "bg-muted text-muted-foreground",
  running: "bg-info/15 text-info",
  need_confirm: "bg-warning/15 text-warning",
  succeeded: "bg-success/15 text-success",
  failed: "bg-destructive/10 text-destructive",
  cancelled: "bg-muted text-muted-foreground",
};

const STEP_RUN_STATUS_STYLE: Record<StepRunStatus, string> = {
  pending: "bg-muted/40 text-muted-foreground",
  blocked: "bg-muted text-muted-foreground",
  running: "bg-info/15 text-info",
  claimed: "bg-info/15 text-info",
  need_confirm: "bg-warning/15 text-warning",
  succeeded: "bg-success/15 text-success",
  failed: "bg-destructive/10 text-destructive",
  skipped: "bg-muted/60 text-muted-foreground",
};

const JOB_STATUS_LABEL: Record<JobStatus, string> = {
  pending: "等待执行",
  running: "执行中",
  need_confirm: "待确认",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

const STEP_RUN_STATUS_LABEL: Record<StepRunStatus, string> = {
  pending: "等待执行",
  blocked: "等待前序",
  running: "执行中",
  claimed: "执行中",
  need_confirm: "待确认",
  succeeded: "已完成",
  failed: "失败",
  skipped: "已跳过",
};

export function JobStatusBadge({ status }: { status: JobStatus }) {
  return (
    <Badge className={cn("font-medium capitalize", JOB_STATUS_STYLE[status])}>
      {JOB_STATUS_LABEL[status]}
    </Badge>
  );
}

export function StepRunStatusBadge({ status }: { status: StepRunStatus }) {
  return (
    <Badge
      className={cn("font-medium capitalize", STEP_RUN_STATUS_STYLE[status])}
    >
      {STEP_RUN_STATUS_LABEL[status]}
    </Badge>
  );
}
