import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useCreateTemplate } from "@/domains/templates/hooks/use-template-mutations";
import { useChangeTemplateVisibility } from "@/domains/templates/hooks/use-template-mutations";
import { TemplateForm } from "@/domains/templates/components/template-form";
import { Spinner } from "@/components/ui/spinner";
import type { TemplateInput } from "@/domains/templates/types";
import { hostShim } from "@/foundation/lib/host-shim";

export function TemplateCreate({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation();
  const shell = hostShim.useShellContext();
  const createTemplate = useCreateTemplate();
  const changeVisibility = useChangeTemplateVisibility();
  const [value, setValue] = useState<TemplateInput>({
    scope: "",
    name: "",
    description: "",
    prompt_body: "",
    inputs: [],
    output_asset_kind: "",
    platform: "",
    gen_way: "",
    gen_type: "",
    needs_confirm: false,
    candidate_count: 0,
    capability_key: "",
    capability_version: 1,
    invocation_mapping: { prompt: { op: "render_template" } },
    visibility: "private",
  });

  function handleSubmit() {
    if (shell.isLoading || !shell.organization.id) return;
    createTemplate.mutate(
      { ...value, organization_id: shell.organization.id },
      {
        onSuccess: (created) => {
          // proto's CreateTemplateReq does not carry visibility; if the user
          // selected a non-default value, apply it via the dedicated endpoint.
          if (value.visibility && value.visibility !== "private") {
            changeVisibility.mutate({
              id: created.id,
              input: { visibility: value.visibility },
            });
          }
          onDone();
        },
      },
    );
  }

  if (createTemplate.isPending) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto p-4">
      {createTemplate.isError || changeVisibility.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {createTemplate.error?.message || changeVisibility.error?.message}
        </p>
      ) : null}
      <h2 className="text-lg font-semibold">{t("v12.newTemplate")}</h2>
      <TemplateForm
        value={value}
        onChange={setValue}
        onSubmit={handleSubmit}
        submitLabel={t("editor.savedDraft")}
        isPending={createTemplate.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
