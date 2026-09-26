import {
  ArrowLeft,
  Boxes,
  CalendarClock,
  Eye,
  GitBranch,
  Pencil,
  Play,
  Send,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { WorkflowGraph } from "@/domains/workflows/components/workflow-graph";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import {
  useArchiveWorkflow,
  usePublishWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import { cn } from "@/foundation/lib/utils";

const statusLabel = {
  draft: "草稿",
  published: "已发布",
  archived: "已归档",
} as const;
const visibilityLabel = {
  private: "仅自己",
  team: "部门",
  public: "组织内公开",
} as const;

export function WorkflowShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: workflow, isLoading } = useWorkflow(id);
  const archiveWorkflow = useArchiveWorkflow();
  const publishWorkflow = usePublishWorkflow();

  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!workflow)
    return (
      <div className="p-8 text-sm text-muted-foreground">
        没有找到这个工作流。
      </div>
    );

  return (
    <div className="h-full overflow-y-auto bg-gradient-to-b from-slate-50/80 to-background dark:from-slate-950/80">
      <div className="mx-auto flex max-w-[1500px] flex-col gap-5 p-4 sm:p-6 lg:p-8">
        <header className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <Button
              variant="outline"
              size="icon"
              onClick={onBack}
              aria-label="返回"
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="min-w-0">
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <span
                  className={cn(
                    "rounded-full px-2.5 py-1 text-[11px] font-semibold",
                    workflow.status === "published"
                      ? "bg-emerald-100 text-emerald-700"
                      : workflow.status === "archived"
                        ? "bg-slate-200 text-slate-600"
                        : "bg-amber-100 text-amber-700",
                  )}
                >
                  {statusLabel[workflow.status]}
                </span>
                <span className="rounded-full bg-primary/10 px-2.5 py-1 text-[11px] font-semibold text-primary">
                  v{workflow.version}
                </span>
              </div>
              <h1 className="truncate text-2xl font-semibold tracking-tight sm:text-3xl">
                {workflow.name}
              </h1>
              <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                {workflow.description || "这个工作流还没有说明。"}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {workflow.status === "draft" ? (
              <Button
                onClick={() => publishWorkflow.mutate(id)}
                disabled={publishWorkflow.isPending}
              >
                <Send className="mr-2 h-4 w-4" />
                发布
              </Button>
            ) : null}
            <Button variant="outline" onClick={onEdit}>
              <Pencil className="mr-2 h-4 w-4" />
              编辑
            </Button>
          </div>
        </header>

        <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <InfoCard
            icon={GitBranch}
            label="生成节点"
            value={`${workflow.steps.length} 个`}
          />
          <InfoCard
            icon={Boxes}
            label="最终输出"
            value={workflow.output_step_key}
          />
          <InfoCard
            icon={Eye}
            label="可见范围"
            value={visibilityLabel[workflow.visibility]}
          />
          <InfoCard
            icon={CalendarClock}
            label="业务范围"
            value={workflow.scope}
          />
        </section>

        <section className="overflow-hidden rounded-2xl border border-border/70 bg-card shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-4">
            <div>
              <h2 className="font-semibold">流程拓扑</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                由 Slot Binding 推导依赖，画布不是第二份业务事实。
              </p>
            </div>
            <div className="inline-flex items-center gap-2 rounded-full bg-muted px-3 py-1.5 text-xs text-muted-foreground">
              <Play className="h-3.5 w-3.5" />
              FlowGram
            </div>
          </div>
          <WorkflowGraph
            model={{
              steps: workflow.steps,
              outputStepKey: workflow.output_step_key,
            }}
            className="h-[540px]"
          />
        </section>

        <section className="grid gap-3 lg:grid-cols-2 xl:grid-cols-3">
          {workflow.steps.map((step, index) => (
            <article
              key={step.key}
              className="rounded-2xl border border-border/70 bg-card p-4 shadow-sm"
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <span className="text-[10px] font-semibold uppercase tracking-[0.18em] text-primary">
                    Step {index + 1}
                  </span>
                  <h3 className="mt-1 font-semibold">{step.key}</h3>
                </div>
                {step.key === workflow.output_step_key ? (
                  <span className="rounded-full bg-primary/10 px-2 py-1 text-[10px] font-semibold text-primary">
                    最终输出
                  </span>
                ) : null}
              </div>
              <p className="mt-3 truncate text-xs text-muted-foreground">
                Template · {step.template_id} · v
                {step.template_version ?? "latest"}
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                {step.slot_bindings.map((binding) => (
                  <span
                    key={`${binding.to_input}-${binding.ref}`}
                    className="rounded-lg border bg-muted/40 px-2.5 py-1.5 text-[11px]"
                  >
                    <strong>{binding.to_input}</strong>
                    <span className="mx-1.5 text-muted-foreground">←</span>
                    <span className="text-muted-foreground">
                      {binding.source === "const"
                        ? String(binding.const_value ?? "固定值")
                        : binding.ref}
                    </span>
                  </span>
                ))}
              </div>
            </article>
          ))}
        </section>

        {workflow.status !== "archived" ? (
          <section className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-destructive/20 bg-destructive/[0.03] p-5">
            <div>
              <h2 className="font-medium">归档工作流</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                归档后不能再用于创建新的任务，已有执行记录会保留。
              </p>
            </div>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                archiveWorkflow.mutate(id);
                onBack();
              }}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              归档
            </Button>
          </section>
        ) : null}
      </div>
    </div>
  );
}

function InfoCard({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof GitBranch;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-2xl border border-border/70 bg-card/90 p-4 shadow-sm">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="rounded-lg bg-primary/10 p-2 text-primary">
          <Icon className="h-4 w-4" />
        </span>
        {label}
      </div>
      <div className="mt-3 truncate font-semibold">{value}</div>
    </div>
  );
}
