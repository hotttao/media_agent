import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function ProductListPage() {
  return (
    <ResourcePageScaffold
      resourceKey="product"
      mode="list"
      listPath="/products"
      createPath="/products/new"
    />
  );
}
