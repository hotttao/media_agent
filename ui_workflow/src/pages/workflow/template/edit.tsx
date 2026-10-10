import { useEffect, useState } from "react";
import { CapabilitySelector } from "@/domains/capabilities/components/capability-selector";
import { useTranslation } from "react-i18next";
import { useTemplate } from "@/domains/templates/hooks/use-template";
import { useUpdateTemplate } from "@/domains/templates/hooks/use-template-mutations";
import { TemplateForm } from "@/domains/templates/components/template-form";
import { Spinner } from "@/components/ui/spinner";
import { ListState } from "@/foundation/components/resource-list-layout";
import type { TemplateInput } from "@/domains/templates/types";
import { hostShim } from "@/foundation/lib/host-shim";
import { allows } from "@/foundation/types/resource-access";

export function TemplateEdit({
  id,
  onDone,
  instanceId,
}: {
  id: string;
  onDone: () => void;
  instanceId: string;
}) {
  const {
    data: template,
    isLoading,
    isError,
    error,
    refetch,
  } = useTemplate(id);
  const { t } = useTranslation();
  const [capabilityReady, setCapabilityReady] = useState(false);
  const updateTemplate = useUpdateTemplate();
  const [value, setValue] = hostShim.useInstanceState<TemplateInput | null>(
    instanceId,
    `workflow_template_draft_${id}`,
    () => null,
  );

  useEffect(() => {
    if (template) {
      setValue(
        (current) =>
          current ?? {
            scope: template.scope,
            name: template.name,
            description: template.description,
            prompt_body: template.prompt_body,
            inputs: template.inputs.map((s) => ({
              name: s.name,
              asset_kind: s.asset_kind,
              resource_type: s.resource_type,
              required: s.required,
              display_order: s.display_order,
            })),
            output_asset_kind: template.output_asset_kind,
            platform: template.platform,
            gen_way: template.gen_way,
            gen_type: template.gen_type,
            needs_confirm: template.needs_confirm,
            candidate_count: template.candidate_count,
            capability_key: template.capability_key,
            capability_version: template.capability_version,
            invocation_mapping: template.invocation_mapping,
            visibility: template.visibility,
          },
      );
    }
  }, [template, setValue]);

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

  function handleSubmit() {
    if (!value || !allows(template, "update") || !capabilityReady) return;
    updateTemplate.mutate(
      { id, patch: value },
      {
        onSuccess: () => {
          setValue(null);
          onDone();
        },
      },
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {!allows(template, "update") ? (
        <p role="alert" className="p-4 text-sm text-muted-foreground">
          {t("sharing.readOnly")}
        </p>
      ) : null}
      {updateTemplate.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {updateTemplate.error?.message}
        </p>
      ) : null}
      <TemplateForm
        capabilityReady={capabilityReady}
        capabilitySelector={
          <CapabilitySelector
            capabilityKey={value.capability_key}
            version={value.capability_version}
            onReady={setCapabilityReady}
            onSelect={(contract) => {
              setCapabilityReady(false);
              setValue({
                ...value,
                capability_key: contract.key,
                capability_version: contract.version,
                platform: contract.provider,
                gen_way: contract.generation_method,
                gen_type: contract.generation_type,
              });
            }}
          />
        }
        value={value}
        onChange={(next) => {
          if (allows(template, "update")) setValue(next);
        }}
        onSubmit={handleSubmit}
        submitLabel={t("editor.savedDraft")}
        isPending={updateTemplate.isPending}
        editable={allows(template, "update")}
        onCancel={onDone}
        scopeLocked
      />
    </div>
  );
}
