import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { StepRunStatusBadge } from "@/domains/jobs/components/status-badge";
import {
  useConfirmStepRun,
  useRegenerateStepRun,
} from "@/domains/jobs/hooks/use-job-mutations";
import type { StepRun } from "@/domains/jobs/types";
import { cn } from "@/foundation/lib/utils";

function isVideoURL(url: string) {
  return /\.(mp4|webm|mov|m4v)(?:$|[?#])/i.test(url);
}

function CandidatePreview({ url, index }: { url: string; index: number }) {
  const [failed, setFailed] = useState(false);
  if (failed) {
    return (
      <p
        className="rounded-lg bg-muted p-4 text-sm text-muted-foreground"
        role="status"
      >
        媒体暂时无法预览，请检查资源地址。
      </p>
    );
  }
  if (isVideoURL(url)) {
    return (
      // biome-ignore lint/a11y/useMediaCaption: generated videos do not provide caption tracks.
      <video
        src={url}
        controls
        preload="metadata"
        onError={() => setFailed(true)}
        className="aspect-video max-h-80 w-full rounded-lg bg-muted object-contain"
        aria-label={`候选视频 ${index + 1}`}
      />
    );
  }
  return (
    <img
      src={url}
      alt={`候选图片 ${index + 1}`}
      onError={() => setFailed(true)}
      className="aspect-video max-h-80 w-full rounded-lg bg-muted object-contain"
      loading="lazy"
    />
  );
}

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
  const canRegenerate = stepRun.status === "need_confirm";

  return (
    <article className="flex flex-col gap-4 rounded-2xl border border-border/70 bg-card/80 p-4 sm:p-5">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <h3 className="font-semibold">
            {stepRun.step_key.replaceAll("_", " ")}
          </h3>
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
              disabled={confirm.isPending || picked === null}
              onClick={() =>
                confirm.mutate({
                  jobId,
                  stepRunId: stepRun.id,
                  selectedIndex: picked ?? 0,
                })
              }
            >
              {confirm.isPending ? <Spinner className="h-4 w-4" /> : null}
              确认使用
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
              重新生成
            </Button>
          ) : null}
        </div>
      </div>

      <dl className="grid gap-x-3 gap-y-1 text-xs sm:grid-cols-[8rem_1fr]">
        <dt className="text-muted-foreground">模板版本</dt>
        <dd>v{stepRun.template_version}</dd>
        {stepRun.output_asset_id ? (
          <>
            <dt className="text-muted-foreground">产物编号</dt>
            <dd className="break-all">{stepRun.output_asset_id}</dd>
          </>
        ) : null}
        {stepRun.failure_reason ? (
          <>
            <dt className="text-muted-foreground">失败原因</dt>
            <dd className="break-words text-destructive">
              {stepRun.failure_reason}
            </dd>
          </>
        ) : null}
      </dl>

      {stepRun.rendered_prompt ? (
        <details className="text-sm">
          <summary className="cursor-pointer font-medium">
            查看生成提示词
          </summary>
          <p className="mt-2 max-h-60 overflow-auto whitespace-pre-wrap rounded-lg bg-muted/60 p-3 leading-6">
            {stepRun.rendered_prompt}
          </p>
        </details>
      ) : null}

      {stepRun.candidates.length > 0 ? (
        <div className="flex flex-col gap-1">
          <p className="text-sm font-medium">
            {needsConfirm ? "请选择要使用的结果" : "生成结果"}
          </p>
          <ul className="grid gap-3 sm:grid-cols-2">
            {stepRun.candidates.map((c) => {
              const isPicked = picked === c.index;
              const isSelected = stepRun.selected_index === c.index;
              return (
                <li
                  key={c.index}
                  className={cn(
                    "flex min-w-0 items-start gap-2 rounded-xl border p-3 text-xs",
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
                      {isSelected && !needsConfirm ? (
                        <span className="text-success">selected</span>
                      ) : null}
                    </div>
                    {c.url ? (
                      <>
                        <CandidatePreview url={c.url} index={c.index} />
                        <a
                          href={c.url}
                          target="_blank"
                          rel="noreferrer"
                          className="text-info hover:underline"
                        >
                          打开媒体
                        </a>
                      </>
                    ) : null}
                    {c.content ? (
                      <p className="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-muted p-2 text-xs">
                        {c.content}
                      </p>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}
    </article>
  );
}
