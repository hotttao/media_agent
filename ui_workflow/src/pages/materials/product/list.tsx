import { PackageOpen, Search, ShoppingBag, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { StatusBadge } from "@/domains/products/components/status-badge";
import { useDeleteProduct } from "@/domains/products/hooks/use-product-mutations";
import { useProductsList } from "@/domains/products/hooks/use-products-list";
import type { Product } from "@/domains/products/types";
import { LibraryImage } from "@/foundation/components/library-image";

function recordOf(value: Product["metadata"]): Record<string, unknown> {
  if (!value) return {};
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as Record<string, unknown>;
  } catch {
    return {};
  }
}

export function ProductList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading, isError, error } = useProductsList();
  const deleteProduct = useDeleteProduct();
  const [search, setSearch] = useState("");
  const items = data?.items ?? [];
  const visibleItems = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase();
    if (!keyword) return items;
    return items.filter((item) =>
      [item.name, item.scope, item.description]
        .filter(Boolean)
        .some((value) => value?.toLocaleLowerCase().includes(keyword)),
    );
  }, [items, search]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (isError) {
    return (
      <div className="flex h-full items-center justify-center p-4 text-sm text-destructive">
        Unable to load products:{" "}
        {error instanceof Error ? error.message : "request failed"}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-gradient-to-b from-background to-muted/10">
      <div className="flex flex-col gap-4 border-b border-border/60 px-5 py-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold tracking-tight">Products</h2>
            <span className="rounded-full border border-border/70 bg-muted/40 px-2 py-0.5 text-xs text-muted-foreground">
              {items.length}
            </span>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Product references and commerce assets for generation workflows.
          </p>
        </div>
        <div className="relative w-full sm:w-72">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search products"
            className="bg-background/70 pl-9"
          />
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        {visibleItems.length === 0 ? (
          <div className="flex h-64 flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
            <PackageOpen className="h-8 w-8 opacity-60" />
            {items.length === 0
              ? "No products yet."
              : `No products match “${search}”.`}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 p-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
            {visibleItems.map((item) => {
              const metadata = recordOf(item.metadata);
              const categories = Array.isArray(metadata.categories)
                ? metadata.categories.filter(
                    (value): value is string => typeof value === "string",
                  )
                : [];
              const mainImage =
                item.images?.find((image) => image.is_main) ?? item.images?.[0];
              return (
                <article
                  key={item.id}
                  className="group relative overflow-hidden rounded-2xl border border-border/70 bg-card/80 shadow-sm transition duration-300 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-xl"
                >
                  <button
                    type="button"
                    className="block w-full text-left"
                    onClick={() => onSelect(item.id)}
                  >
                    <div className="relative aspect-square overflow-hidden bg-gradient-to-br from-amber-500/10 via-orange-500/5 to-rose-500/10">
                      <LibraryImage
                        organizationId={item.organization_id}
                        objectPath={mainImage?.object_path}
                        alt={item.name}
                        className="object-contain p-3 transition duration-500 group-hover:scale-[1.03]"
                        fallbackClassName="bg-transparent"
                      />
                      <div className="absolute left-3 top-3 flex items-center gap-1.5 rounded-full border border-white/10 bg-background/75 px-2.5 py-1 text-[11px] backdrop-blur-md">
                        <ShoppingBag className="h-3 w-3" />
                        {item.scope || "product"}
                      </div>
                      <div className="absolute right-3 top-3">
                        <StatusBadge status={item.status} />
                      </div>
                    </div>
                    <div className="p-4">
                      <h3 className="h-10 overflow-hidden text-sm font-medium leading-5">
                        {item.name}
                      </h3>
                      <p className="mt-2 truncate text-xs text-muted-foreground">
                        {categories.length > 0
                          ? categories.join(" · ")
                          : item.description || "Commerce product"}
                      </p>
                    </div>
                  </button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`Delete ${item.name}`}
                    className="absolute bottom-2 right-2 h-8 w-8 opacity-0 transition group-hover:opacity-100 focus:opacity-100"
                    disabled={deleteProduct.isPending}
                    onClick={() => deleteProduct.mutate(item.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </article>
              );
            })}
          </div>
        )}
      </ScrollArea>
    </div>
  );
}
