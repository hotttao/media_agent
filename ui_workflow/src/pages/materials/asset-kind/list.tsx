import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import {
  ListPagination,
  ListState,
  ResourceListLayout,
} from "@/foundation/components/resource-list-layout";
import { useDeleteAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import { useAssetKindsList } from "@/domains/asset-kinds/hooks/use-asset-kinds-list";

export function AssetKindList({
  onSelect,
}: {
  onSelect: (name: string) => void;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const query = useAssetKindsList({ page, page_size: 20 });
  const remove = useDeleteAssetKind();
  const items = (query.data?.items || []).filter((item) =>
    item.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  );
  return (
    <ResourceListLayout
      title={t("nav.assetKinds")}
      description={t("registry.hint")}
      filters={null}
      filterLabel={t("v12.pageOnly")}
      search={search}
      onSearch={setSearch}
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
          {items.map((kind) => (
            <article
              key={kind.name}
              className="space-y-3 rounded-2xl bg-background p-4"
            >
              <button
                type="button"
                onClick={() => onSelect(kind.name)}
                className="block w-full text-left"
              >
                <h2 className="break-words text-base font-semibold">
                  {kind.name}
                </h2>
                <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">
                  {kind.description}
                </p>
              </button>
              <dl className="grid grid-cols-2 gap-2 text-xs">
                <dt>{t("registry.resourceType")}</dt>
                <dd>{kind.resource_type}</dd>
                <dt>{t("registry.mediaType")}</dt>
                <dd>{kind.media_type}</dd>
                <dt>{t("registry.status")}</dt>
                <dd>{kind.status}</dd>
              </dl>
              <ConfirmActionButton
                variant="ghost"
                size="sm"
                disabled={remove.isPending || kind.status === "SYSTEM"}
                onConfirm={() => remove.mutateAsync(kind.name)}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                {t("common.delete")}
              </ConfirmActionButton>
            </article>
          ))}
        </div>
      )}
      <ListPagination
        page={page}
        pageSize={20}
        total={query.data?.total || 0}
        onPage={setPage}
      />
    </ResourceListLayout>
  );
}
