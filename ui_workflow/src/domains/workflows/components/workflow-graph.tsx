import {
  EditorRenderer,
  type FlowNodeMeta,
  FreeLayoutEditorProvider,
  type FreeLayoutProps,
  useNodeRender,
  type WorkflowJSON,
  type WorkflowNodeProps,
  WorkflowNodeRenderer,
  useClientContext,
} from "@flowgram.ai/free-layout-editor";
import { CheckCircle2, GitBranch, Layers3, ShieldCheck } from "lucide-react";
import { createContext, useContext, useMemo, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { WorkflowStep } from "@/domains/workflows/types";
import { cn } from "@/foundation/lib/utils";

type GraphNodeData = {
  title: string;
  subtitle: string;
  hasTemplate: boolean;
  bindingCount: number;
  approvalPolicy?: WorkflowStep["approval_policy"];
  output?: boolean;
  executionStatus?: string;
};

type WorkflowGraphContextValue = {
  onNodeSelect?: (nodeId: string) => void;
  selectedNodeId?: string;
  executionStatus?: WorkflowExecutionStatus;
};

const WorkflowGraphContext = createContext<WorkflowGraphContextValue>({});
const WorkflowNodeIdContext = createContext<string | undefined>(undefined);

function ViewportTools() {
  const context = useClientContext();
  const { t } = useTranslation();
  useEffect(() => {
    const subscription = context.playground.onResize(() => {
      void context.document.fitView(false);
    });
    return () => subscription.dispose();
  }, [context]);
  return (
    <div className="absolute right-3 top-3 z-10">
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => void context.document.fitView(false)}
      >
        {t("editorViewport.fit")}
      </Button>
    </div>
  );
}

export type WorkflowGraphModel = {
  steps: WorkflowStep[];
  outputStepKey: string;
  templateNameByID?: Record<string, string>;
  layout?: "studio" | "horizontal";
};

export type WorkflowExecutionStatus = Record<string, string>;

function dependencyLevels(steps: WorkflowStep[]) {
  const levels = new Map<string, number>();
  for (const step of steps) {
    const predecessors = step.slot_bindings
      .filter((binding) => binding.source === "step_output" && binding.ref)
      .map((binding) => levels.get(binding.ref ?? "") ?? 0);
    levels.set(
      step.key,
      Math.max(1, ...predecessors.map((level) => level + 1)),
    );
  }
  return levels;
}

export function workflowToFlowDocument({
  steps,
  outputStepKey,
  templateNameByID,
  layout,
}: WorkflowGraphModel): WorkflowJSON {
  const levels = dependencyLevels(steps);
  const rows = new Map<number, number>();
  const nodes: WorkflowJSON["nodes"] = [];
  // A compact zig-zag for a linear generation flow; branched DAGs keep rank layout.
  // This is presentation state, never an additional DSL edge or input node.
  const compact =
    layout === "studio" &&
    steps.every((step, index) => levels.get(step.key) === index + 1);

  for (const step of steps) {
    const level = levels.get(step.key) ?? 1;
    const row = rows.get(level) ?? 0;
    rows.set(level, row + 1);
    nodes.push({
      id: step.key,
      type: "generation_step",
      meta: {
        position: compact
          ? { x: 64 + ((level - 1) % 2) * 320, y: 56 + (level - 1) * 160 }
          : { x: 64 + (level - 1) * 320, y: 56 + row * 184 },
      },
      data: {
        title: step.key,
        subtitle: templateNameByID?.[step.template_id] ?? "",
        hasTemplate: Boolean(step.template_id),
        bindingCount: step.slot_bindings.length,
        approvalPolicy: step.approval_policy,
        output: step.key === outputStepKey,
      } satisfies GraphNodeData,
    });
  }

  const edgeKeys = new Set<string>();
  const edges: WorkflowJSON["edges"] = [];
  for (const step of steps) {
    for (const binding of step.slot_bindings) {
      const source = binding.source === "step_output" ? binding.ref : undefined;
      if (!source) continue;
      const key = `${source}:${step.key}`;
      if (edgeKeys.has(key)) continue;
      edgeKeys.add(key);
      edges.push({ sourceNodeID: source, targetNodeID: step.key });
    }
  }
  return { nodes, edges };
}

const nodeRegistries: FreeLayoutProps["nodeRegistries"] = [
  {
    type: "generation_step",
    meta: { defaultExpanded: true },
    defaultPorts: [{ type: "input" }, { type: "output" }],
    formMeta: {
      render: ({ form }) => (
        <WorkflowNodeCard data={form.values as GraphNodeData} />
      ),
    },
  },
];

