import { useTranslation } from "react-i18next";
import { hostShim } from "@/foundation/lib/host-shim";
import { useCreateAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import { AssetKindForm } from "@/domains/asset-kinds/components/asset-kind-form";
import type { AssetKindInput } from "@/domains/asset-kinds/types";

export function AssetKindCreate({
  onDone,
  instanceId,
}: {
  onDone: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const createAssetKind = useCreateAssetKind();
  const [value, setValue] = hostShim.useInstanceState<AssetKindInput>(
    instanceId,
    "materials_kind_create",
    () => ({
      name: "",
      resource_type: "MATERIAL",
      media_type: "IMAGE",
    }),
  );

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      {createAssetKind.error ? (
        <p role="alert" className="text-sm text-destructive">
          {createAssetKind.error.message}
        </p>
      ) : null}
      <AssetKindForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          createAssetKind.mutate(value, {
            onSuccess: () => {
              setValue({
                name: "",
                resource_type: "MATERIAL",
                media_type: "IMAGE",
              });
              onDone();
            },
          })
        }
        submitLabel={t("common.create")}
        isPending={createAssetKind.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
