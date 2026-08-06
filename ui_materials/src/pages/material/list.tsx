import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function MaterialListPage() {
  return (
    <ResourcePageScaffold
      resourceKey="material"
      mode="list"
      listPath="/materials"
      createPath="/materials/new"
    />
  );
}
