import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/foundation/lib/utils";
import { StepRunStatusBadge } from "@/domains/jobs/components/status-badge";
import {
  useConfirmStepRun,
  useRegenerateStepRun,
} from "@/domains/jobs/hooks/use-job-mutations";
import type { StepRun } from "@/domains/jobs/types";

/**
 * One StepRun rendered as a card. When status === need_confirm, shows the
 * candidates list and lets the user pick one (selected_index) then Confirm;
 * Regenerate is always available for non-terminal steps. Local state holds the
 * in-progress selection so the radio does not jump back to selected_index
 * while the mutation is in flight.
 */
export function StepRunCard({
  jobId,
  stepRun,
}: {
  jobId: string;
  stepRun: StepRun;
}) {
  const confirm = useConfirmStepRun();
  const regenerate = useRegenerateStepRun();
  const [picked, setPicked] = useState<number | null>(
    stepRun.selected_index ?? null,
  );

  const needsConfirm = stepRun.status === "need_confirm";
  const canRegenerate =
    stepRun.status === "succeeded" ||
    stepRun.status === "failed" ||
    stepRun.status === "skipped" ||
    stepRun.status === "need_confirm";

  return (
    <div className="flex flex-col gap-2 rounded-md border border-foreground/[0.06] p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="font-medium">{stepRun.step_key}</span>
          <span className="text-xs text-muted-foreground">
            #{stepRun.step_order}
          </span>
          <StepRunStatusBadge status={stepRun.status} />
          {stepRun.retry_count > 0 ? (
            <span className="text-xs text-muted-foreground">
              retry {stepRun.retry_count}
            </span>
          ) : null}
        </div>
        <div className="flex gap-2">
          {needsConfirm ? (
            <Button
              size="sm"
              disabled={
                confirm.isPending ||
                picked === null ||
                picked === stepRun.selected_index
              }
              onClick={() =>
                confirm.mutate({
                  jobId,
                  stepRunId: stepRun.id,
                  selectedIndex: picked ?? 0,
                })
              }
            >
              {confirm.isPending ? <Spinner className="h-4 w-4" /> : null}
              Confirm
            </Button>
          ) : null}
          {canRegenerate ? (
            <Button
              size="sm"
              variant="outline"
              disabled={regenerate.isPending}
              onClick={() =>
                regenerate.mutate({ jobId, stepRunId: stepRun.id })
              }
            >
              {regenerate.isPending ? <Spinner className="h-4 w-4" /> : null}
              Regenerate
            </Button>
          ) : null}
        </div>
      </div>

      <dl className="grid grid-cols-3 gap-1 text-xs">
        <dt className="text-muted-foreground">Template</dt>
        <dd className="col-span-2">
          {stepRun.template_id}
          <span className="text-muted-foreground">
            {" "}
            (v{stepRun.template_version})
          </span>
        </dd>
        <dt className="text-muted-foreground">Step Run ID</dt>
        <dd className="col-span-2 break-all text-muted-foreground">
          {stepRun.id}
        </dd>
        {stepRun.output_asset_id ? (
          <>
            <dt className="text-muted-foreground">Output Asset</dt>
            <dd className="col-span-2 break-all">
              {stepRun.output_asset_id}
            </dd>
          </>
        ) : null}
        {stepRun.failure_reason ? (
          <>
            <dt className="text-muted-foreground">Failure</dt>
            <dd className="col-span-2 break-words text-destructive">
              {stepRun.failure_reason}
            </dd>
          </>
        ) : null}
      </dl>

      {stepRun.rendered_prompt ? (
        <div className="flex flex-col gap-1">
          <Label className="text-xs uppercase text-muted-foreground">
            Rendered Prompt
          </Label>
          <pre className="max-h-60 overflow-auto rounded bg-muted p-2 text-xs whitespace-pre-wrap">
            {stepRun.rendered_prompt}
          </pre>
        </div>
      ) : null}

      {stepRun.candidates.length > 0 ? (
        <div className="flex flex-col gap-1">
          <Label className="text-xs uppercase text-muted-foreground">
            Candidates
          </Label>
          <ul className="flex flex-col gap-1">
            {stepRun.candidates.map((c) => {
              const isPicked = picked === c.index;
              const isSelected = stepRun.selected_index === c.index;
              return (
                <li
                  key={c.index}
                  className={cn(
                    "flex items-start gap-2 rounded border p-2 text-xs",
                  isPicked
                    ? "border-info bg-info/10"
                    : "border-border bg-card",
                )}
                >
                  <input
                    type="radio"
                    name={`step-run-${stepRun.id}-candidate`}
                    checked={isPicked}
                    disabled={!needsConfirm}
                    onChange={() => setPicked(c.index)}
                    className="mt-0.5"
                  />
                  <div className="flex flex-1 flex-col gap-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">#{c.index}</span>
                      {c.is_primary ? (
                        <span className="text-muted-foreground">primary</span>
                      ) : null}
                      {isSelected ? (
                        <span className="text-success">selected</span>
                      ) : null}
                    </div>
                    {c.url ? (
                      <a
                        href={c.url}
                        target="_blank"
                        rel="noreferrer"
                        className="break-all text-info hover:underline"
                      >
                        {c.url}
                      </a>
                    ) : null}
                    {c.content ? (
                      <pre className="max-h-40 overflow-auto rounded bg-muted p-1 text-xs whitespace-pre-wrap">
                        {c.content}
                      </pre>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}

      {Object.keys(stepRun.resolved_inputs).length > 0 ? (
        <details className="text-xs">
          <summary className="cursor-pointer text-muted-foreground">
            Resolved Inputs
          </summary>
          <pre className="mt-1 max-h-40 overflow-auto rounded bg-muted p-2 text-xs">
            {JSON.stringify(stepRun.resolved_inputs, null, 2)}
          </pre>
        </details>
      ) : null}
    </div>
  );
}
