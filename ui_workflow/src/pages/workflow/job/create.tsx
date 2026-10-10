import { useEffect, useState } from "react";
import type { WorkflowLaunchIntent } from "@/foundation/types/app-navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import { ListState } from "@/foundation/components/resource-list-layout";
import { LibraryMedia } from "@/foundation/components/library-media";
import { hostShim } from "@/foundation/lib/host-shim";
import { api } from "@/foundation/lib/api-client";
import { requestKey } from "@/foundation/lib/request-key";
import { resolveLibraryObject } from "@/foundation/lib/library-object";
import { useWorkflowsList } from "@/domains/jobs/hooks/use-workflows-list";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import { submitJob, resumeJob, retryJob } from "@/domains/jobs/lib/api";
import type {
  Job,
  StepRun,
  SubmitJobInput,
  WorkflowOption,
} from "@/domains/jobs/types";
import {
  buildJobInput,
  missingRequiredInputs,
  isWorkflowReady,
  selectedInputPreview,
  resumeInputContract,
} from "./job-input";
import { useJobInputOptions } from "./use-job-input-options";

export function JobCreate({
  onDone,
  onSubmitted,
  preferredWorkflowId,
  instanceId,
  sourceJob,
  sourceStepRuns,
  mode = "submit",
  launchIntent,
}: {
  onDone: () => void;
  onSubmitted: (id: string) => void;
  preferredWorkflowId?: string;
  instanceId: string;
  sourceJob?: Job;
  sourceStepRuns?: StepRun[];
  mode?: "submit" | "resume" | "rerun";
  launchIntent?: WorkflowLaunchIntent;
}) {
  const { t } = useTranslation();
  const shell = hostShim.useShellContext();
  const qc = useQueryClient();
  const workflows = useWorkflowsList();
  const [draft, setDraft] = hostShim.useInstanceState(
    instanceId,
    `workflow_run_draft_${mode}_${launchIntent?.requestId || sourceJob?.id || preferredWorkflowId || "new"}`,
    () => ({
      workflowId: sourceJob?.workflow_id || preferredWorkflowId || "",
      workspaceId: sourceJob?.workspace_id || "",
      selected: {} as Record<string, string>,
    }),
  );
  const [intent, setIntent] = hostShim.useInstanceState<{
    fingerprint: string;
    request: SubmitJobInput;
  } | null>(
    instanceId,
    `workflow_run_intent_${mode}_${launchIntent?.requestId || sourceJob?.id || preferredWorkflowId || "new"}`,
    () => null,
  );
  const [preparing, setPreparing] = useState(false);
  const definition = useWorkflow(
    sourceJob?.workflow_id || draft.workflowId,
    sourceJob?.workflow_version,
  );
  const [seeded, setSeeded] = hostShim.useInstanceState(
    instanceId,
    `workflow_product_seed_${launchIntent?.requestId || "none"}`,
    () => false,
  );
  const wrongCompany = Boolean(
    launchIntent &&
      shell.organization.id &&
      launchIntent.organizationId !== shell.organization.id,
  );
  useEffect(() => {
    if (!launchIntent || seeded || !definition.data || shell.isLoading) return;
    setSeeded(true);
    if (wrongCompany || draft.workflowId !== launchIntent.workflowId) return;
    const entries = definition.data.input_schema?.entries as
      | { name: string; resource_type: string }[]
      | undefined;
    const entry = entries?.find((item) => item.resource_type === "PRODUCT");
    if (entry)
      setDraft((current) => ({
        ...current,
        selected: { ...current.selected, [entry.name]: launchIntent.productId },
      }));
  }, [
    launchIntent,
    seeded,
    definition.data,
    shell.isLoading,
    wrongCompany,
    draft.workflowId,
    setSeeded,
    setDraft,
  ]);
  const [error, setError] = useState("");
  const workspaces = useQuery({
    queryKey: ["available-workspaces", shell.organization.id],
    enabled: Boolean(shell.organization.id) && !sourceJob,
    queryFn: () =>
      api.request<{
        workspaces?: { id: string; name: string; organization_id: string }[];
      }>("/workspaces"),
  });
  const selectedWorkflow: WorkflowOption | undefined = definition.data
    ? {
        ...definition.data,
        // Historical IR did not retain Scope metadata; the Job did. Never use
        // a newer definition's Scope or input contract to fill the gap.
        scope: sourceJob?.scope || definition.data.scope,
        input_schema: definition.data
          .input_schema as WorkflowOption["input_schema"],
      }
    : undefined;
  const ready = mode !== "submit" || isWorkflowReady(selectedWorkflow);
  const versionMismatch = Boolean(
    sourceJob &&
      selectedWorkflow &&
      selectedWorkflow.version !== sourceJob.workflow_version,
  );
  const effectiveWorkflow: WorkflowOption | undefined =
    selectedWorkflow && mode === "resume" && sourceJob
      ? resumeInputContract(
          selectedWorkflow,
          definition.data?.steps,
          sourceStepRuns,
          sourceJob,
        )
      : selectedWorkflow;
  const options = useJobInputOptions(
    effectiveWorkflow,
    Boolean(shell.organization.id) && !versionMismatch && ready,
    shell.organization.id,
  );
  const missing = missingRequiredInputs(effectiveWorkflow, draft.selected);
  const fingerprint = JSON.stringify([
    shell.organization.id,
    draft,
    selectedWorkflow?.version,
    mode,
  ]);
  const currentIntent = intent?.fingerprint === fingerprint ? intent : null;
  const mutation = useMutation({
    mutationFn: async (request: SubmitJobInput) => {
      if (mode === "resume" && sourceJob) {
        if (!request.idempotency_key) throw new Error(t("runForm.failed"));
        return {
          id: (
            await resumeJob(
              sourceJob.id,
              request.input,
              request.idempotency_key,
            )
          ).id,
        };
      }
      if (mode === "rerun" && sourceJob)
        return {
          id: (await retryJob(sourceJob.id, { input_override: request.input }))
            .id,
        };
      return submitJob(request);
    },
    onSuccess: (receipt) => {
      setIntent(null);
      setDraft({
        workflowId: preferredWorkflowId || "",
        workspaceId: "",
        selected: {},
      });
      qc.invalidateQueries({ queryKey: ["jobs"] });
      qc.invalidateQueries({ queryKey: ["job", receipt.id] });
      onSubmitted(receipt.id);
    },
  });
  const busy = preparing || mutation.isPending;
  const choices = (workspaces.data?.workspaces ?? []).filter(
    (workspace) => workspace.organization_id === shell.organization.id,
  );
  const validWorkspace = Boolean(
    shell.organization.id &&
      draft.workspaceId &&
      (sourceJob
        ? sourceJob.workspace_id === draft.workspaceId &&
          sourceJob.organization_id === shell.organization.id
        : choices.some((workspace) => workspace.id === draft.workspaceId)),
  );
  const validSelections = Object.entries(draft.selected).every(
    ([name, id]) => !id || options.data?.[name]?.some((item) => item.id === id),
  );
  async function submit() {
    if (
      !effectiveWorkflow ||
      !validWorkspace ||
      !validSelections ||
      wrongCompany ||
      !ready ||
      missing.length ||
      versionMismatch ||
      busy
    )
      return;
    setPreparing(true);
    setError("");
    try {
      const request = currentIntent?.request ?? {
        workflow_id: effectiveWorkflow.id,
        workflow_version:
          sourceJob?.workflow_version ?? effectiveWorkflow.version,
        workspace_id: draft.workspaceId,
        organization_id: sourceJob?.organization_id || shell.organization.id,
        idempotency_key: requestKey(),
        input: await buildJobInput(
          effectiveWorkflow,
          draft.selected,
          options.data ?? {},
          (path) => resolveLibraryObject(shell.organization.id, path),
        ),
      };
      setIntent({ fingerprint, request });
      await mutation.mutateAsync(request);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("runForm.failed"));
    } finally {
      setPreparing(false);
    }
  }
  return (
    <div className="flex h-full min-h-0 flex-col">
      <AppStudioHeader>
        <h1 className="truncate text-base font-semibold">
          {t(`runForm.${mode}`)}
        </h1>
        <div className="flex shrink-0 gap-2">
          <Button variant="ghost" size="sm" disabled={busy} onClick={onDone}>
            {t("common.cancel")}
          </Button>
          <Button
            size="sm"
            disabled={
              busy ||
              options.isLoading ||
              options.isError ||
              !selectedWorkflow ||
              !effectiveWorkflow ||
              !validWorkspace ||
              !validSelections ||
              wrongCompany ||
              !ready ||
              missing.length > 0 ||
              versionMismatch ||
              Boolean(currentIntent && mode === "rerun")
            }
            onClick={() => void submit()}
          >
            {t(
              busy
                ? "runForm.submitting"
                : currentIntent
                  ? "runForm.replay"
                  : "runForm.submit",
            )}
          </Button>
        </div>
      </AppStudioHeader>
      <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="mx-auto max-w-4xl space-y-6">
          <p className="text-sm leading-6 text-muted-foreground">
            {t(`runForm.${mode}Hint`)}
          </p>
          {wrongCompany ? (
            <p role="alert" className="text-sm text-destructive">
              {t("productWorkflow.wrongCompany")}
            </p>
          ) : null}
          {!validSelections && options.isSuccess ? (
            <p role="alert" className="text-sm text-destructive">
              {t("productWorkflow.staleSelection")}
            </p>
          ) : null}
          {definition.isLoading ? (
            <ListState loading />
          ) : definition.isError ? (
            <ListState
              error={definition.error.message}
              onRetry={() => void definition.refetch()}
            />
          ) : selectedWorkflow && !ready ? (
            <p role="alert" className="text-sm text-destructive">
              {t("runForm.notRegistered")}
            </p>
          ) : null}
          {workflows.isLoading ? (
            <ListState loading />
          ) : workflows.isError ? (
            <ListState
              error={workflows.error.message}
              onRetry={() => void workflows.refetch()}
            />
          ) : (
            <div className="grid gap-4 rounded-2xl bg-background p-5 sm:grid-cols-2">
              <div className="grid gap-2">
                <label htmlFor="run-workflow">{t("nav.workflows")}</label>
                <select
                  id="run-workflow"
                  disabled={Boolean(sourceJob) || busy}
                  className="h-10 rounded-lg border bg-background px-3"
                  value={draft.workflowId}
                  onChange={(event) => {
                    setDraft({
                      ...draft,
                      workflowId: event.target.value,
                      selected: {},
                    });
                    setIntent(null);
                  }}
                >
                  <option value="">{t("runForm.chooseWorkflow")}</option>
                  {workflows.data?.map((workflow) => (
                    <option key={workflow.id} value={workflow.id}>
                      {workflow.name} · v{workflow.version}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-2">
                <label htmlFor="run-workspace">
                  {t("editorContext.workspace")}
                </label>
                <select
                  id="run-workspace"
                  className="h-10 rounded-lg border bg-background px-3"
                  disabled={Boolean(sourceJob) || workspaces.isLoading || busy}
                  value={draft.workspaceId}
                  onChange={(event) => {
                    setDraft({ ...draft, workspaceId: event.target.value });
                    setIntent(null);
                  }}
                >
                  <option value="">{t("editorContext.chooseWorkspace")}</option>
                  {sourceJob ? (
                    <option value={sourceJob.workspace_id}>
                      {sourceJob.workspace_id}
                    </option>
                  ) : (
                    choices.map((workspace) => (
                      <option key={workspace.id} value={workspace.id}>
                        {workspace.name}
                      </option>
                    ))
                  )}
                </select>
              </div>
            </div>
          )}
          {workspaces.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {workspaces.error.message}
            </p>
          ) : null}
          {(mode === "resume" && selectedWorkflow && !effectiveWorkflow) ||
          versionMismatch ||
          (sourceJob && !selectedWorkflow && !workflows.isLoading) ? (
            <p role="alert" className="rounded-xl bg-warning/10 p-4 text-sm">
              {t("runForm.frozenUnavailable")}
            </p>
          ) : options.isLoading ? (
            <ListState loading />
          ) : options.isError ? (
            <ListState
              error={options.error.message}
              onRetry={() => void options.refetch()}
            />
          ) : selectedWorkflow ? (
            <div className="grid gap-4 sm:grid-cols-2">
              {(effectiveWorkflow?.input_schema?.entries ?? []).map((entry) => {
                const resource = options.data?.[entry.name]?.find(
                  (item) => item.id === draft.selected[entry.name],
                );
                const preview = selectedInputPreview(
                  entry.resource_type,
                  resource,
                );
                const labelKey = (
                  {
                    product: "product",
                    virtual_ip: "character",
                    movement: "movement",
                    pose: "pose",
                    scene: "scene",
                  } as Record<string, string>
                )[entry.name];
                return (
                  <section
                    key={entry.name}
                    className="space-y-3 rounded-2xl bg-background p-5"
                  >
                    <label
                      htmlFor={`run-input-${entry.name}`}
                      className="font-semibold"
                    >
                      {labelKey ? t(`runForm.fields.${labelKey}`) : entry.name}
                      {entry.required ? " *" : ""}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {entry.name} · {entry.asset_kind || entry.resource_type}
                    </p>
                    <select
                      id={`run-input-${entry.name}`}
                      data-testid={`run-input-${entry.name}`}
                      disabled={busy}
                      className="h-10 w-full rounded-lg border bg-background px-3"
                      value={draft.selected[entry.name] || ""}
                      onChange={(event) => {
                        setDraft({
                          ...draft,
                          selected: {
                            ...draft.selected,
                            [entry.name]: event.target.value,
                          },
                        });
                        setIntent(null);
                      }}
                    >
                      <option value="">{t("runForm.chooseResource")}</option>
                      {(options.data?.[entry.name] ?? []).map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.name || item.id}
                        </option>
                      ))}
                    </select>
                    {preview ? (
                      <div className="space-y-2 [&_img]:max-h-48 [&_video]:max-h-48">
                        {preview.paths.map((path) => (
                          <LibraryMedia
                            key={path}
                            organizationId={shell.organization.id}
                            objectPath={path}
                            alt={resource?.name || entry.name}
                          />
                        ))}
                        {preview.content ? (
                          <p className="line-clamp-4 whitespace-pre-wrap text-xs leading-6 text-muted-foreground">
                            {preview.content}
                          </p>
                        ) : null}
                      </div>
                    ) : null}
                    {!options.data?.[entry.name]?.length ? (
                      <p className="text-xs text-muted-foreground">
                        {t("runForm.noCompatible")}
                      </p>
                    ) : null}
                  </section>
                );
              })}
            </div>
          ) : null}
          {error ? (
            <p
              role="alert"
              className="rounded-xl bg-destructive/10 p-4 text-sm text-destructive"
            >
              {error}
            </p>
          ) : null}
          {currentIntent ? (
            <p className="rounded-xl bg-warning/10 p-4 text-sm">
              {t(
                mode === "rerun"
                  ? "runForm.rerunUnknown"
                  : "runForm.intentRetained",
              )}
            </p>
          ) : null}
        </div>
      </div>
    </div>
  );
}
