import { useJobsList } from "@/domains/jobs/hooks/use-jobs-list";
import { useCancelJob } from "@/domains/jobs/hooks/use-job-mutations";
import { JobStatusBadge } from "@/domains/jobs/components/status-badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { CANCELABLE_JOB_STATUS } from "@/domains/jobs/types";
import { Ban } from "lucide-react";

export function JobList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading } = useJobsList();
  const cancelJob = useCancelJob();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  const items = data?.items ?? [];
  if (items.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        No jobs yet.
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-card">
          <tr className="border-b text-left text-xs uppercase text-muted-foreground">
            <th className="px-3 py-2">Workflow</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2">Submitted</th>
            <th className="px-3 py-2" aria-label="actions" />
          </tr>
        </thead>
        <tbody>
          {items.map((j) => (
            <tr key={j.id} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(j.id)}
                >
                  {j.workflow_id}
                  <span className="text-muted-foreground">
                    {" "}
                    (v{j.workflow_version})
                  </span>
                </button>
              </td>
              <td className="px-3 py-2">
                <JobStatusBadge status={j.status} />
              </td>
              <td className="px-3 py-2 text-muted-foreground">
                {j.submitted_at}
              </td>
              <td className="px-3 py-2">
                {CANCELABLE_JOB_STATUS.has(j.status) ? (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="cancel"
                    disabled={cancelJob.isPending}
                    onClick={() => cancelJob.mutate({ jobId: j.id })}
                  >
                    <Ban className="h-4 w-4" />
                  </Button>
                ) : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </ScrollArea>
  );
}
