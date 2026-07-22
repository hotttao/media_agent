import {
  useArchiveProduct,
  useDeleteProduct,
} from "@/domains/products/hooks/use-product-mutations";
import { useProduct } from "@/domains/products/hooks/use-product";
import { StatusBadge } from "@/domains/products/components/status-badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

export function ProductShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: product, isLoading } = useProduct(id);
  const deleteProduct = useDeleteProduct();
  const archiveProduct = useArchiveProduct();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!product) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
        <div className="flex gap-2">
          {product.status !== "archived" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => archiveProduct.mutate(id)}
              disabled={archiveProduct.isPending}
            >
              Archive
            </Button>
          ) : null}
          <Button size="sm" onClick={onEdit}>
            Edit
          </Button>
        </div>
      </div>
      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="col-span-2 font-medium">{product.name}</dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{product.scope}</dd>
        <dt className="text-muted-foreground">Status</dt>
        <dd className="col-span-2">
          <StatusBadge status={product.status} />
        </dd>
        <dt className="text-muted-foreground">Description</dt>
        <dd className="col-span-2">{product.description || "-"}</dd>
        <dt className="text-muted-foreground">Metadata</dt>
        <dd className="col-span-2">
          {product.metadata ? (
            <pre className="max-h-80 overflow-auto rounded bg-muted p-2 text-xs">
              {JSON.stringify(product.metadata, null, 2)}
            </pre>
          ) : (
            "-"
          )}
        </dd>
        <dt className="text-muted-foreground">Created By</dt>
        <dd className="col-span-2">{product.created_by}</dd>
        <dt className="text-muted-foreground">Updated By</dt>
        <dd className="col-span-2">{product.updated_by ?? "-"}</dd>
        <dt className="text-muted-foreground">Created At</dt>
        <dd className="col-span-2">{product.created_at}</dd>
        <dt className="text-muted-foreground">Updated At</dt>
        <dd className="col-span-2">{product.updated_at}</dd>
      </dl>
      <Button
        variant="destructive"
        size="sm"
        onClick={() => {
          deleteProduct.mutate(id);
          onBack();
        }}
      >
        Delete
      </Button>
    </div>
  );
}