function WorkflowNodeCard({ data }: { data: GraphNodeData }) {
  const { t } = useTranslation();
  const title = data.title || t("graphUI.unnamed");
  const { onNodeSelect, selectedNodeId, executionStatus } =
    useContext(WorkflowGraphContext);
  const renderedNodeId = useContext(WorkflowNodeIdContext);
  const Icon = data.output ? CheckCircle2 : Layers3;
  const status = renderedNodeId ? executionStatus?.[renderedNodeId] : undefined;
  return (
    <button
      type="button"
      className={cn(
        "workflow-node-card",
        data.output && "is-output",
        status === "running" || status === "claimed" ? "is-running" : undefined,
        status === "succeeded" ? "is-succeeded" : undefined,
        status === "need_confirm" ? "is-approval" : undefined,
        selectedNodeId === renderedNodeId && "is-selected",
      )}
      onClick={() => renderedNodeId && onNodeSelect?.(renderedNodeId)}
      aria-label={t("graphUI.select", { name: title })}
    >
      <div className="workflow-node-icon">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <strong className="truncate text-[13px]">{title}</strong>
          {data.output ? (
            <span className="workflow-node-pill">{t("graphUI.output")}</span>
          ) : null}
          {status ? (
            <span className="workflow-node-pill">
              {t(`execution.stepStatus.${status}`, { defaultValue: status })}
            </span>
          ) : null}
        </div>
        <p className="mt-1 truncate text-[11px] text-slate-500">
          {data.subtitle ||
            t(data.hasTemplate ? "graphUI.generation" : "graphUI.noTemplate")}
        </p>
        <div className="mt-3 flex items-center gap-3 text-[10px] text-slate-500">
          <span className="inline-flex items-center gap-1">
            <GitBranch className="h-3 w-3" />{" "}
            {t("graphUI.bindings", { count: data.bindingCount })}
          </span>
          {data.approvalPolicy === "always" ? (
            <span className="inline-flex items-center gap-1 text-amber-700">
              <ShieldCheck className="h-3 w-3" /> {t("graphUI.alwaysReview")}
            </span>
          ) : null}
        </div>
      </div>
    </button>
  );
}

function WorkflowGraphNode(props: WorkflowNodeProps) {
  const { node, form } = useNodeRender();
  const meta = node.getNodeMeta<FlowNodeMeta>();
  return (
    <WorkflowNodeRenderer
      node={props.node}
      style={meta.wrapperStyle}
      className="workflow-graph-node"
    >
      <WorkflowNodeIdContext.Provider value={node.id}>
        {form?.render()}
      </WorkflowNodeIdContext.Provider>
    </WorkflowNodeRenderer>
  );
}

export function WorkflowGraph({
  model,
  className,
  onNodeSelect,
  selectedNodeId,
  executionStatus,
}: {
  model: WorkflowGraphModel;
  className?: string;
  onNodeSelect?: (nodeId: string) => void;
  selectedNodeId?: string;
  executionStatus?: WorkflowExecutionStatus;
}) {
  const documentKey = JSON.stringify(model);
  const document = useMemo(
    () => workflowToFlowDocument(JSON.parse(documentKey) as WorkflowGraphModel),
    [documentKey],
  );
  const props = useMemo<FreeLayoutProps>(
    () => ({
      background: false,
      readonly: true,
      enableReadonlyNodeDragging: true,
      initialData: document,
      nodeRegistries,
      fromNodeJSON: (_node, json) => json,
      toNodeJSON: (_node, json) => json,
      getNodeDefaultRegistry: (type) => ({
        type,
        meta: { defaultExpanded: true },
        formMeta: { render: () => null },
      }),
      materials: { renderDefaultNode: WorkflowGraphNode },
      nodeEngine: { enable: true },
      history: { enable: false },
      playground: { preventGlobalGesture: true },
      scroll: { enableScrollLimit: false },
      onAllLayersRendered(ctx) {
        ctx.document.fitView(false);
      },
    }),
    [document],
  );

  return (
    <WorkflowGraphContext.Provider
      value={{ onNodeSelect, selectedNodeId, executionStatus }}
    >
      <div
        data-testid="workflow-canvas"
        className={cn("workflow-graph", className)}
      >
        <FreeLayoutEditorProvider key={documentKey} {...props}>
          <EditorRenderer className="h-full w-full" />
          <ViewportTools />
        </FreeLayoutEditorProvider>
      </div>
    </WorkflowGraphContext.Provider>
  );
}
