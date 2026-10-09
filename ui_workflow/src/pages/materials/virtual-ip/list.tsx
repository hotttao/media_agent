import { Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { ConfirmActionButton } from "@/foundation/components/confirm-action-button";
import {
  ListPagination,
  ListState,
  ResourceListLayout,
} from "@/foundation/components/resource-list-layout";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useDeleteVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import { useVirtualIpsList } from "@/domains/virtual-ips/hooks/use-virtual-ips-list";
import { LibraryImage } from "@/foundation/components/library-image";

export function VirtualIpList({
  onSelect,
}: {
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const query = useVirtualIpsList({ page, page_size: 20, name: search.trim() });
  const remove = useDeleteVirtualIp();
  return (
    <ResourceListLayout
      title={t("nav.virtualIps")}
      description={t("library.virtualHint")}
      filterLabel={t("v12.search")}
      filters={null}
      search={search}
      onSearch={(value) => {
        setSearch(value);
        setPage(1);
      }}
    >
      {query.isLoading ? (
        <ListState loading />
      ) : query.isError ? (
        <ListState
          error={query.error.message}
          onRetry={() => void query.refetch()}
        />
      ) : !query.data?.items.length ? (
        <ListState empty={t("library.empty")} />
      ) : (
        <div className="media-library-grid">
          {query.data.items.map((item) => (
            <article
              key={item.id}
              className="group relative overflow-hidden rounded-2xl bg-background"
            >
              <button
                type="button"
                className="block w-full text-left"
                onClick={() => onSelect(item.id)}
              >
                <div className="relative aspect-[4/3] overflow-hidden bg-muted/40">
                  <LibraryImage
                    organizationId={item.organization_id}
                    objectPath={
                      item.avatar_object_path ||
                      item.full_body_object_path ||
                      item.three_view_object_path ||
                      item.nine_view_object_path
                    }
                    alt={item.name}
                    className="object-contain transition duration-300 group-hover:scale-[1.02]"
                  />
                  <div className="absolute right-3 top-3">
                    <VisibilityBadge visibility={item.visibility} />
                  </div>
                </div>
                <div className="p-4 pr-12">
                  <h2 className="truncate text-sm font-semibold">
                    {item.name}
                  </h2>
                  <p className="mt-2 line-clamp-2 text-xs leading-5 text-muted-foreground">
                    {item.description || item.scope}
                  </p>
                  <span className="mt-2 inline-block text-xs text-muted-foreground">
                    {t(`common.status.${item.status}`, {
                      defaultValue: item.status,
                    })}
                  </span>
                </div>
              </button>
              <ConfirmActionButton
                variant="ghost"
                size="icon"
                aria-label={`${t("common.delete")} ${item.name}`}
                className="absolute bottom-3 right-3 h-8 w-8"
                disabled={remove.isPending}
                onConfirm={() => remove.mutateAsync(item.id)}
              >
                <Trash2 className="h-4 w-4" />
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
