import {
  AlertCircle,
  ArrowRight,
  ChevronDown,
  Plus,
  Settings2,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { useMemo, useState, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { WorkflowGraph } from "@/domains/workflows/components/workflow-graph";
import type {
  SlotBinding,
  SlotBindingSource,
  ValidationResult,
  Visibility,
  WorkflowInput,
  WorkflowStep,
} from "@/domains/workflows/types";
import { cn } from "@/foundation/lib/utils";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
import {
  updateDraftStep,
  removeDraftStep,
  removalImpact,
} from "@/domains/workflows/lib/draft-editing";

export type WorkflowTemplateOption = {
  id: string;
  name: string;
  version: number;
  status: string;
  inputs?: { name: string; required?: boolean }[];
};

function nextStepKey(steps: WorkflowStep[]) {
  let index = steps.length + 1;
  while (steps.some((step) => step.key === `step_${index}`)) index += 1;
  return `step_${index}`;
}

function StepBindingEditor({
  binding,
  stepIndex,
  bindingIndex,
  steps,
  declaredSlot,
  onChange,
  onRemove,
}: {
  binding: SlotBinding;
  stepIndex: number;
  bindingIndex: number;
  steps: WorkflowStep[];
  declaredSlot?: { name: string; required?: boolean };
  onChange: (binding: SlotBinding) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const previousSteps = steps.slice(0, stepIndex).filter((step) => step.key);
  return (
    <div className="grid gap-2 rounded-xl border border-border/70 bg-background/70 p-3">
      <div className="grid grid-cols-[1fr_auto] gap-2">
        <div className="grid gap-1.5">
          <Label
            htmlFor={`binding-slot-${stepIndex}-${bindingIndex}`}
            className="text-[11px] text-muted-foreground"
          >
            {t("editorUI.neededInput")}
          </Label>
          {declaredSlot ? (
            <div
              id={`binding-slot-${stepIndex}-${bindingIndex}`}
              className="flex h-9 items-center rounded-md bg-muted px-3 text-sm font-medium"
            >
              {declaredSlot.name}
              {declaredSlot.required ? (
                <span className="ml-1 text-destructive">*</span>
              ) : null}
            </div>
          ) : (
            <Input
              id={`binding-slot-${stepIndex}-${bindingIndex}`}
              value={binding.to_input}
              placeholder={t("editorUI.bindingExample")}
              onChange={(event) =>
                onChange({ ...binding, to_input: event.target.value })
              }
            />
          )}
        </div>
        {!declaredSlot?.required ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="self-end text-muted-foreground hover:text-destructive"
            aria-label={t("editorUI.removeBinding")}
            onClick={onRemove}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        ) : null}
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label className="text-[11px] text-muted-foreground">
            {t("editorUI.source")}
          </Label>
          <Select
            value={binding.source}
            onValueChange={(source) =>
              onChange({
                ...binding,
                source: source as SlotBindingSource,
                ref: source === "const" ? undefined : binding.ref,
                const_value:
                  source === "const" ? (binding.const_value ?? "") : undefined,
              })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="workflow_input">
                {t("editorUI.workflowInput")}
              </SelectItem>
              <SelectItem value="step_output">
                {t("editorUI.upstreamOutput")}
              </SelectItem>
              <SelectItem value="const">{t("editorUI.constant")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-1.5">
          <Label className="text-[11px] text-muted-foreground">
            {binding.source === "const"
              ? t("editorUI.constant")
              : binding.source === "step_output"
                ? t("editorUI.upstream")
                : t("editorUI.inputName")}
          </Label>
          {binding.source === "step_output" && previousSteps.length > 0 ? (
            <Select
              value={binding.ref ?? ""}
              onValueChange={(ref) => onChange({ ...binding, ref })}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("editorUI.chooseUpstream")} />
              </SelectTrigger>
              <SelectContent>
                {previousSteps.map((step) => (
                  <SelectItem key={step.key} value={step.key}>
                    {step.key}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              aria-label={`${binding.source === "const" ? t("editorBinding.constant") : t("editorBinding.inputName")} ${binding.to_input}`}
              value={
                binding.source === "const"
                  ? String(binding.const_value ?? "")
                  : (binding.ref ?? "")
              }
              placeholder={
                binding.source === "const"
                  ? t("editorUI.constantHint")
                  : t("editorUI.inputExample")
              }
              onChange={(event) =>
                onChange(
                  binding.source === "const"
                    ? { ...binding, const_value: event.target.value }
                    : { ...binding, ref: event.target.value },
                )
              }
            />
          )}
        </div>
      </div>
    </div>
  );
}

/** Workflow 的唯一编辑源仍是业务 DSL；FlowGram 只承载画布和依赖可视化。 */
export function WorkflowForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  editable = true,
  onCancel,
  onValidate,
  validationResult,
  templateOptions = [],
  metadataLocked = false,
  error,
  submitDisabled = false,
  resolveTemplate,
  settingsExtra,
}: {
  value: WorkflowInput;
  onChange: (v: WorkflowInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  editable?: boolean;
  onCancel: () => void;
  onValidate?: () => void;
  validationResult?: ValidationResult | null;
  templateOptions?: WorkflowTemplateOption[];
  metadataLocked?: boolean;
  error?: string;
  submitDisabled?: boolean;
  resolveTemplate?: (
    template: WorkflowTemplateOption,
  ) => Promise<WorkflowTemplateOption>;
  settingsExtra?: ReactNode;
}) {
  const { t } = useTranslation();
  const [selectedStep, setSelectedStep] = useState(-1);
  const [removeIndex, setRemoveIndex] = useState<number | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(!value.name);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [templateSearch, setTemplateSearch] = useState("");
  const [templatePending, setTemplatePending] = useState(false);
  const [templateError, setTemplateError] = useState("");
  const latestValue = useRef(value);
  latestValue.current = value;
  const graphModel = useMemo(
    () => ({
      steps: value.steps,
      layout: "studio" as const,
      outputStepKey: value.output_step_key,
      templateNameByID: Object.fromEntries(
        templateOptions.map((template) => [template.id, template.name]),
      ),
    }),
    [value.output_step_key, value.steps, templateOptions],
  );

  function updateStep(index: number, patch: Partial<WorkflowStep>) {
    onChange(updateDraftStep(value, index, patch));
  }

  function addStep() {
    setTemplateError("");
    setPickerOpen(true);
  }

  async function selectTemplate(option: WorkflowTemplateOption) {
    setTemplatePending(true);
    setTemplateError("");
    try {
      const template = resolveTemplate ? await resolveTemplate(option) : option;
      if (
        template.id !== option.id ||
        template.version !== option.version ||
        !template.inputs
      ) {
        throw new Error(t("studio.templateContractError"));
      }
      const current = latestValue.current;
      const key = nextStepKey(current.steps);
      const step: WorkflowStep = {
        key,
        template_id: template?.id ?? "",
        template_version: template?.version,
        slot_bindings: (template?.inputs ?? []).map((slot) => ({
          to_input: slot.name,
          source: "workflow_input" as const,
          ref: slot.name,
        })),
        approval_policy: "when_multiple_candidates",
        reuse_policy: "reuse_if_available",
      };
      const steps = [...current.steps, step];
      onChange({
        ...current,
        steps,
        output_step_key: current.output_step_key || key,
      });
      setSelectedStep(steps.length - 1);
      setPickerOpen(false);
    } catch (error) {
      setTemplateError(
        error instanceof Error
          ? error.message
          : t("studio.templateContractError"),
      );
    } finally {
      setTemplatePending(false);
    }
  }

  function removeStep(index: number) {
    const removed = value.steps[index];
    if (!removed) return;
    onChange(removeDraftStep(value, removed.key));
    setSelectedStep(-1);
    setRemoveIndex(null);
  }

  const activeStep = value.steps[selectedStep];
  return (
    <div className="flex min-h-0 flex-1 flex-col bg-background">
      <AppStudioHeader>
        <div className="min-w-0">
          <p className="truncate text-base font-semibold">
            {value.name || t("editorUI.untitled")}
          </p>
          <p className="text-xs text-muted-foreground">
            {t("editorUI.stepCount", { count: value.steps.length })}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={addStep}
            disabled={isPending}
          >
            <Plus className="h-4 w-4" />
            {t("editor.addNode")}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setSettingsOpen((open) => !open)}
            aria-expanded={settingsOpen}
          >
            <Settings2 className="mr-1.5 h-4 w-4" />
            {t("editorUI.settings")}
            <ChevronDown
              className={cn(
                "ml-1 h-3.5 w-3.5 transition-transform",
                settingsOpen && "rotate-180",
              )}
            />
          </Button>
          {onValidate ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onValidate}
              disabled={isPending}
            >
              {t("editorUI.validate")}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            onClick={onSubmit}
            disabled={
              !editable ||
              isPending ||
              templatePending ||
              submitDisabled ||
              !value.name ||
              !value.scope ||
              !value.output_step_key ||
              value.steps.length === 0
            }
          >
            {submitLabel}
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
            {t("editorUI.exit")}
          </Button>
        </div>
      </AppStudioHeader>
      <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t("studio.settings")}</DialogTitle>
            <DialogDescription>{t("studio.settingsHint")}</DialogDescription>
          </DialogHeader>
          <section
            aria-label={t("editorUI.settings")}
            className="grid gap-4 sm:grid-cols-2"
          >
            {settingsExtra}
            <div className="grid gap-2">
              <Label htmlFor="workflow-name">{t("editorUI.name")}</Label>
              <Input
                id="workflow-name"
                value={value.name}
                placeholder={t("editorUI.nameHint")}
                onChange={(e) => onChange({ ...value, name: e.target.value })}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="workflow-scope">{t("editorUI.scope")}</Label>
              <Input
                id="workflow-scope"
                value={value.scope}
                disabled={metadataLocked}
                placeholder={t("editorUI.scopeHint")}
                onChange={(e) => onChange({ ...value, scope: e.target.value })}
              />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="workflow-description">
                {t("editorUI.description")}
              </Label>
              <Textarea
                id="workflow-description"
                rows={2}
                value={value.description ?? ""}
                placeholder={t("editorUI.descriptionHint")}
                onChange={(e) =>
                  onChange({ ...value, description: e.target.value })
                }
              />
            </div>
            <div className="grid gap-2">
              <Label>{t("editorUI.output")}</Label>
              <Select
                value={value.output_step_key}
                onValueChange={(output_step_key) =>
                  onChange({ ...value, output_step_key })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder={t("editorUI.chooseOutput")} />
                </SelectTrigger>
                <SelectContent>
                  {value.steps
                    .filter((step) => step.key)
                    .map((step) => (
                      <SelectItem key={step.key} value={step.key}>
                        {step.key}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="workflow-visibility">
                {t("editorUI.visibility")}
              </Label>
              <Select
                disabled
                value={value.visibility ?? "private"}
                onValueChange={(visibility) =>
                  onChange({ ...value, visibility: visibility as Visibility })
                }
              >
                <SelectTrigger id="workflow-visibility">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="private">
                    {t("editorUI.private")}
                  </SelectItem>
                  <SelectItem value="team">{t("editorUI.team")}</SelectItem>
                  <SelectItem value="public">{t("editorUI.public")}</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {t("studio.permissionsHint")}
              </p>
            </div>
          </section>
        </DialogContent>
      </Dialog>

      {error ? (
        <p role="alert" className="px-4 py-3 text-sm text-destructive">
          {error}
        </p>
      ) : null}
      <section
        className={cn("workflow-workbench", activeStep && "has-selection")}
      >
        <aside className="workflow-toolbox" aria-label={t("studio.toolbox")}>
          <div className="workflow-toolbox-title">{t("studio.toolbox")}</div>
          <button
            type="button"
            className="workflow-toolbox-node"
            onClick={addStep}
            disabled={isPending}
          >
            <span className="workflow-toolbox-icon">S</span>
            <span>
              <strong>{t("studio.generationNode")}</strong>
              <small>{t("studio.generationHint")}</small>
            </span>
            <Plus className="h-4 w-4 shrink-0" />
          </button>
          <div className="workflow-toolbox-title mt-7">
            {t("studio.inputSources")}
          </div>
          <p className="workflow-toolbox-help">{t("studio.inputHint")}</p>
          <div className="workflow-toolbox-sources">
            {["material", "asset", "product", "virtualIP"].map((kind) => (
              <span key={kind} data-kind={kind}>
                {t(`studio.${kind}`)}
              </span>
            ))}
          </div>
          <div className="mt-auto space-y-3 pt-8">
            <Label htmlFor="studio-node-navigation">
              {t("studio.locateNode")}
            </Label>
            <select
              id="studio-node-navigation"
              value={selectedStep < 0 ? "" : selectedStep}
              onChange={(event) => setSelectedStep(Number(event.target.value))}
            >
              <option value="" disabled>
                {t("editor.selectNode")}
              </option>
              {value.steps.map((step, index) => (
                <option key={`${step.key}-${index}`} value={index}>
                  {step.key}
                </option>
              ))}
            </select>
            <p className="workflow-toolbox-help">
              {t("studio.dependencyHint")}
            </p>
          </div>
        </aside>
        <div className="workflow-workbench-canvas">
          {value.steps.length > 0 ? (
            <WorkflowGraph
              model={graphModel}
              className="absolute inset-0"
              selectedNodeId={activeStep?.key}
              onNodeSelect={(nodeId) => {
                const index = value.steps.findIndex(
                  (step) => step.key === nodeId,
                );
                if (index >= 0) setSelectedStep(index);
              }}
            />
          ) : (
            <div className="flex h-full min-h-[420px] flex-col items-center justify-center px-6 text-center">
              <div className="rounded-2xl bg-primary/10 p-4 text-primary">
                <ArrowRight className="h-6 w-6" />
              </div>
              <h3 className="mt-4 font-semibold">{t("editorUI.begin")}</h3>
              <p className="mt-1 max-w-sm text-sm text-muted-foreground">
                {t("editorUI.beginHint")}
              </p>
              <Button type="button" className="mt-5" onClick={addStep}>
                <Plus className="mr-2 h-4 w-4" />
                {t("editorUI.add")}
              </Button>
            </div>
          )}
          {value.steps.length > 0 ? (
            <div className="absolute bottom-4 left-4 max-w-[80%] rounded-full border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground shadow-sm backdrop-blur pointer-events-none">
              {t("studio.canvasLegend")}
            </div>
          ) : null}
        </div>

        {activeStep ? (
          <aside className="workflow-workbench-panel">
            <div className="flex items-center justify-between border-b px-4 py-3">
              <div>
                <h3 className="font-semibold">{t("editorUI.configuration")}</h3>
                <p className="text-xs text-muted-foreground">
                  {t("editorUI.configurationHint")}
                </p>
              </div>
              {activeStep ? (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setSelectedStep(-1)}
                >
                  {t("editor.closePanel")}
                </Button>
              ) : null}
            </div>
            {activeStep ? (
              <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-xs font-medium text-primary">
                      {t("editorUI.stepNumber", { number: selectedStep + 1 })}
                    </div>
                    <h4 className="mt-1 font-semibold">
                      {activeStep.key || t("editorUI.untitledStep")}
                    </h4>
                  </div>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="text-muted-foreground hover:text-destructive"
                    aria-label={t("editorUI.removeStep")}
                    onClick={() => setRemoveIndex(selectedStep)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="workflow-step-key">{t("editorUI.key")}</Label>
                  <Input
                    id="workflow-step-key"
                    value={activeStep.key}
                    onChange={(e) =>
                      updateStep(selectedStep, { key: e.target.value })
                    }
                  />
                </div>
                <div className="grid gap-2">
                  <Label>{t("editorUI.template")}</Label>
                  {templateOptions.length > 0 ? (
                    <Select
                      disabled={templatePending}
                      value={`${activeStep.template_id}@${activeStep.template_version ?? ""}`}
                      onValueChange={async (reference) => {
                        const option = templateOptions.find(
                          (item) => `${item.id}@${item.version}` === reference,
                        );
                        if (!option) return;
                        setTemplatePending(true);
                        setTemplateError("");
                        try {
                          const template = resolveTemplate
                            ? await resolveTemplate(option)
                            : option;
                          if (
                            template.id !== option.id ||
                            template.version !== option.version ||
                            !template.inputs
                          )
                            throw new Error(t("studio.templateContractError"));
                          const template_id = template.id;
                          const current = latestValue.current;
                          const targetIndex = current.steps.findIndex(
                            (step) => step.key === activeStep.key,
                          );
                          if (targetIndex < 0) return;
                          const existing = new Map(
                            current.steps[targetIndex].slot_bindings.map(
                              (binding) => [binding.to_input, binding],
                            ),
                          );
                          const nextBindings = (template?.inputs ?? []).map(
                            (slot) =>
                              existing.get(slot.name) ?? {
                                to_input: slot.name,
                                source: "workflow_input" as const,
                                ref: slot.name,
                              },
                          );
                          onChange(
                            updateDraftStep(current, targetIndex, {
                              template_id,
                              template_version: template?.version,
                              slot_bindings: template?.inputs
                                ? nextBindings
                                : activeStep.slot_bindings,
                            }),
                          );
                        } catch (error) {
                          setTemplateError(
                            error instanceof Error
                              ? error.message
                              : t("studio.templateContractError"),
                          );
                        } finally {
                          setTemplatePending(false);
                        }
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue
                          placeholder={t("editorUI.chooseTemplate")}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {templateOptions
                          .filter(
                            (template) =>
                              template.status === "published" ||
                              (template.id === activeStep.template_id &&
                                template.version ===
                                  activeStep.template_version),
                          )
                          .map((template) => (
                            <SelectItem
                              key={`${template.id}@${template.version}`}
                              value={`${template.id}@${template.version}`}
                            >
                              {template.name} · v{template.version}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input
                      value={activeStep.template_id}
                      placeholder="Template ID"
                      onChange={(e) =>
                        updateStep(selectedStep, {
                          template_id: e.target.value,
                        })
                      }
                    />
                  )}
                </div>
                {templateError ? (
                  <p role="alert" className="text-xs text-destructive">
                    {templateError}
                  </p>
                ) : null}
                <details
                  key={activeStep.key}
                  className="space-y-3 border-t pt-4"
                >
                  <summary className="cursor-pointer text-sm font-semibold">
                    {t("studio.configureInputs", {
                      count: activeStep.slot_bindings.length,
                    })}
                  </summary>
                  <div className="flex items-center justify-between">
                    <div>
                      <Label>{t("editorUI.bindings")}</Label>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t("editorUI.bindingsHint")}
                      </p>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        updateStep(selectedStep, {
                          slot_bindings: [
                            ...activeStep.slot_bindings,
                            { source: "workflow_input", to_input: "", ref: "" },
                          ],
                        })
                      }
                    >
                      <Plus className="mr-1 h-3.5 w-3.5" />
                      {t("editorUI.bind")}
                    </Button>
                  </div>
                  {activeStep.slot_bindings.map((binding, bindingIndex) => (
                    <StepBindingEditor
                      key={bindingIndex}
                      binding={binding}
                      stepIndex={selectedStep}
                      bindingIndex={bindingIndex}
                      steps={value.steps}
                      declaredSlot={templateOptions
                        .find(
                          (item) =>
                            item.id === activeStep.template_id &&
                            item.version === activeStep.template_version,
                        )
                        ?.inputs?.find(
                          (slot) => slot.name === binding.to_input,
                        )}
                      onChange={(nextBinding) =>
                        updateStep(selectedStep, {
                          slot_bindings: activeStep.slot_bindings.map(
                            (item, index) =>
                              index === bindingIndex ? nextBinding : item,
                          ),
                        })
                      }
                      onRemove={() =>
                        updateStep(selectedStep, {
                          slot_bindings: activeStep.slot_bindings.filter(
                            (_, index) => index !== bindingIndex,
                          ),
                        })
                      }
                    />
                  ))}
                  {activeStep.slot_bindings.length === 0 ? (
                    <div className="rounded-xl border border-dashed p-4 text-center text-xs text-muted-foreground">
                      {t("editorUI.noBindings")}
                    </div>
                  ) : null}
                </details>
                <div className="grid gap-4 border-t pt-4">
                  <p className="text-sm font-semibold">
                    {t("editorUI.results")}
                  </p>
                  <div className="grid gap-2">
                    <Label>{t("editorUI.approval")}</Label>
                    <div
                      role="radiogroup"
                      aria-label={t("studio.approval")}
                      className="grid gap-2"
                    >
                      {(["when_multiple_candidates", "always"] as const).map(
                        (policy) => (
                          <button
                            type="button"
                            role="radio"
                            aria-checked={
                              (activeStep.approval_policy ??
                                "when_multiple_candidates") === policy
                            }
                            key={policy}
                            className="workflow-policy-option"
                            onClick={() =>
                              updateStep(selectedStep, {
                                approval_policy: policy,
                              })
                            }
                          >
                            {t(`studio.${policy}`)}
                          </button>
                        ),
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {t("editorUI.approvalHint")}
                    </p>
                  </div>
                  <div className="grid gap-2">
                    <Label>{t("editorUI.reuse")}</Label>
                    <div
                      role="radiogroup"
                      aria-label={t("studio.reuse")}
                      className="grid grid-cols-2 gap-2"
                    >
                      {(["reuse_if_available", "force_new"] as const).map(
                        (policy) => (
                          <button
                            type="button"
                            role="radio"
                            aria-checked={
                              (activeStep.reuse_policy ??
                                "reuse_if_available") === policy
                            }
                            key={policy}
                            className="workflow-policy-option"
                            onClick={() =>
                              updateStep(selectedStep, { reuse_policy: policy })
                            }
                          >
                            {t(`studio.${policy}`)}
                          </button>
                        ),
                      )}
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={value.output_step_key === activeStep.key}
                      onClick={() =>
                        onChange({ ...value, output_step_key: activeStep.key })
                      }
                    >
                      {t(
                        value.output_step_key === activeStep.key
                          ? "studio.finalOutput"
                          : "studio.setOutput",
                      )}
                    </Button>
                  </div>
                </div>
              </div>
            ) : null}
          </aside>
        ) : null}
      </section>

      {validationResult ? (
        <div
          className={cn(
            "rounded-xl border p-4 text-sm",
            validationResult.errors.length === 0
              ? "border-emerald-200 bg-emerald-50 text-emerald-800"
              : "border-destructive/30 bg-destructive/5 text-destructive",
          )}
        >
          {validationResult.errors.length === 0 ? (
            <span className="inline-flex items-center gap-2">
              <ShieldCheck className="h-4 w-4" />
              {t("editorUI.valid")}
            </span>
          ) : (
            <div className="space-y-2">
              <div className="inline-flex items-center gap-2 font-medium">
                <AlertCircle className="h-4 w-4" />
                {t("editorUI.issues")}
              </div>
              <ul className="list-disc space-y-1 pl-5">
                {validationResult.errors.map((error, index) => (
                  <li key={`${error.step_key}-${index}`}>
                    {error.step_key ? `[${error.step_key}] ` : ""}
                    {error.message}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      ) : null}
      <Dialog
        open={pickerOpen}
        onOpenChange={(open) => {
          if (!templatePending) setPickerOpen(open);
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("studio.chooseTemplate")}</DialogTitle>
            <DialogDescription>
              {t("studio.chooseTemplateHint")}
            </DialogDescription>
          </DialogHeader>
          <Input
            aria-label={t("studio.searchTemplates")}
            placeholder={t("studio.searchTemplates")}
            value={templateSearch}
            disabled={templatePending}
            onChange={(event) => setTemplateSearch(event.target.value)}
          />
          {templateError ? (
            <p role="alert" className="text-sm text-destructive">
              {templateError}
            </p>
          ) : null}
          <div className="grid max-h-[50vh] gap-3 overflow-y-auto sm:grid-cols-2">
            {templateOptions
              .filter(
                (option) =>
                  option.status === "published" &&
                  (option.name || option.id)
                    .toLowerCase()
                    .includes(templateSearch.toLowerCase()),
              )
              .map((option) => (
                <button
                  key={`${option.id}@${option.version}`}
                  type="button"
                  disabled={templatePending}
                  onClick={() => void selectTemplate(option)}
                  className="rounded-xl border bg-background p-4 text-left transition-colors hover:border-primary hover:bg-primary/5 disabled:opacity-50"
                >
                  <span className="block font-semibold">
                    {option.name || option.id}
                  </span>
                  <span className="mt-2 block text-xs text-muted-foreground">
                    {t("studio.publishedVersion", { version: option.version })}
                  </span>
                </button>
              ))}
          </div>
          {!templateOptions.some(
            (option) =>
              option.status === "published" &&
              (option.name || option.id)
                .toLowerCase()
                .includes(templateSearch.toLowerCase()),
          ) ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {t(
                templateSearch
                  ? "studio.noTemplates"
                  : "studio.publishTemplateFirst",
              )}
            </p>
          ) : null}
          {templatePending ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t("studio.loadingTemplate")}
            </p>
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog
        open={removeIndex !== null}
        onOpenChange={(open) => {
          if (!open) setRemoveIndex(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("editor.deleteNode")}</DialogTitle>
            <DialogDescription>{t("editor.deleteHint")}</DialogDescription>
          </DialogHeader>
          <p className="font-semibold">
            {removeIndex !== null ? value.steps[removeIndex]?.key : ""}
          </p>
          <ul className="space-y-1 text-sm">
            {removeIndex !== null
              ? removalImpact(value, value.steps[removeIndex]?.key ?? "").map(
                  (impact) => <li key={impact}>{impact}</li>,
                )
              : null}
          </ul>
          {removeIndex !== null &&
          value.output_step_key === value.steps[removeIndex]?.key ? (
            <p className="text-sm text-destructive">
              {t("editor.outputRemoved")}
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setRemoveIndex(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => removeIndex !== null && removeStep(removeIndex)}
            >
              {t("editor.deleteNode")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
