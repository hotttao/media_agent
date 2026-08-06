import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function AssetKindEditPage() {
  return (
    <ResourcePageScaffold
      resourceKey="assetKind"
      mode="edit"
      listPath="/settings/asset-kinds"
    />
  );
}
