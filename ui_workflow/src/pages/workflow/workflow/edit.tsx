import { useEffect, useState } from "react";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import {
  useUpdateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import { Spinner } from "@/components/ui/spinner";
import type {
  ValidationResult,
  WorkflowInput,
} from "@/domains/workflows/types";

export function WorkflowEdit({
  id,
  onDone,
}: {
  id: string;
  onDone: () => void;
}) {
  const { data: workflow, isLoading } = useWorkflow(id);
  const updateWorkflow = useUpdateWorkflow();
  const validate = useValidateWorkflow();
  const [value, setValue] = useState<WorkflowInput | null>(null);
  const [validationResult, setValidationResult] =
    useState<ValidationResult | null>(null);
  const { data: templates } = useTemplatesList({
    scope: value?.scope || undefined,
    page_size: 100,
  });

  useEffect(() => {
    if (workflow) {
      setValue({
        scope: workflow.scope,
        name: workflow.name,
        description: workflow.description,
        output_step_key: workflow.output_step_key,
        steps: workflow.steps,
        visibility: workflow.visibility,
      });
    }
  }, [workflow]);

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto bg-gradient-to-b from-slate-50/80 to-background dark:from-slate-950/80">
      <div className="mx-auto flex min-h-full max-w-[1600px] flex-col gap-5 p-4 sm:p-6 lg:p-8">
        <div>
          <div className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">
            Workflow Studio
          </div>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight">
            编辑工作流
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            画布展示依赖关系，右侧面板编辑节点的业务定义。
          </p>
        </div>
        <WorkflowForm
          value={value}
          onChange={setValue}
          onSubmit={() =>
            updateWorkflow.mutate({ id, patch: value }, { onSuccess: onDone })
          }
          submitLabel="Save"
          isPending={updateWorkflow.isPending}
          onCancel={onDone}
          onValidate={() =>
            validate.mutate(value, { onSuccess: setValidationResult })
          }
          validationResult={validationResult}
          templateOptions={templates?.items}
        />
      </div>
    </div>
  );
}
