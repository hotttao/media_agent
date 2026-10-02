import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { useSubmitJob } from "@/domains/jobs/hooks/use-job-mutations";
import { useWorkflowsList } from "@/domains/jobs/hooks/use-workflows-list";
import type { WorkflowInputSchemaEntry } from "@/domains/jobs/types";
import { hostShim } from "@/foundation/lib/host-shim";
import { resolveLibraryObject } from "@/foundation/lib/library-object";
import { buildJobInput, missingRequiredInputs } from "./job-input";
import { useJobInputOptions } from "./use-job-input-options";

const RESOURCE_LABELS: Record<string, string> = {
  PRODUCT: "商品",
  VIRTUAL_IP: "虚拟 IP",
  MATERIAL: "素材",
  ASSET: "生成资产",
};

function inputLabel(entry: WorkflowInputSchemaEntry) {
  const resource = RESOURCE_LABELS[entry.resource_type] ?? entry.resource_type;
  return `${entry.name} · ${entry.asset_kind || resource}`;
}

export function JobCreate({
  onDone,
  onSubmitted,
  preferredWorkflowId,
}: {
  onDone: () => void;
  onSubmitted: (jobId: string) => void;
  preferredWorkflowId?: string;
}) {
  const shell = hostShim.useShellContext();
  const submitJob = useSubmitJob();
  const { data: workflows, isLoading: workflowsLoading } = useWorkflowsList();
  const [workflowId, setWorkflowId] = useState(preferredWorkflowId ?? "");
  const [selectedIds, setSelectedIds] = useState<Record<string, string>>({});
  const selectedWorkflow = workflows?.find((item) => item.id === workflowId);
  const optionsQuery = useJobInputOptions(
    selectedWorkflow,
    !shell.isLoading && Boolean(shell.organization.id),
  );
  const missing = missingRequiredInputs(selectedWorkflow, selectedIds);

  async function handleSubmit() {
    if (!selectedWorkflow || missing.length > 0) return;
    const input = await buildJobInput(
      selectedWorkflow,
      selectedIds,
      optionsQuery.data ?? {},
      (path) => resolveLibraryObject(shell.organization.id, path),
    );
    submitJob.mutate(
      {
        workflow_id: selectedWorkflow.id,
        workflow_version: selectedWorkflow.version,
        input,
      },
      { onSuccess: (job) => onSubmitted(job.id) },
    );
  }

  return (
    <div className="flex flex-col gap-5 p-4">
      <div>
        <h2 className="text-lg font-semibold">创建生成任务</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          选择工作流后，系统会按它的输入契约加载可用素材。
        </p>
      </div>
      <div className="grid gap-2">
        <Label>工作流</Label>
        {workflowsLoading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="h-4 w-4" /> 正在加载工作流…
          </div>
        ) : (
          <Select
            value={workflowId}
            onValueChange={(value) => {
              setWorkflowId(value);
              setSelectedIds({});
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择已发布的工作流" />
            </SelectTrigger>
            <SelectContent>
              {(workflows ?? []).map((workflow) => (
                <SelectItem key={workflow.id} value={workflow.id}>
                  {workflow.name}（v{workflow.version}）
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {selectedWorkflow ? (
        <div className="grid gap-4 rounded-lg border bg-muted/20 p-4 md:grid-cols-2">
          {optionsQuery.isLoading || shell.isLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground md:col-span-2">
              <Spinner className="h-4 w-4" /> 正在加载商品与素材…
            </div>
          ) : null}
          {optionsQuery.isError ? (
            <p className="text-sm text-destructive md:col-span-2">
              无法加载工作流所需素材：{optionsQuery.error.message}
            </p>
          ) : null}
          {!optionsQuery.isLoading &&
            !optionsQuery.isError &&
            (selectedWorkflow.input_schema?.entries ?? []).map((entry) => {
              const options = optionsQuery.data?.[entry.name] ?? [];
              return (
                <div className="grid gap-2" key={entry.name}>
                  <Label>
                    {inputLabel(entry)}
                    {entry.required ? (
                      <span className="ml-1 text-destructive">*</span>
                    ) : null}
                  </Label>
                  <Select
                    value={selectedIds[entry.name] ?? ""}
                    onValueChange={(value) =>
                      setSelectedIds((current) => ({
                        ...current,
                        [entry.name]: value,
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={`选择${inputLabel(entry)}`} />
                    </SelectTrigger>
                    <SelectContent>
                      {options.map((option) => (
                        <SelectItem key={option.id} value={option.id}>
                          {option.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {options.length === 0 ? (
                    <p className="text-xs text-muted-foreground">
                      当前组织内没有可用的{inputLabel(entry)}。
                    </p>
                  ) : null}
                </div>
              );
            })}
        </div>
      ) : null}

      {submitJob.isError ? (
        <p className="text-sm text-destructive">
          提交失败：{submitJob.error.message}
        </p>
      ) : null}
      <div className="flex gap-2">
        <Button
          onClick={() => void handleSubmit()}
          disabled={
            submitJob.isPending ||
            optionsQuery.isLoading ||
            optionsQuery.isError ||
            !selectedWorkflow ||
            missing.length > 0
          }
        >
          {submitJob.isPending ? <Spinner className="h-4 w-4" /> : null}提交任务
        </Button>
        <Button variant="ghost" onClick={onDone}>
          取消
        </Button>
      </div>
    </div>
  );
}
