import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function ProductCreatePage() {
  return (
    <ResourcePageScaffold
      resourceKey="product"
      mode="create"
      listPath="/products"
    />
  );
}
