import { useEffect, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { getTemplateVersion } from "@/domains/templates/lib/api";
import { useTranslation } from "react-i18next";
import { ListState } from "@/foundation/components/resource-list-layout";
import { hostShim } from "@/foundation/lib/host-shim";
import { allows } from "@/foundation/types/resource-access";
import { Spinner } from "@/components/ui/spinner";
import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { WorkflowForm } from "@/domains/workflows/components/workflow-form";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import {
  useUpdateWorkflow,
  useValidateWorkflow,
} from "@/domains/workflows/hooks/use-workflow-mutations";
import type {
  ValidationResult,
  WorkflowInput,
} from "@/domains/workflows/types";

export function WorkflowEdit({
  id,
  onDone,
  instanceId,
}: {
  id: string;
  onDone: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const {
    data: workflow,
    isLoading,
    isError,
    error,
    refetch,
  } = useWorkflow(id);
  const updateWorkflow = useUpdateWorkflow();
  const validate = useValidateWorkflow();
  const [value, setValue] = hostShim.useInstanceState<WorkflowInput | null>(
    instanceId,
    `workflow_draft_${id}`,
    () => null,
  );
  const [validationResult, setValidationResult] = useState<{
    fingerprint: string;
    result: ValidationResult;
  } | null>(null);
  const { data: templates } = useTemplatesList({
    scope: value?.scope || undefined,
    page_size: 100,
  });
  const references = [
    ...new Map(
      (value?.steps ?? [])
        .filter((step) => step.template_id && step.template_version)
        .map((step) => [`${step.template_id}@${step.template_version}`, step]),
    ).values(),
  ];
  const versions = useQueries({
    queries: references.map((step) => ({
      queryKey: ["template-version", step.template_id, step.template_version],
      queryFn: () =>
        getTemplateVersion(step.template_id, step.template_version!),
    })),
  });
  const templateOptions = [
    ...new Map(
      [
        ...(templates?.items ?? []),
        ...versions.flatMap((query) => (query.data ? [query.data] : [])),
      ].map((template) => [`${template.id}@${template.version}`, template]),
    ).values(),
  ];

  useEffect(() => {
    if (workflow) {
      setValue(
        (current) =>
          current ?? {
            scope: workflow.scope,
            name: workflow.name,
            description: workflow.description,
            output_step_key: workflow.output_step_key,
            steps: workflow.steps,
            visibility: workflow.visibility,
          },
      );
    }
  }, [workflow, setValue]);

  if (isError)
    return (
      <ListState
        error={error.message || t("editor.loadError")}
        onRetry={() => void refetch()}
      />
    );

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-background">
      {!allows(workflow, "update") ? (
        <p role="alert" className="p-4 text-sm text-muted-foreground">
          {t("sharing.readOnly")}
        </p>
      ) : null}
      <WorkflowForm
        value={value}
        resolveTemplate={async (option) => ({
          ...(await getTemplateVersion(option.id, option.version)),
          name: option.name,
        })}
        onChange={(next) => {
          if (allows(workflow, "update")) setValue(next);
        }}
        onSubmit={() =>
          allows(workflow, "update") &&
          updateWorkflow.mutate(
            { id, patch: value },
            {
              onSuccess: () => {
                setValue(null);
                onDone();
              },
            },
          )
        }
        submitLabel={t("editor.savedDraft")}
        isPending={updateWorkflow.isPending}
        editable={allows(workflow, "update")}
        onCancel={onDone}
        onValidate={() =>
          validate.mutate(value, {
            onSuccess: (result) =>
              setValidationResult({
                fingerprint: JSON.stringify(value),
                result,
              }),
          })
        }
        validationResult={
          validationResult?.fingerprint === JSON.stringify(value)
            ? validationResult.result
            : null
        }
        templateOptions={templateOptions.map((template) => ({
          ...template,
          name:
            template.name ||
            templates?.items.find((head) => head.id === template.id)?.name ||
            template.id,
        }))}
        metadataLocked
        error={updateWorkflow.error?.message || validate.error?.message}
      />
    </div>
  );
}
