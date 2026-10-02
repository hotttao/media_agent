import { useState } from "react";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import {
  useCreateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import type {
  ValidationResult,
  WorkflowInput,
} from "@/domains/workflows/types";

export function WorkflowCreate({ onDone }: { onDone: () => void }) {
  const createWorkflow = useCreateWorkflow();
  const validate = useValidateWorkflow();
  const [value, setValue] = useState<WorkflowInput>({
    scope: "",
    name: "",
    output_step_key: "",
    steps: [],
    visibility: "private",
  });
  const [validationResult, setValidationResult] =
    useState<ValidationResult | null>(null);
  const { data: templates } = useTemplatesList({
    scope: value.scope || undefined,
    page_size: 100,
  });

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-background">
      <WorkflowForm
        value={value}
        onChange={setValue}
        onSubmit={() => createWorkflow.mutate(value, { onSuccess: onDone })}
        submitLabel="Create"
        isPending={createWorkflow.isPending}
        onCancel={onDone}
        onValidate={() =>
          validate.mutate(value, { onSuccess: setValidationResult })
        }
        validationResult={validationResult}
        templateOptions={templates?.items}
      />
    </div>
  );
}
