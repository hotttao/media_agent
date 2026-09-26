import { Archive, ArrowUpRight, GitBranch, Plus, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useArchiveWorkflow } from "@/domains/workflows/hooks/use-workflow-mutations";
import { useWorkflowsList } from "@/domains/workflows/hooks/use-workflows-list";
import { cn } from "@/foundation/lib/utils";

const statusLabel = {
  draft: "草稿",
  published: "已发布",
  archived: "已归档",
} as const;

export function WorkflowList({
  onSelect,
  onCreate,
}: {
  onSelect: (id: string) => void;
  onCreate?: () => void;
}) {
  const { data, isLoading, isError, error } = useWorkflowsList();
  const archiveWorkflow = useArchiveWorkflow();
  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (isError)
    return (
      <div className="flex flex-1 items-center justify-center p-8 text-sm text-destructive">
        无法加载工作流：{error instanceof Error ? error.message : "请求失败"}
      </div>
    );

  const items = data?.items ?? [];
  return (
    <div className="h-full overflow-y-auto bg-gradient-to-b from-slate-50/80 to-background dark:from-slate-950/80">
      <div className="mx-auto max-w-[1500px] p-4 sm:p-6 lg:p-8">
        <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
          <div>
            <div className="mb-2 inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary">
              <Sparkles className="h-3.5 w-3.5" />
              Workflow Studio
            </div>
            <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
              工作流
            </h1>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              把素材、Template、审批与生成能力组合成可复用的媒体生产流程。
            </p>
          </div>
          {onCreate ? (
            <Button onClick={onCreate}>
              <Plus className="mr-2 h-4 w-4" />
              新建工作流
            </Button>
          ) : null}
        </div>
        {items.length === 0 ? (
          <div className="flex min-h-[420px] flex-col items-center justify-center rounded-3xl border border-dashed bg-card/70 p-8 text-center">
            <div className="rounded-2xl bg-primary/10 p-4 text-primary">
              <GitBranch className="h-7 w-7" />
            </div>
            <h2 className="mt-5 text-lg font-semibold">还没有工作流</h2>
            <p className="mt-2 max-w-sm text-sm text-muted-foreground">
              创建第一个流程，通过可视化节点连接生成步骤。
            </p>
            {onCreate ? (
              <Button className="mt-5" onClick={onCreate}>
                开始创建
              </Button>
            ) : null}
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {items.map((workflow) => (
              <article
                key={workflow.id}
                className="group relative overflow-hidden rounded-2xl border border-border/70 bg-card p-5 shadow-sm transition duration-200 hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-lg"
              >
                <div className="absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-primary/80 via-violet-400/70 to-cyan-400/70 opacity-0 transition group-hover:opacity-100" />
                <div className="flex items-start justify-between gap-3">
                  <div className="rounded-xl bg-primary/10 p-2.5 text-primary">
                    <GitBranch className="h-5 w-5" />
                  </div>
                  <span
                    className={cn(
                      "rounded-full px-2.5 py-1 text-[10px] font-semibold",
                      workflow.status === "published"
                        ? "bg-emerald-100 text-emerald-700"
                        : workflow.status === "archived"
                          ? "bg-slate-100 text-slate-600"
                          : "bg-amber-100 text-amber-700",
                    )}
                  >
                    {statusLabel[workflow.status]}
                  </span>
                </div>
                <button
                  type="button"
                  className="mt-5 block w-full text-left"
                  onClick={() => onSelect(workflow.id)}
                >
                  <h2 className="line-clamp-1 text-lg font-semibold group-hover:text-primary">
                    {workflow.name}
                  </h2>
                  <p className="mt-2 line-clamp-2 min-h-10 text-sm leading-5 text-muted-foreground">
                    {workflow.description || "暂无说明"}
                  </p>
                </button>
                <div className="mt-5 grid grid-cols-3 gap-2 rounded-xl bg-muted/45 p-3 text-center">
                  <Metric label="节点" value={String(workflow.steps.length)} />
                  <Metric label="版本" value={`v${workflow.version}`} />
                  <Metric
                    label="输出"
                    value={workflow.output_step_key || "—"}
                  />
                </div>
                <div className="mt-4 flex items-center justify-between">
                  <span className="truncate text-xs text-muted-foreground">
                    {workflow.scope}
                  </span>
                  <div className="flex items-center gap-1">
                    {workflow.status !== "archived" ? (
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label="归档"
                        onClick={() => archiveWorkflow.mutate(workflow.id)}
                      >
                        <Archive className="h-4 w-4" />
                      </Button>
                    ) : null}
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="查看"
                      onClick={() => onSelect(workflow.id)}
                    >
                      <ArrowUpRight className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </article>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="truncate text-sm font-semibold">{value}</div>
      <div className="mt-0.5 text-[10px] text-muted-foreground">{label}</div>
    </div>
  );
}
