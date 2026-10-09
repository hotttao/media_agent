import { AlertCircle, Search } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function ResourceListLayout({
  title,
  description,
  actions,
  filterLabel,
  filters,
  search,
  onSearch,
  children,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
  filterLabel: string;
  filters: ReactNode;
  search: string;
  onSearch: (value: string) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div
      className={`media-list-layout${filters ? "" : " media-list-layout-unfiltered"}`}
    >
      {filters ? (
        <aside className="media-filter-panel" aria-label={filterLabel}>
          <h2 className="mb-3 px-3 text-xs font-medium text-white/80">
            {filterLabel}
          </h2>
          <div className="media-filter-options">{filters}</div>
        </aside>
      ) : null}
      <main className="media-list-content">
        <header className="media-list-heading">
          <div>
            <h1>{title}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
            {!filters ? (
              <p className="mt-1 text-xs text-muted-foreground">
                {filterLabel}
              </p>
            ) : null}
          </div>
          {actions}
        </header>
        <div className="relative mb-5 max-w-md">
          <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
          <Input
            className="h-10 rounded-full bg-white pl-10"
            aria-label={t("v12.search")}
            placeholder={t("v12.search")}
            value={search}
            onChange={(event) => onSearch(event.target.value)}
          />
        </div>
        {children}
      </main>
    </div>
  );
}

export function ListState({
  loading,
  error,
  empty,
  onRetry,
}: {
  loading?: boolean;
  error?: string;
  empty?: string;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  if (loading)
    return (
      <div className="space-y-3" role="status" aria-label={t("v12.loading")}>
        {[1, 2, 3].map((key) => (
          <div
            key={key}
            className="h-28 animate-pulse rounded-2xl bg-white/70"
          />
        ))}
      </div>
    );
  return (
    <div className="media-page-state rounded-2xl border border-dashed bg-white/60">
      {error ? <AlertCircle className="h-6 w-6 text-destructive" /> : null}
      <p
        role={error ? "alert" : undefined}
        className={error ? "text-destructive" : "text-muted-foreground"}
      >
        {error || empty}
      </p>
      {error && onRetry ? (
        <Button variant="outline" onClick={onRetry}>
          {t("v12.retry")}
        </Button>
      ) : null}
    </div>
  );
}

export function ListPagination({
  page,
  pageSize,
  total,
  onPage,
}: {
  page: number;
  pageSize: number;
  total: number;
  onPage: (page: number) => void;
}) {
  const { t } = useTranslation();
  const pages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <nav className="media-pagination" aria-label={t("v12.pagination")}>
      <span className="text-xs text-muted-foreground">
        {t("v12.total", { count: total })} · {t("common.page", { page, pages })}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={page <= 1}
          onClick={() => onPage(page - 1)}
        >
          {t("common.previous")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={page >= pages}
          onClick={() => onPage(page + 1)}
        >
          {t("common.next")}
        </Button>
      </div>
    </nav>
  );
}
