import {
  EditorRenderer,
  FreeLayoutEditorProvider,
  type FreeLayoutProps,
  type FlowNodeMeta,
  type WorkflowJSON,
  type WorkflowNodeProps,
  WorkflowNodeRenderer,
  useNodeRender,
} from "@flowgram.ai/free-layout-editor";
import "@flowgram.ai/free-layout-editor/index.css";
import {
  CheckCircle2,
  GitBranch,
  Layers3,
  Play,
  ShieldCheck,
} from "lucide-react";
import { createContext, useContext, useMemo } from "react";
import { cn } from "@/foundation/lib/utils";
import type { WorkflowStep } from "@/domains/workflows/types";

type GraphNodeData = {
  title: string;
  subtitle: string;
  bindingCount: number;
  approvalPolicy?: WorkflowStep["approval_policy"];
  output?: boolean;
  input?: boolean;
};

type WorkflowGraphContextValue = {
  onNodeSelect?: (nodeId: string) => void;
  selectedNodeId?: string;
};

const WorkflowGraphContext = createContext<WorkflowGraphContextValue>({});
const WorkflowNodeIdContext = createContext<string | undefined>(undefined);

export type WorkflowGraphModel = {
  steps: WorkflowStep[];
  outputStepKey: string;
};

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
}: WorkflowGraphModel): WorkflowJSON {
  const levels = dependencyLevels(steps);
  const rows = new Map<number, number>();
  const nodes: WorkflowJSON["nodes"] = [
    {
      id: "__workflow_input__",
      type: "workflow_input",
      meta: { position: { x: 64, y: 96 } },
      data: {
        title: "工作流输入",
        subtitle: "运行时资源",
        bindingCount: steps.reduce(
          (count, step) =>
            count +
            step.slot_bindings.filter(
              (binding) => binding.source === "workflow_input",
            ).length,
          0,
        ),
        input: true,
      } satisfies GraphNodeData,
    },
  ];

  for (const step of steps) {
    const level = levels.get(step.key) ?? 1;
    const row = rows.get(level) ?? 0;
    rows.set(level, row + 1);
    nodes.push({
      id: step.key,
      type: "generation_step",
      meta: { position: { x: 64 + level * 320, y: 56 + row * 184 } },
      data: {
        title: step.key || "未命名节点",
        subtitle: `Template · ${step.template_id || "未选择"}`,
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
      const source =
        binding.source === "step_output"
          ? binding.ref
          : binding.source === "workflow_input"
            ? "__workflow_input__"
            : undefined;
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
    type: "workflow_input",
    meta: { defaultExpanded: true },
    defaultPorts: [{ type: "output" }],
    formMeta: {
      render: ({ form }) => (
        <WorkflowNodeCard data={form.values as GraphNodeData} />
      ),
    },
  },
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
  const { onNodeSelect, selectedNodeId } = useContext(WorkflowGraphContext);
  const renderedNodeId = useContext(WorkflowNodeIdContext);
  const Icon = data.input ? Play : data.output ? CheckCircle2 : Layers3;
  return (
    <button
      type="button"
      className={cn(
        "workflow-node-card",
        data.output && "is-output",
        selectedNodeId === renderedNodeId && "is-selected",
      )}
      onClick={() => renderedNodeId && onNodeSelect?.(renderedNodeId)}
      aria-label={`选择节点 ${data.title}`}
    >
      <div className="workflow-node-icon">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <strong className="truncate text-[13px]">{data.title}</strong>
          {data.output ? (
            <span className="workflow-node-pill">输出</span>
          ) : null}
        </div>
        <p className="mt-1 truncate text-[11px] text-slate-500">
          {data.subtitle}
        </p>
        <div className="mt-3 flex items-center gap-3 text-[10px] text-slate-500">
          <span className="inline-flex items-center gap-1">
            <GitBranch className="h-3 w-3" /> {data.bindingCount} 个绑定
          </span>
          {!data.input && data.approvalPolicy === "always" ? (
            <span className="inline-flex items-center gap-1 text-amber-700">
              <ShieldCheck className="h-3 w-3" /> 始终审批
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
}: {
  model: WorkflowGraphModel;
  className?: string;
  onNodeSelect?: (nodeId: string) => void;
  selectedNodeId?: string;
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
    <WorkflowGraphContext.Provider value={{ onNodeSelect, selectedNodeId }}>
      <div className={cn("workflow-graph", className)}>
        <FreeLayoutEditorProvider key={documentKey} {...props}>
          <EditorRenderer className="h-full w-full" />
        </FreeLayoutEditorProvider>
      </div>
    </WorkflowGraphContext.Provider>
  );
}
