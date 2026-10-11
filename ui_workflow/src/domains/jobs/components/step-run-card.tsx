import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { StepRunStatusBadge } from "@/domains/jobs/components/status-badge";
import {
  useConfirmStepRun,
  useRegenerateStepRun,
} from "@/domains/jobs/hooks/use-job-mutations";
import {
  candidateFingerprint,
  feedbackBytes,
  validCandidatePosition,
  type ReviewIntent,
} from "@/domains/jobs/lib/review-intent";
import type { StepRun } from "@/domains/jobs/types";
import { hostShim } from "@/foundation/lib/host-shim";
import { cn } from "@/foundation/lib/utils";
import { requestKey } from "@/foundation/lib/request-key";

export function CandidatePreview({
  url,
  label,
}: {
  url: string;
  label: string;
}) {
  const { t } = useTranslation();
  const [failedURL, setFailedURL] = useState<string>();
  if (failedURL === url)
    return (
      <p
        role="status"
        className="rounded-xl bg-muted p-6 text-xs text-muted-foreground"
      >
        {t("execution.mediaError")}
      </p>
    );
  if (/\.(mp4|webm|mov|m4v)(?:$|[?#])/i.test(url))
    return (
      // biome-ignore lint/a11y/useMediaCaption: generated videos have no caption tracks.
      <video
        src={url}
        controls
        preload="metadata"
        aria-label={label}
        onError={() => setFailedURL(url)}
        className="aspect-video max-h-80 w-full rounded-xl bg-muted object-contain"
      />
    );
  return (
    <img
      src={url}
      alt={label}
      loading="lazy"
      onError={() => setFailedURL(url)}
      className="aspect-[4/3] max-h-80 w-full rounded-xl bg-muted object-contain"
    />
  );
}

export function StepRunCard({
  jobId,
  stepRun,
  instanceId,
  reviewAllowed = true,
}: {
  jobId: string;
  stepRun: StepRun;
  instanceId: string;
  reviewAllowed?: boolean;
}) {
  const { t } = useTranslation();
  const confirm = useConfirmStepRun();
  const regenerate = useRegenerateStepRun();
  const fingerprint = candidateFingerprint(stepRun);
  const [selection, setSelection] = hostShim.useInstanceState<{
    fingerprint: string;
    position: number;
  } | null>(instanceId, `workflow_review_selection_${stepRun.id}`, () => null);
  const [intent, setIntent] = hostShim.useInstanceState<ReviewIntent | null>(
    instanceId,
    `workflow_review_intent_${stepRun.id}`,
    () => null,
  );
  const [feedback, setFeedback] = hostShim.useInstanceState(
    instanceId,
    `workflow_review_feedback_${stepRun.id}`,
    () => "",
  );
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const needsConfirm = reviewAllowed && stepRun.status === "need_confirm";
  const picked =
    selection?.fingerprint === fingerprint ? selection.position : null;
  const currentIntent = intent?.fingerprint === fingerprint ? intent : null;
  const pendingReceipt = stepRun.last_review_status === "pending";
  const locked =
    confirm.isPending ||
    regenerate.isPending ||
    pendingReceipt ||
    Boolean(currentIntent);
  useEffect(() => {
    if (!needsConfirm) {
      setSelection(null);
      if (intent?.accepted) {
        setIntent(null);
        setFeedback("");
      }
    } else if (
      intent?.accepted &&
      intent.decision === "regenerate" &&
      intent.receiptID &&
      stepRun.last_review_id === intent.receiptID &&
      stepRun.last_review_status === "applied"
    ) {
      // An applied regeneration may return the same immutable media in a mock.
      // The parent has returned to review, and this exact receipt is applied.
      setIntent(null);
      setSelection(null);
      setFeedback("");
    }
  }, [
    needsConfirm,
    intent?.accepted,
    intent?.decision,
    intent?.receiptID,
    stepRun.last_review_id,
    stepRun.last_review_status,
    setSelection,
    setIntent,
    setFeedback,
  ]);

  function submit(command: ReviewIntent) {
    setIntent(command);
    const onSuccess = (receipt: StepRun) => {
      setIntent({
        ...command,
        accepted: true,
        receiptID: receipt?.last_review_id,
      });
      setFeedbackOpen(false);
    };
    if (command.decision === "accept")
      confirm.mutate(
        {
          jobId,
          stepRunId: stepRun.id,
          selectedIndex: command.position!,
          idempotencyKey: command.key,
        },
        { onSuccess },
      );
    else
      regenerate.mutate(
        {
          jobId,
          stepRunId: stepRun.id,
          feedback: command.feedback,
          idempotencyKey: command.key,
        },
        { onSuccess },
      );
  }
  const error = confirm.error || regenerate.error;
  return (
    <article
      aria-label={t("execution.nodeResults")}
      className="flex min-w-0 flex-col gap-4 p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-semibold">{stepRun.step_key}</h2>
        <StepRunStatusBadge status={stepRun.status} />
      </div>
      <p className="text-xs text-muted-foreground">
        {t("execution.templateVersion", { version: stepRun.template_version })}
      </p>
      {stepRun.failure_reason ? (
        <p
          role="alert"
          className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
        >
          {stepRun.failure_reason}
        </p>
      ) : null}
      {needsConfirm ? (
        <p className="rounded-xl bg-warning/10 p-3 text-sm">
          {t("execution.chooseResult")}
        </p>
      ) : null}
      {!reviewAllowed && stepRun.status === "need_confirm" ? (
        <p
          role="status"
          className="rounded-xl bg-muted p-3 text-xs text-muted-foreground"
        >
          {t("execution.readOnlyResult")}
        </p>
      ) : null}
      {pendingReceipt || currentIntent?.accepted ? (
        <p role="status" className="rounded-xl bg-primary/10 p-3 text-sm">
          {t("execution.decisionPending")}
        </p>
      ) : null}
      {error ? (
        <div role="alert" className="space-y-2 text-sm text-destructive">
          <p>{error.message}</p>
        </div>
      ) : null}
      {needsConfirm &&
      currentIntent &&
      !currentIntent.accepted &&
      !pendingReceipt &&
      !confirm.isPending &&
      !regenerate.isPending ? (
        <div role="status" className="space-y-2 rounded-lg border p-3 text-sm">
          <p className="text-xs">{t("execution.unknownDecision")}</p>
          <Button
            variant="outline"
            disabled={confirm.isPending || regenerate.isPending}
            onClick={() => submit(currentIntent)}
          >
            {t("execution.replayDecision")}
          </Button>
        </div>
      ) : null}
      {stepRun.last_review_status === "failed" ? (
        <p className="text-xs text-destructive">
          {t("execution.receiptFailed")}
        </p>
      ) : null}
      <div className="grid gap-3">
        {stepRun.candidates.map((candidate, position) => {
          const selected = needsConfirm
            ? picked === position
            : stepRun.status === "succeeded" &&
              stepRun.selected_index === position;
          return (
            <div
              key={`${fingerprint}-${position}`}
              className={cn(
                "space-y-3 rounded-xl border p-3",
                selected
                  ? "border-primary bg-primary/5"
                  : "border-border bg-background",
              )}
            >
              <label className="flex items-center gap-2 text-sm font-medium">
                <input
                  type="radio"
                  name={`candidate-${stepRun.id}`}
                  aria-label={t("execution.candidate", {
                    number: position + 1,
                  })}
                  checked={selected}
                  disabled={!needsConfirm || locked}
                  onChange={() => setSelection({ fingerprint, position })}
                />
                {t("execution.candidate", { number: position + 1 })}
                {selected && !needsConfirm ? (
                  <span className="ml-auto text-xs text-success">
                    {t("execution.accepted")}
                  </span>
                ) : null}
              </label>
              {candidate.url ? (
                <>
                  <CandidatePreview
                    url={candidate.url}
                    label={t("execution.candidate", { number: position + 1 })}
                  />
                  <a
                    href={candidate.url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-primary underline"
                  >
                    {t("execution.openMedia")}
                  </a>
                </>
              ) : null}
              {candidate.content ? (
                <p className="max-h-40 overflow-auto whitespace-pre-wrap text-sm leading-6">
                  {candidate.content}
                </p>
              ) : null}
            </div>
          );
        })}
      </div>
      {needsConfirm ? (
        <div className="space-y-3">
          <Button
            className="w-full"
            disabled={locked || !validCandidatePosition(stepRun, picked)}
            onClick={() =>
              submit({
                key: requestKey(),
                fingerprint,
                decision: "accept",
                position: picked!,
              })
            }
          >
            {t("execution.confirm")}
          </Button>
          <Button
            className="w-full"
            variant="outline"
            disabled={locked}
            onClick={() => setFeedbackOpen((open) => !open)}
          >
            {t("execution.regenerate")}
          </Button>
          {feedbackOpen ? (
            <div className="space-y-2">
              <Textarea
                aria-label={t("execution.feedback")}
                placeholder={t("execution.feedbackHint")}
                value={feedback}
                onChange={(event) => setFeedback(event.target.value)}
                disabled={locked}
              />
              <p className="text-xs text-muted-foreground">
                {feedbackBytes(feedback)} / 8192 bytes
              </p>
              <Button
                disabled={locked || feedbackBytes(feedback) > 8192}
                onClick={() =>
                  submit({
                    key: requestKey(),
                    fingerprint,
                    decision: "regenerate",
                    feedback,
                  })
                }
              >
                {t("execution.sendFeedback")}
              </Button>
            </div>
          ) : null}
        </div>
      ) : null}
      {stepRun.output_asset_id && (stepRun.result_source === "reused" || stepRun.result_source === "generated") ? (
        <p className="text-xs text-muted-foreground">
          {stepRun.result_source === "reused"
            ? t("execution.resultReused")
            : stepRun.result_source === "generated"
              ? t("execution.resultGenerated")
              : null}
        </p>
      ) : null}
      {stepRun.output_asset_id ? (
        <details className="text-xs">
          <summary className="cursor-pointer">
            {t("execution.assetReference")}
          </summary>
          <p className="mt-2 break-all text-muted-foreground">
            {stepRun.output_asset_id}
          </p>
          {stepRun.result_source === "reused" && stepRun.reused_from_step_run_id ? (
            <p className="mt-2 break-all text-muted-foreground">
              {t("execution.reusedFromStep", { id: stepRun.reused_from_step_run_id })}
            </p>
          ) : null}
        </details>
      ) : null}
      {stepRun.rendered_prompt ? (
        <details className="text-sm">
          <summary className="cursor-pointer font-medium">
            {t("execution.prompt")}
          </summary>
          <p className="mt-2 max-h-60 overflow-auto whitespace-pre-wrap rounded-xl bg-muted/50 p-3 text-xs leading-6">
            {stepRun.rendered_prompt}
          </p>
        </details>
      ) : null}
      {!stepRun.candidates.length && !stepRun.output_asset_id ? (
        <p className="py-6 text-center text-sm text-muted-foreground">
          {t("execution.noResults")}
        </p>
      ) : null}
    </article>
  );
}
