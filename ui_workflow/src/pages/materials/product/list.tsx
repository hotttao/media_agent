import { useProductsList } from "@/domains/products/hooks/use-products-list";
import { useDeleteProduct } from "@/domains/products/hooks/use-product-mutations";
import { StatusBadge } from "@/domains/products/components/status-badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { Trash2 } from "lucide-react";

export function ProductList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading } = useProductsList();
  const deleteProduct = useDeleteProduct();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  const items = data?.items ?? [];
  if (items.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        No products yet.
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-card">
          <tr className="border-b text-left text-xs uppercase text-muted-foreground">
            <th className="px-3 py-2">Name</th>
            <th className="px-3 py-2">Scope</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2">Updated</th>
            <th className="px-3 py-2" aria-label="actions" />
          </tr>
        </thead>
        <tbody>
          {items.map((p) => (
            <tr key={p.id} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(p.id)}
                >
                  {p.name}
                </button>
              </td>
              <td className="px-3 py-2 text-muted-foreground">{p.scope}</td>
              <td className="px-3 py-2">
                <StatusBadge status={p.status} />
              </td>
              <td className="px-3 py-2 text-muted-foreground">{p.updated_at}</td>
              <td className="px-3 py-2">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="delete"
                  onClick={() => deleteProduct.mutate(p.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </ScrollArea>
  );
}
