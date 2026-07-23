import { Badge } from "@/components/ui/badge";
import { cn } from "@/foundation/lib/utils";
import type { JobStatus, StepRunStatus } from "@/domains/jobs/types";

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

export function JobStatusBadge({ status }: { status: JobStatus }) {
  return (
    <Badge className={cn("font-medium capitalize", JOB_STATUS_STYLE[status])}>
      {status}
    </Badge>
  );
}

export function StepRunStatusBadge({ status }: { status: StepRunStatus }) {
  return (
    <Badge
      className={cn("font-medium capitalize", STEP_RUN_STATUS_STYLE[status])}
    >
      {status}
    </Badge>
  );
}
