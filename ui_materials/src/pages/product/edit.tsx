import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function ProductEditPage() {
  return (
    <ResourcePageScaffold
      resourceKey="product"
      mode="edit"
      listPath="/products"
    />
  );
}
