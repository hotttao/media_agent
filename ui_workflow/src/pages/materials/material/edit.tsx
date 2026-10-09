import { hostShim } from "@/foundation/lib/host-shim";
import { ListState } from "@/foundation/components/resource-list-layout";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/spinner";
import { MaterialForm } from "@/domains/materials/components/material-form";
import { useMaterial } from "@/domains/materials/hooks/use-material";
import { useUpdateMaterial } from "@/domains/materials/hooks/use-material-mutations";
import type { MaterialInput } from "@/domains/materials/types";

function editableMetadata(
  metadata: Record<string, unknown> | string | undefined,
): Record<string, unknown> | undefined {
  if (typeof metadata !== "string") return metadata;
  try {
    return JSON.parse(metadata) as Record<string, unknown>;
  } catch {
    return undefined;
  }
}

export function MaterialsEdit({
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
    data: material,
    isLoading,
    isError,
    error,
    refetch,
  } = useMaterial(id);
  const updateMaterial = useUpdateMaterial();
  const [value, setValue] = hostShim.useInstanceState<MaterialInput | null>(
    instanceId,
    "materials_material_draft_" + id,
    () => null,
  );

  useEffect(() => {
    if (material) {
      setValue(
        (current) =>
          current ?? {
            scope: material.scope,
            asset_kind: material.asset_kind,
            name: material.name,
            object_path: material.object_path ?? "",
            description: material.description,
            prompt: material.prompt,
            content: material.content,
            visibility: material.visibility,
            team_id: material.team_id,
            metadata: editableMetadata(material.metadata),
          },
      );
    }
  }, [material, setValue]);

  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      {updateMaterial.error ? (
        <p role="alert" className="text-sm text-destructive">
          {updateMaterial.error.message}
        </p>
      ) : null}
      <h2 className="text-lg font-semibold">{t("materialForm.editTitle")}</h2>
      <MaterialForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateMaterial.mutate(
            { id, patch: value },
            {
              onSuccess: () => {
                setValue(null);
                onDone();
              },
            },
          )
        }
        submitLabel={t("common.save")}
        isPending={updateMaterial.isPending}
        onCancel={onDone}
        isEdit
      />
    </div>
  );
}
