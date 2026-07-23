import { useState } from "react";
import { useSubmitJob } from "@/domains/jobs/hooks/use-job-mutations";
import { useWorkflowsList } from "@/domains/jobs/hooks/use-workflows-list";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Spinner } from "@/components/ui/spinner";
import type { JobInput } from "@/domains/jobs/types";

const EMPTY_INPUT_JSON = `{
  "product": { "id": "", "name": "" },
  "virtual_ip": { "id": "", "name": "" },
  "asset": [],
  "material": []
}`;

export function JobCreate({
  onDone,
  onSubmitted,
}: {
  onDone: () => void;
  onSubmitted: (jobId: string) => void;
}) {
  const submitJob = useSubmitJob();
  const { data: workflows, isLoading: workflowsLoading } = useWorkflowsList();
  const [workflowId, setWorkflowId] = useState<string>("");
  const [inputJson, setInputJson] = useState<string>(EMPTY_INPUT_JSON);
  const [parseError, setParseError] = useState<string | null>(null);

  // scope is intentionally not a form field - the server copies it from the
  // workflow (docs/media_workflow/domain/0003-execution: scope "沿用
  // workflow.scope"). submitted_by is inferred from auth.

  function handleSubmit() {
    let parsed: JobInput;
    try {
      parsed = JSON.parse(inputJson) as JobInput;
    } catch (e) {
      setParseError(e instanceof Error ? e.message : "Invalid JSON");
      return;
    }
    setParseError(null);
    submitJob.mutate(
      { workflow_id: workflowId, input: parsed },
      {
        onSuccess: (job) => {
          onSubmitted(job.id);
        },
      },
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">New Job</h2>
      <div className="grid gap-2">
        <Label>Workflow</Label>
        {workflowsLoading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="h-4 w-4" /> Loading workflows…
          </div>
        ) : (
          <Select value={workflowId} onValueChange={setWorkflowId}>
            <SelectTrigger>
              <SelectValue placeholder="Select workflow" />
            </SelectTrigger>
            <SelectContent>
              {(workflows ?? []).map((w) => (
                <SelectItem key={w.id} value={w.id}>
                  {w.name}{" "}
                  <span className="text-muted-foreground">(v{w.version})</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>
      <div className="grid gap-2">
        <Label>Input (JSON)</Label>
        <Textarea
          className="min-h-[200px] font-mono text-xs"
          value={inputJson}
          onChange={(e) => setInputJson(e.target.value)}
        />
        {parseError ? (
          <p className="text-xs text-destructive">{parseError}</p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          Paste a JobInput JSON bundle (product / virtual_ip / asset[] /
          material[]). Matches the JobInput message in idl/media_workflow/job.proto.
        </p>
      </div>
      <div className="flex gap-2">
        <Button
          onClick={handleSubmit}
          disabled={submitJob.isPending || !workflowId}
        >
          {submitJob.isPending ? <Spinner className="h-4 w-4" /> : null}
          Submit
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
