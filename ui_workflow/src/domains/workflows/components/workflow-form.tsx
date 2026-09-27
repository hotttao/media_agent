import {
  AlertCircle,
  ArrowRight,
  Plus,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { useMemo, useState } from "react";
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

export type WorkflowTemplateOption = {
  id: string;
  name: string;
  version: number;
  status: string;
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
  onChange,
  onRemove,
}: {
  binding: SlotBinding;
  stepIndex: number;
  bindingIndex: number;
  steps: WorkflowStep[];
  onChange: (binding: SlotBinding) => void;
  onRemove: () => void;
}) {
  const previousSteps = steps.slice(0, stepIndex).filter((step) => step.key);
  return (
    <div className="grid gap-2 rounded-xl border border-border/70 bg-background/70 p-3">
      <div className="grid grid-cols-[1fr_auto] gap-2">
        <div className="grid gap-1.5">
          <Label
            htmlFor={`binding-slot-${stepIndex}-${bindingIndex}`}
            className="text-[11px] text-muted-foreground"
          >
            Template 输入
          </Label>
          <Input
            id={`binding-slot-${stepIndex}-${bindingIndex}`}
            value={binding.to_input}
            placeholder="例如 first_frame"
            onChange={(event) =>
              onChange({ ...binding, to_input: event.target.value })
            }
          />
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="self-end text-muted-foreground hover:text-destructive"
          aria-label="删除绑定"
          onClick={onRemove}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label className="text-[11px] text-muted-foreground">数据来源</Label>
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
              <SelectItem value="workflow_input">工作流输入</SelectItem>
              <SelectItem value="step_output">上游节点输出</SelectItem>
              <SelectItem value="const">固定值</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-1.5">
          <Label className="text-[11px] text-muted-foreground">
            {binding.source === "const" ? "固定值" : "引用名称"}
          </Label>
          {binding.source === "step_output" && previousSteps.length > 0 ? (
            <Select
              value={binding.ref ?? ""}
              onValueChange={(ref) => onChange({ ...binding, ref })}
            >
              <SelectTrigger>
                <SelectValue placeholder="选择上游节点" />
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
              value={
                binding.source === "const"
                  ? String(binding.const_value ?? "")
                  : (binding.ref ?? "")
              }
              placeholder={
                binding.source === "const" ? "输入固定值" : "例如 product"
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
  onCancel,
  onValidate,
  validationResult,
  templateOptions = [],
}: {
  value: WorkflowInput;
  onChange: (v: WorkflowInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  onValidate?: () => void;
  validationResult?: ValidationResult | null;
  templateOptions?: WorkflowTemplateOption[];
}) {
  const [selectedStep, setSelectedStep] = useState(0);
  const graphModel = useMemo(
    () => ({ steps: value.steps, outputStepKey: value.output_step_key }),
    [value.output_step_key, value.steps],
  );

  function updateStep(index: number, patch: Partial<WorkflowStep>) {
    const steps = value.steps.map((step, stepIndex) =>
      stepIndex === index ? { ...step, ...patch } : step,
    );
    const previousKey = value.steps[index]?.key;
    const nextKey = steps[index]?.key;
    onChange({
      ...value,
      steps,
      output_step_key:
        previousKey && previousKey === value.output_step_key && nextKey
          ? nextKey
          : value.output_step_key,
    });
  }

  function addStep() {
    const key = nextStepKey(value.steps);
    const template =
      templateOptions.find((item) => item.status === "published") ??
      templateOptions[0];
    const step: WorkflowStep = {
      key,
      template_id: template?.id ?? "",
      template_version: template?.version,
      slot_bindings: [],
      approval_policy: "when_multiple_candidates",
      reuse_policy: "reuse_if_available",
    };
    const steps = [...value.steps, step];
    onChange({
      ...value,
      steps,
      output_step_key: value.output_step_key || key,
    });
    setSelectedStep(steps.length - 1);
  }

  function removeStep(index: number) {
    const removed = value.steps[index];
    const steps = value.steps
      .filter((_, stepIndex) => stepIndex !== index)
      .map((step) => ({
        ...step,
        slot_bindings: step.slot_bindings.filter(
          (binding) =>
            binding.source !== "step_output" || binding.ref !== removed?.key,
        ),
      }));
    onChange({
      ...value,
      steps,
      output_step_key:
        value.output_step_key === removed?.key
          ? (steps.at(-1)?.key ?? "")
          : value.output_step_key,
    });
    setSelectedStep(Math.max(0, Math.min(index, steps.length - 1)));
  }

  const activeStep = value.steps[selectedStep];
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-5">
      <section className="grid gap-4 rounded-2xl border border-border/70 bg-card/80 p-5 shadow-sm lg:grid-cols-2">
        <div className="grid gap-2">
          <Label>名称</Label>
          <Input
            value={value.name}
            placeholder="例如：商品短视频生成"
            onChange={(e) => onChange({ ...value, name: e.target.value })}
          />
        </div>
        <div className="grid gap-2">
          <Label>业务范围</Label>
          <Input
            value={value.scope}
            placeholder="例如：media-production"
            onChange={(e) => onChange({ ...value, scope: e.target.value })}
          />
        </div>
        <div className="grid gap-2 lg:col-span-2">
          <Label>说明</Label>
          <Textarea
            rows={2}
            value={value.description ?? ""}
            placeholder="说明这个流程解决什么业务问题"
            onChange={(e) =>
              onChange({ ...value, description: e.target.value })
            }
          />
        </div>
        <div className="grid gap-2">
          <Label>最终输出节点</Label>
          <Select
            value={value.output_step_key}
            onValueChange={(output_step_key) =>
              onChange({ ...value, output_step_key })
            }
          >
            <SelectTrigger>
              <SelectValue placeholder="选择最终节点" />
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
          <Label>可见范围</Label>
          <Select
            value={value.visibility ?? "private"}
            onValueChange={(visibility) =>
              onChange({ ...value, visibility: visibility as Visibility })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="private">仅自己</SelectItem>
              <SelectItem value="team">部门</SelectItem>
              <SelectItem value="public">组织内公开</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </section>

      <section className="grid min-h-[560px] flex-1 overflow-hidden rounded-2xl border border-border/70 bg-card shadow-sm xl:grid-cols-[minmax(0,1fr)_390px]">
        <div className="relative min-h-[420px] overflow-hidden border-b border-border/70 bg-slate-50/70 dark:bg-slate-950/70 xl:border-b-0 xl:border-r">
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
              <h3 className="mt-4 font-semibold">从第一个生成节点开始</h3>
              <p className="mt-1 max-w-sm text-sm text-muted-foreground">
                添加节点、选择 Template，并将输入绑定到工作流参数或上游输出。
              </p>
              <Button type="button" className="mt-5" onClick={addStep}>
                <Plus className="mr-2 h-4 w-4" />
                添加节点
              </Button>
            </div>
          )}
          {value.steps.length > 0 ? (
            <div className="absolute left-4 top-4 rounded-full border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground shadow-sm backdrop-blur">
              FlowGram 画布 · {value.steps.length} 个节点
            </div>
          ) : null}
        </div>

        <aside className="flex min-h-0 flex-col bg-background/80">
          <div className="flex items-center justify-between border-b px-4 py-3">
            <div>
              <h3 className="font-semibold">节点配置</h3>
              <p className="text-xs text-muted-foreground">
                业务 DSL 与画布保持同步
              </p>
            </div>
            <Button type="button" size="sm" variant="outline" onClick={addStep}>
              <Plus className="mr-1.5 h-3.5 w-3.5" />
              节点
            </Button>
          </div>
          {value.steps.length > 0 ? (
            <p className="border-b px-4 py-3 text-xs text-muted-foreground">
              点击画布节点查看和编辑配置；拖动节点可调整画布布局。
            </p>
          ) : null}
          {activeStep ? (
            <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-xs font-medium uppercase tracking-[0.16em] text-primary">
                    Step {selectedStep + 1}
                  </div>
                  <h4 className="mt-1 font-semibold">生成节点</h4>
                </div>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => removeStep(selectedStep)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="workflow-step-key">节点标识</Label>
                <Input
                  id="workflow-step-key"
                  value={activeStep.key}
                  onChange={(e) =>
                    updateStep(selectedStep, { key: e.target.value })
                  }
                />
              </div>
              <div className="grid gap-2">
                <Label>Template</Label>
                {templateOptions.length > 0 ? (
                  <Select
                    value={activeStep.template_id}
                    onValueChange={(template_id) => {
                      const template = templateOptions.find(
                        (item) => item.id === template_id,
                      );
                      updateStep(selectedStep, {
                        template_id,
                        template_version: template?.version,
                      });
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="选择 Template" />
                    </SelectTrigger>
                    <SelectContent>
                      {templateOptions.map((template) => (
                        <SelectItem key={template.id} value={template.id}>
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
                      updateStep(selectedStep, { template_id: e.target.value })
                    }
                  />
                )}
              </div>
              <div className="grid gap-2">
                <Label>审批策略</Label>
                <Select
                  value={
                    activeStep.approval_policy ?? "when_multiple_candidates"
                  }
                  onValueChange={(approval_policy) =>
                    updateStep(selectedStep, {
                      approval_policy:
                        approval_policy as WorkflowStep["approval_policy"],
                    })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="when_multiple_candidates">
                      多候选时审批
                    </SelectItem>
                    <SelectItem value="always">始终审批</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label>复用策略</Label>
                <Select
                  value={activeStep.reuse_policy ?? "reuse_if_available"}
                  onValueChange={(reuse_policy) =>
                    updateStep(selectedStep, {
                      reuse_policy:
                        reuse_policy as WorkflowStep["reuse_policy"],
                    })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="reuse_if_available">
                      优先复用已有结果
                    </SelectItem>
                    <SelectItem value="force_new">始终重新生成</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <div>
                    <Label>输入绑定</Label>
                    <p className="mt-1 text-xs text-muted-foreground">
                      连接工作流输入、上游输出或固定值
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
                    绑定
                  </Button>
                </div>
                {activeStep.slot_bindings.map((binding, bindingIndex) => (
                  <StepBindingEditor
                    key={bindingIndex}
                    binding={binding}
                    stepIndex={selectedStep}
                    bindingIndex={bindingIndex}
                    steps={value.steps}
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
                    该节点还没有输入绑定
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}
        </aside>
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
              结构校验通过，可以保存或发布。
            </span>
          ) : (
            <div className="space-y-2">
              <div className="inline-flex items-center gap-2 font-medium">
                <AlertCircle className="h-4 w-4" />
                请修正以下问题
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
      <div className="sticky bottom-0 flex flex-wrap items-center justify-end gap-2 rounded-2xl border bg-background/90 p-3 shadow-lg backdrop-blur">
        <Button type="button" variant="ghost" onClick={onCancel}>
          取消
        </Button>
        {onValidate ? (
          <Button
            type="button"
            variant="outline"
            onClick={onValidate}
            disabled={isPending}
          >
            校验结构
          </Button>
        ) : null}
        <Button
          type="button"
          onClick={onSubmit}
          disabled={isPending || !value.name || value.steps.length === 0}
        >
          {submitLabel}
        </Button>
      </div>
    </div>
  );
}
