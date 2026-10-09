import { hostShim } from "@/foundation/lib/host-shim";
import { useTranslation } from "react-i18next";
import { MaterialForm } from "@/domains/materials/components/material-form";
import { useCreateMaterial } from "@/domains/materials/hooks/use-material-mutations";
import type { MaterialInput } from "@/domains/materials/types";

export function MaterialsCreate({
  onDone,
  instanceId,
}: {
  onDone: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const createMaterial = useCreateMaterial();
  const [value, setValue] = hostShim.useInstanceState<MaterialInput>(
    instanceId,
    "materials_material_create",
    () => ({
      scope: "",
      asset_kind: "",
      name: "",
      object_path: "",
      visibility: "private",
    }),
  );

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      {createMaterial.error ? (
        <p role="alert" className="text-sm text-destructive">
          {createMaterial.error.message}
        </p>
      ) : null}
      <h2 className="text-lg font-semibold">{t("materialForm.createTitle")}</h2>
      <MaterialForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          createMaterial.mutate(value, {
            onSuccess: () => {
              setValue({
                scope: "",
                asset_kind: "",
                name: "",
                object_path: "",
                visibility: "private",
              });
              onDone();
            },
          })
        }
        submitLabel={t("common.create")}
        isPending={createMaterial.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
