import {
  useCancelJob,
  useRetryJob,
} from "@/domains/jobs/hooks/use-job-mutations";
import { useJob } from "@/domains/jobs/hooks/use-job";
import { JobStatusBadge } from "@/domains/jobs/components/status-badge";
import { StepRunCard } from "@/domains/jobs/components/step-run-card";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  CANCELABLE_JOB_STATUS,
  RETRYABLE_JOB_STATUS,
} from "@/domains/jobs/types";

export function JobShow({
  id,
  onBack,
}: {
  id: string;
  onBack: () => void;
}) {
  const { data, isLoading } = useJob(id);
  const cancelJob = useCancelJob();
  const retryJob = useRetryJob();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!data) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  const { job, step_runs } = data;
  const sortedStepRuns = [...step_runs].sort(
    (a, b) => a.step_order - b.step_order,
  );

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
        <div className="flex gap-2">
          {CANCELABLE_JOB_STATUS.has(job.status) ? (
            <Button
              variant="outline"
              size="sm"
              disabled={cancelJob.isPending}
              onClick={() => cancelJob.mutate({ jobId: id })}
            >
              {cancelJob.isPending ? <Spinner className="h-4 w-4" /> : null}
              Cancel
            </Button>
          ) : null}
          {RETRYABLE_JOB_STATUS.has(job.status) ? (
            <Button
              size="sm"
              disabled={retryJob.isPending}
              onClick={() => retryJob.mutate({ jobId: id })}
            >
              {retryJob.isPending ? <Spinner className="h-4 w-4" /> : null}
              Retry
            </Button>
          ) : null}
        </div>
      </div>

      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Job ID</dt>
        <dd className="col-span-2 break-all font-medium">{job.id}</dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{job.scope}</dd>
        <dt className="text-muted-foreground">Workflow</dt>
        <dd className="col-span-2">
          {job.workflow_id}
          <span className="text-muted-foreground">
            {" "}
            (v{job.workflow_version})
          </span>
        </dd>
        <dt className="text-muted-foreground">Status</dt>
        <dd className="col-span-2">
          <JobStatusBadge status={job.status} />
        </dd>
        <dt className="text-muted-foreground">Retry Count</dt>
        <dd className="col-span-2">{job.retry_count}</dd>
        <dt className="text-muted-foreground">Submitted By</dt>
        <dd className="col-span-2">{job.submitted_by}</dd>
        <dt className="text-muted-foreground">Submitted</dt>
        <dd className="col-span-2">{job.submitted_at}</dd>
        {job.started_at ? (
          <>
            <dt className="text-muted-foreground">Started</dt>
            <dd className="col-span-2">{job.started_at}</dd>
          </>
        ) : null}
        {job.finished_at ? (
          <>
            <dt className="text-muted-foreground">Finished</dt>
            <dd className="col-span-2">{job.finished_at}</dd>
          </>
        ) : null}
        {job.failure_reason ? (
          <>
            <dt className="text-muted-foreground">Failure</dt>
            <dd className="col-span-2 break-words text-destructive">
              {job.failure_reason}
            </dd>
          </>
        ) : null}
        <dt className="text-muted-foreground">Input</dt>
        <dd className="col-span-2">
          <pre className="max-h-80 overflow-auto rounded bg-muted p-2 text-xs">
            {JSON.stringify(job.input, null, 2)}
          </pre>
        </dd>
        {job.retry_history.length > 0 ? (
          <>
            <dt className="text-muted-foreground">Retry History</dt>
            <dd className="col-span-2">
              <pre className="max-h-60 overflow-auto rounded bg-muted p-2 text-xs">
                {JSON.stringify(job.retry_history, null, 2)}
              </pre>
            </dd>
          </>
        ) : null}
      </dl>

      <div className="flex flex-col gap-2">
        <h3 className="text-xs uppercase text-muted-foreground">
          Step Runs ({sortedStepRuns.length})
        </h3>
        {sortedStepRuns.length === 0 ? (
          <div className="text-sm text-muted-foreground">
            No step runs yet.
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {sortedStepRuns.map((sr) => (
              <StepRunCard key={sr.id} jobId={id} stepRun={sr} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
