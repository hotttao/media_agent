import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { hostShim } from "@/foundation/lib/host-shim";
import { ListState } from "@/foundation/components/resource-list-layout";
import { Spinner } from "@/components/ui/spinner";
import { AssetKindForm } from "@/domains/asset-kinds/components/asset-kind-form";
import { useAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind";
import { useUpdateAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import type { AssetKindInput } from "@/domains/asset-kinds/types";

export function AssetKindEdit({
  name,
  onDone,
  instanceId,
}: {
  name: string;
  onDone: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const {
    data: assetKind,
    isLoading,
    isError,
    error,
    refetch,
  } = useAssetKind(name);
  const updateAssetKind = useUpdateAssetKind();
  const [value, setValue] = hostShim.useInstanceState<AssetKindInput | null>(
    instanceId,
    `materials_kind_edit_${name}`,
    () => null,
  );

  useEffect(() => {
    if (assetKind) {
      setValue(
        (current) =>
          current ?? {
            name: assetKind.name,
            resource_type: assetKind.resource_type,
            media_type: assetKind.media_type,
            description: assetKind.description,
            status: assetKind.status,
          },
      );
    }
  }, [assetKind, setValue]);

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
      {updateAssetKind.error ? (
        <p role="alert" className="text-sm text-destructive">
          {updateAssetKind.error.message}
        </p>
      ) : null}
      <AssetKindForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateAssetKind.mutate(
            {
              name,
              patch: {
                description: value.description,
                status: value.status ?? "ENABLED",
              },
            },
            {
              onSuccess: () => {
                setValue(null);
                onDone();
              },
            },
          )
        }
        submitLabel={t("common.save")}
        isPending={updateAssetKind.isPending}
        onCancel={onDone}
        nameDisabled
      />
    </div>
  );
}
