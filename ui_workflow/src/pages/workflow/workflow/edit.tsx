import { useEffect, useState } from "react";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import {
  useUpdateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import { Spinner } from "@/components/ui/spinner";
import type { ValidationResult, WorkflowInput } from "@/domains/workflows/types";

export function WorkflowEdit({ id, onDone }: { id: string; onDone: () => void }) {
  const { data: workflow, isLoading } = useWorkflow(id);
  const updateWorkflow = useUpdateWorkflow();
  const validate = useValidateWorkflow();
  const [value, setValue] = useState<WorkflowInput | null>(null);
  const [validationResult, setValidationResult] = useState<ValidationResult | null>(null);

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
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-lg font-semibold">Edit Workflow</h2>
      <WorkflowForm
        value={value}
        onChange={setValue}
        onSubmit={() => updateWorkflow.mutate({ id, patch: value }, { onSuccess: onDone })}
        submitLabel="Save"
        isPending={updateWorkflow.isPending}
        onCancel={onDone}
        onValidate={() => validate.mutate(value, { onSuccess: setValidationResult })}
        validationResult={validationResult}
      />
    </div>
  );
}
