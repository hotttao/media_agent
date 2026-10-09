import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useGeneratedAssetsList } from "@/domains/generated-assets/hooks/use-generated-assets-list";
import { useDeleteGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset-mutations";
import {
  ListPagination,
  ListState,
  ResourceListLayout,
} from "@/foundation/components/resource-list-layout";
import { LibraryMedia } from "@/foundation/components/library-media";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import { hostShim } from "@/foundation/lib/host-shim";

export function GeneratedAssetList({
  onSelect,
}: {
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  const shell = hostShim.useShellContext();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const query = useGeneratedAssetsList({ page, page_size: 20 });
  const remove = useDeleteGeneratedAsset();
  const items = (query.data?.items ?? []).filter((item) =>
    item.name?.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <ResourceListLayout
      title={t("nav.generated")}
      description={t("library.generatedHint")}
      search={search}
      onSearch={setSearch}
      filterLabel={t("v12.pageOnly")}
      filters={null}
    >
      {query.isLoading ? (
        <ListState loading />
      ) : query.isError ? (
        <ListState
          error={query.error.message}
          onRetry={() => void query.refetch()}
        />
      ) : !items.length ? (
        <ListState empty={t("library.empty")} />
      ) : (
        <div className="media-library-grid">
          {items.map((item) => (
            <article
              key={item.id}
              className="overflow-hidden rounded-2xl bg-background p-3"
            >
              {item.object_path || item.url ? (
                <LibraryMedia
                  organizationId={shell.organization.id}
                  objectPath={item.object_path || item.url}
                  alt={item.name}
                  video={item.asset_kind === "video"}
                />
              ) : (
                <p className="line-clamp-6 min-h-36 whitespace-pre-wrap rounded-xl bg-muted/40 p-4 text-sm">
                  {item.content || "—"}
                </p>
              )}
              <button
                type="button"
                onClick={() => onSelect(item.id)}
                className="mt-3 text-left"
              >
                <h2 className="text-sm font-semibold">{item.name}</h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  {item.asset_kind}
                </p>
              </button>
              <ConfirmActionButton
                className="float-right mt-2"
                size="sm"
                variant="ghost"
                onConfirm={() => remove.mutateAsync(item.id)}
              >
                {t("common.delete")}
              </ConfirmActionButton>
            </article>
          ))}
        </div>
      )}
      <ListPagination
        page={page}
        total={query.data?.total ?? 0}
        pageSize={20}
        onPage={setPage}
      />
    </ResourceListLayout>
  );
}
