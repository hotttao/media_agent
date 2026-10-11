import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import { LibraryMedia } from "@/foundation/components/library-media";
import { ListState } from "@/foundation/components/resource-list-layout";
import { hostShim } from "@/foundation/lib/host-shim";
import type { Workflow } from "@/domains/workflows/types";
import type { WorkflowInputSchemaEntry } from "@/domains/jobs/types";
import { WorkflowGraph } from "@/domains/workflows/components/workflow-graph";
import { runInputGroups } from "./run-input-groups";
import { selectedInputPreview } from "./job-input";
import type { JobInputOptions } from "./use-job-input-options";

export function RunWizard(props: {
  instanceId: string;
  organizationId: string;
  organizationName?: string;
  launchKey: string;
  workflow: Workflow;
  entries: WorkflowInputSchemaEntry[];
  selected: Record<string, string>;
  options: JobInputOptions;
  workspaceId: string;
  workspaces: { id: string; name: string }[];
  busy: boolean;
  loading: boolean;
  error?: string;
  submitError?: string;
  canSubmit: boolean;
  replay: boolean;
  onRetry: () => void;
  onChoose: (name: string, id: string) => void;
  onWorkspace: (id: string) => void;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  const [panelOpen, setPanelOpen] = useState(true);
  const plan = useMemo(() => {
    try {
      return {
        ...runInputGroups(props.workflow.steps, props.entries),
        error: false,
      };
    } catch {
      return { groups: [], steps: props.workflow.steps, error: true };
    }
  }, [props.workflow.steps, props.entries]);
  const [cursor, setCursor] = hostShim.useInstanceState(
    props.instanceId,
    `run_cursor_${props.organizationId}_${props.workflow.id}_${props.workflow.version}_${props.launchKey}`,
    () => 0,
  );
  const review = cursor >= plan.groups.length;
  const active = plan.groups[cursor];
  const complete =
    active?.entries.every(
      (entry) =>
        !entry.required ||
        props.options[entry.name]?.some(
          (item) => item.id === props.selected[entry.name],
        ),
    ) ?? true;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <AppStudioHeader>
        <div className="min-w-0">
          <h1 className="truncate font-semibold">{props.workflow.name}</h1>
          <p className="text-xs text-muted-foreground">
            {t("runWizard.preparation")} · v{props.workflow.version}
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          disabled={props.busy}
          onClick={props.onCancel}
        >
          {t("common.cancel")}
        </Button>
      </AppStudioHeader>
      <div
        className="workflow-run-layout relative min-h-0 flex-1 overflow-hidden"
        data-panel-open={panelOpen}
      >
        <Button
          className="workflow-run-toggle absolute left-3 top-3 z-20"
          variant="outline"
          size="sm"
          aria-expanded={panelOpen}
          onClick={() => setPanelOpen(!panelOpen)}
        >
          {t(panelOpen ? "runWizard.hideInputs" : "runWizard.showInputs")}
        </Button>
        <div className="workflow-run-canvas relative min-h-0">
          <WorkflowGraph
            model={{
              steps: plan.steps,
              outputStepKey: props.workflow.output_step_key,
              layout: "studio",
            }}
            selectedNodeId={active?.stepKey}
            onNodeSelect={(key) => {
              const index = plan.groups.findIndex(
                (group) => group.stepKey === key,
              );
              if (!props.busy && index >= 0 && index <= cursor)
                setCursor(index);
            }}
          />
        </div>
        <aside
          className="workflow-run-panel flex min-h-0 flex-col border-l bg-background"
          aria-label={t("runWizard.preparation")}
        >
          <div className="border-b p-4">
            <p className="text-sm text-muted-foreground">
              {t("runWizard.progress", {
                current: Math.min(cursor + 1, plan.groups.length + 1),
                total: plan.groups.length + 1,
              })}
            </p>
            <h2 className="mt-1 font-semibold">
              {review ? t("runWizard.review") : active?.stepKey}
            </h2>
            <p className="mt-2 text-xs text-muted-foreground">
              {t("runWizard.notExecuting")}
            </p>
          </div>
          <div className="min-h-0 flex-1 space-y-5 overflow-auto p-4">
            {props.submitError ? (
              <p role="alert" className="text-sm text-destructive">
                {props.submitError}
              </p>
            ) : null}
            {plan.error ? (
              <p role="alert">{t("runWizard.invalid")}</p>
            ) : props.loading ? (
              <ListState loading />
            ) : props.error ? (
              <ListState error={props.error} onRetry={props.onRetry} />
            ) : review ? (
              <>
                <p className="text-sm text-muted-foreground">
                  {t("runWizard.company")}：
                  {props.organizationName || props.organizationId}
                </p>
                <label className="grid gap-2 text-sm">
                  {t("editorContext.workspace")}
                  <select
                    className="h-10 rounded-lg border bg-background px-3"
                    value={props.workspaceId}
                    disabled={props.busy}
                    onChange={(event) => props.onWorkspace(event.target.value)}
                  >
                    <option value="">
                      {t("editorContext.chooseWorkspace")}
                    </option>
                    {props.workspaces.map((workspace) => (
                      <option key={workspace.id} value={workspace.id}>
                        {workspace.name}
                      </option>
                    ))}
                  </select>
                </label>
                {props.entries.map((entry) => {
                  const item = props.options[entry.name]?.find(
                    (option) => option.id === props.selected[entry.name],
                  );
                  const preview = selectedInputPreview(
                    entry.resource_type,
                    item,
                  );
                  return (
                    <section
                      className="flex items-center gap-3 border-b pb-3"
                      key={entry.name}
                    >
                      {preview?.paths[0] ? (
                        <div className="w-20 shrink-0">
                          <LibraryMedia
                            organizationId={props.organizationId}
                            objectPath={preview.paths[0]}
                            alt={item?.name || entry.name}
                          />
                        </div>
                      ) : null}
                      <div className="min-w-0">
                        <p className="text-xs text-muted-foreground">
                          {entry.name}
                        </p>
                        <p className="truncate text-sm">
                          {item?.name ||
                            (entry.required
                              ? t("runForm.chooseResource")
                              : t("runWizard.skipped"))}
                        </p>
                      </div>
                    </section>
                  );
                })}
                <p className="text-xs text-muted-foreground">
                  {t("runWizard.approvalHint")}
                </p>
              </>
            ) : (
              active?.entries.map((entry) => (
                <InputCards
                  key={entry.name}
                  entry={entry}
                  options={props.options[entry.name] ?? []}
                  selected={props.selected[entry.name] || ""}
                  organizationId={props.organizationId}
                  disabled={props.busy}
                  onChoose={(id) => props.onChoose(entry.name, id)}
                />
              ))
            )}
          </div>
          <div className="flex items-center justify-between gap-3 border-t p-4">
            <Button
              variant="outline"
              disabled={cursor <= 0 || props.busy}
              onClick={() => setCursor(Math.max(0, cursor - 1))}
            >
              {t("runWizard.previous")}
            </Button>
            {review ? (
              <Button
                disabled={
                  !props.canSubmit ||
                  plan.error ||
                  props.busy ||
                  props.loading ||
                  Boolean(props.error)
                }
                onClick={props.onSubmit}
              >
                {t(
                  props.busy
                    ? "runForm.submitting"
                    : props.replay
                      ? "runForm.replay"
                      : "runForm.submit",
                )}
              </Button>
            ) : (
              <Button
                disabled={
                  !complete ||
                  plan.error ||
                  props.busy ||
                  props.loading ||
                  Boolean(props.error)
                }
                onClick={() => setCursor(cursor + 1)}
              >
                {t("runWizard.next")}
              </Button>
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}

function InputCards({
  entry,
  options,
  selected,
  organizationId,
  disabled,
  onChoose,
}: {
  entry: WorkflowInputSchemaEntry;
  options: JobInputOptions[string];
  selected: string;
  organizationId: string;
  disabled: boolean;
  onChoose: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const labelKey = (
    {
      product: "product",
      virtual_ip: "character",
      movement: "movement",
      pose: "pose",
      scene: "scene",
    } as Record<string, string>
  )[entry.name];
  const label = labelKey ? t(`runForm.fields.${labelKey}`) : entry.name;
  const [page, setPage] = useState(0);
  const filtered = options.filter((item) => {
    const preview = selectedInputPreview(entry.resource_type, item);
    return `${item.name || ""} ${preview?.content || ""}`
      .toLocaleLowerCase()
      .includes(search.trim().toLocaleLowerCase());
  });
  const pages = Math.max(1, Math.ceil(filtered.length / 12));
  const current = Math.min(page, pages - 1);
  return (
    <section className="space-y-3" aria-label={entry.name}>
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">
          {label}
          {entry.required ? " *" : ""}
        </h3>
        <Button
          variant="ghost"
          size="sm"
          disabled={!selected || disabled}
          onClick={() => onChoose("")}
        >
          {t("runWizard.clear")}
        </Button>
      </div>
      <Input
        aria-label={t("runWizard.search", { name: label })}
        placeholder={t("runWizard.search", { name: label })}
        value={search}
        onChange={(event) => {
          setSearch(event.target.value);
          setPage(0);
        }}
      />
      {filtered.length ? (
        <div className="grid grid-cols-2 gap-3">
          {filtered.slice(current * 12, (current + 1) * 12).map((item) => {
            const preview = selectedInputPreview(entry.resource_type, item);
            return (
              <button
                type="button"
                key={item.id}
                data-testid={`run-choice-${entry.name}`}
                aria-pressed={selected === item.id}
                aria-label={item.name || item.id}
                disabled={disabled}
                className={`overflow-hidden rounded-xl border text-left transition-colors ${selected === item.id ? "border-primary bg-primary/5 ring-1 ring-primary" : "hover:border-primary/50"}`}
                onClick={() => onChoose(item.id)}
              >
                {preview?.paths[0] ? (
                  <div className="pointer-events-none aspect-[4/3] overflow-hidden">
                    <LibraryMedia
                      interactive={false}
                      organizationId={organizationId}
                      objectPath={preview.paths[0]}
                      alt={item.name || item.id}
                    />
                  </div>
                ) : (
                  <p className="line-clamp-4 min-h-20 p-3 text-xs text-muted-foreground">
                    {preview?.content || t("runWizard.noImage")}
                  </p>
                )}
                <p className="truncate p-2 text-xs font-medium">
                  {item.name || item.id}
                </p>
              </button>
            );
          })}
        </div>
      ) : (
        <p className="py-6 text-center text-sm text-muted-foreground">
          {t("runWizard.noMatches")}
        </p>
      )}
      {pages > 1 ? (
        <div className="flex justify-between">
          <Button
            size="sm"
            variant="ghost"
            disabled={current === 0}
            onClick={() => setPage(current - 1)}
          >
            {t("runWizard.previous")}
          </Button>
          <span className="text-xs">
            {current + 1} / {pages}
          </span>
          <Button
            size="sm"
            variant="ghost"
            disabled={current + 1 >= pages}
            onClick={() => setPage(current + 1)}
          >
            {t("runWizard.next")}
          </Button>
        </div>
      ) : null}
    </section>
  );
}
