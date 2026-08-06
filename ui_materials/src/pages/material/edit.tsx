import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function MaterialEditPage() {
  return (
    <ResourcePageScaffold
      resourceKey="material"
      mode="edit"
      listPath="/materials"
    />
  );
}
