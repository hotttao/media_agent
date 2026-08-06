import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function AssetKindListPage() {
  return (
    <ResourcePageScaffold
      resourceKey="assetKind"
      mode="list"
      listPath="/settings/asset-kinds"
      createPath="/settings/asset-kinds/new"
    />
  );
}
