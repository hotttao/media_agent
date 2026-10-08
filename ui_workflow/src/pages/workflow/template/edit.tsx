import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useTemplate } from "@/domains/templates/hooks/use-template";
import {
  useChangeTemplateVisibility,
  useUpdateTemplate,
} from "@/domains/templates/hooks/use-template-mutations";
import { TemplateForm } from "@/domains/templates/components/template-form";
import { Spinner } from "@/components/ui/spinner";
import { ListState } from "@/foundation/components/resource-list-layout";
import type { TemplateInput } from "@/domains/templates/types";

export function TemplateEdit({
  id,
  onDone,
}: {
  id: string;
  onDone: () => void;
}) {
  const {
    data: template,
    isLoading,
    isError,
    error,
    refetch,
  } = useTemplate(id);
  const { t } = useTranslation();
  const updateTemplate = useUpdateTemplate();
  const changeVisibility = useChangeTemplateVisibility();
  const [value, setValue] = useState<TemplateInput | null>(null);

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
  }, [template]);

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
    if (!value) return;
    const originalVisibility = template?.visibility;
    updateTemplate.mutate(
      { id, patch: value },
      {
        onSuccess: () => {
          // proto's UpdateTemplateReq does not carry visibility; if the user
          // changed it, apply via the dedicated endpoint.
          if (value.visibility && value.visibility !== originalVisibility) {
            changeVisibility.mutate({
              id,
              input: { visibility: value.visibility },
            });
          }
          onDone();
        },
      },
    );
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto p-4">
      {updateTemplate.isError || changeVisibility.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {updateTemplate.error?.message || changeVisibility.error?.message}
        </p>
      ) : null}
      <h2 className="text-lg font-semibold">
        {t("nav.templates")} · {t("editor.savedDraft")}
      </h2>
      <TemplateForm
        value={value}
        onChange={setValue}
        onSubmit={handleSubmit}
        submitLabel={t("editor.savedDraft")}
        isPending={updateTemplate.isPending}
        onCancel={onDone}
        scopeLocked
      />
    </div>
  );
}
