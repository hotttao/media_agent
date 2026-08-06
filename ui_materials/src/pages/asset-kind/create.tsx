import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function AssetKindCreatePage() {
  return (
    <ResourcePageScaffold
      resourceKey="assetKind"
      mode="create"
      listPath="/settings/asset-kinds"
    />
  );
}
