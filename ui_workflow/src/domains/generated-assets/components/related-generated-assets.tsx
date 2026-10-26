import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useGeneratedAssetsList } from "@/domains/generated-assets/hooks/use-generated-assets-list";
import type { GeneratedAsset } from "@/domains/generated-assets/types";
import { formatDetailDate } from "@/foundation/components/detail-layout";
import { LibraryMedia } from "@/foundation/components/library-media";
import {
  ListPagination,
  ListState,
} from "@/foundation/components/resource-list-layout";

function AssetMedia({
  asset,
  organizationId,
  interactive = true,
}: {
  asset: GeneratedAsset;
  organizationId: string;
  interactive?: boolean;
}) {
  return asset.object_path || asset.url ? (
    <LibraryMedia
      organizationId={organizationId}
      objectPath={asset.object_path || asset.url}
      alt={asset.name}
      interactive={interactive}
    />
  ) : (
    <p
      className={`min-h-36 whitespace-pre-wrap rounded-xl bg-muted/40 p-4 text-sm ${interactive ? "" : "line-clamp-6"}`}
    >
      {asset.content || "—"}
    </p>
  );
}

export function RelatedGeneratedAssets({
  organizationId,
  sourceProductId,
  scope,
}: {
  organizationId: string;
  sourceProductId: string;
  scope: string;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<GeneratedAsset | null>(null);
  const query = useGeneratedAssetsList({
    organization_id: organizationId,
    source_product_id: sourceProductId,
    scope,
    page,
    page_size: 12,
  });
  const items = query.data?.items ?? [];
  return (
    <section aria-label={t("productDetail.relatedTab")} className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {t("productDetail.relatedHint")}
      </p>
      {query.isLoading ? (
        <ListState loading />
      ) : query.isError ? (
        <ListState
          error={query.error.message}
          onRetry={() => void query.refetch()}
        />
      ) : !items.length ? (
        <ListState empty={t("productDetail.relatedEmpty")} />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {items.map((asset) => (
            <button
              key={asset.id}
              type="button"
              onClick={() => setSelected(asset)}
              aria-label={t("productDetail.previewAsset", { name: asset.name })}
              className="min-w-0 overflow-hidden rounded-2xl border border-border/70 bg-card p-3 text-left transition-colors hover:border-primary/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <AssetMedia
                asset={asset}
                organizationId={organizationId}
                interactive={false}
              />
              <h3 className="mt-3 truncate text-sm font-semibold">
                {asset.name}
              </h3>
              <div className="mt-1 flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
                <span>{asset.asset_kind}</span>
                <span>{formatDetailDate(asset.created_at)}</span>
              </div>
            </button>
          ))}
        </div>
      )}
      {!query.isError && !query.isLoading && (query.data?.total ?? 0) > 12 ? (
        <ListPagination
          page={page}
          pageSize={12}
          total={query.data?.total ?? 0}
          onPage={setPage}
        />
      ) : null}
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      >
        {selected ? (
          <DialogContent className="max-h-[90vh] w-[calc(100%-2rem)] max-w-4xl overflow-y-auto">
            <DialogHeader>
              <DialogTitle className="break-words pr-6">
                {selected.name}
              </DialogTitle>
              <DialogDescription>
                {selected.asset_kind} · {formatDetailDate(selected.created_at)}
              </DialogDescription>
            </DialogHeader>
            <AssetMedia asset={selected} organizationId={organizationId} />
          </DialogContent>
        ) : null}
      </Dialog>
    </section>
  );
}
