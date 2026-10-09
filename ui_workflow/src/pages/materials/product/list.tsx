import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import { PackageOpen, Search, ShoppingBag, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const deleteProduct = useDeleteProduct();
  const [search, setSearch] = useState("");
  const [name, setName] = useState("");
  const [page, setPage] = useState(1);
  const pageSize = 20;
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setName(search.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [search]);
  const { data, isLoading, isError, error, isFetching } = useProductsList({
    page,
    page_size: pageSize,
    name,
  });
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  useEffect(() => {
    if (data && page > pages) setPage(pages);
  }, [data, page, pages]);

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
        {t("productsList.error")}:{" "}
        {error instanceof Error ? error.message : "Request failed"}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-gradient-to-b from-background to-muted/10">
      <div className="flex flex-col gap-4 border-b border-border/60 px-5 py-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold tracking-tight">
              {t("productsList.title")}
            </h2>
            <span className="rounded-full border border-border/70 bg-muted/40 px-2 py-0.5 text-xs text-muted-foreground">
              {total}
            </span>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("productsList.description")}
          </p>
        </div>
        <div className="relative w-full sm:w-72">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label={t("productsList.search")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t("productsList.search")}
            className="bg-background/70 pl-9"
          />
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        {items.length === 0 ? (
          <div className="flex h-64 flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
            <PackageOpen className="h-8 w-8 opacity-60" />
            {name ? t("productsList.noMatch") : t("productsList.empty")}
          </div>
        ) : (
          <div className="media-library-grid p-5">
            {items.map((item) => {
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
                  data-testid={`product-card-${item.id}`}
                  className="group relative overflow-hidden rounded-2xl border border-border/70 bg-card/80 shadow-sm transition duration-300 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-xl"
                >
                  <button
                    type="button"
                    className="block w-full text-left"
                    onClick={() => onSelect(item.id)}
                  >
                    <div className="relative aspect-square overflow-hidden bg-muted/40">
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
                          : item.description || t("productsList.fallback")}
                      </p>
                    </div>
                  </button>
                  <ConfirmActionButton
                    variant="ghost"
                    size="icon"
                    aria-label={t("productsList.delete", { name: item.name })}
                    className="absolute bottom-2 right-2 h-8 w-8 opacity-0 transition group-hover:opacity-100 focus:opacity-100"
                    disabled={deleteProduct.isPending}
                    onConfirm={() => deleteProduct.mutateAsync(item.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </ConfirmActionButton>
                </article>
              );
            })}
          </div>
        )}
      </ScrollArea>
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border/60 px-5 py-3 text-sm text-muted-foreground">
        <span>
          {total
            ? t("productsList.showing", {
                start: (page - 1) * pageSize + 1,
                end: Math.min(page * pageSize, total),
                total,
              })
            : t("productsList.total", { count: 0 })}
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1 || isFetching}
            onClick={() => setPage((value) => value - 1)}
          >
            {t("common.previous")}
          </Button>
          <span className="min-w-20 text-center">
            {t("common.page", { page, pages })}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= pages || isFetching}
            onClick={() => setPage((value) => value + 1)}
          >
            {t("common.next")}
          </Button>
        </div>
      </div>
    </div>
  );
}
