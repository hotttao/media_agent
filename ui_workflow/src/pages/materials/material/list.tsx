import { Box, Clock3, ImageIcon, Search, Trash2, Video } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { useAssetKindsList } from "@/domains/asset-kinds/hooks/use-asset-kinds-list";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useDeleteMaterial } from "@/domains/materials/hooks/use-material-mutations";
import { useMaterialsList } from "@/domains/materials/hooks/use-materials-list";
import type { Material } from "@/domains/materials/types";
import { LibraryImage } from "@/foundation/components/library-image";

const KIND_STYLES = [
  "from-emerald-500/25 via-teal-500/10 to-cyan-500/20",
  "from-violet-500/25 via-fuchsia-500/10 to-pink-500/20",
  "from-amber-500/25 via-orange-500/10 to-rose-500/20",
  "from-sky-500/25 via-blue-500/10 to-indigo-500/20",
];

function kindStyle(kind: string): string {
  let hash = 0;
  for (const char of kind) hash = (hash * 31 + char.charCodeAt(0)) | 0;
  return KIND_STYLES[Math.abs(hash) % KIND_STYLES.length];
}

function formatTime(value: string | number, locale: string): string {
  if (!value) return "—";
  const numeric = typeof value === "number" ? value : Number(value);
  const milliseconds =
    Number.isFinite(numeric) && numeric > 10 ** 14
      ? numeric / 10 ** 6
      : numeric;
  const date = Number.isFinite(milliseconds)
    ? new Date(milliseconds)
    : new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    year:
      date.getFullYear() === new Date().getFullYear() ? undefined : "numeric",
  }).format(date);
}

function metadataOf(material: Material): Record<string, unknown> {
  if (!material.metadata) return {};
  if (typeof material.metadata !== "string") return material.metadata;
  try {
    return JSON.parse(material.metadata) as Record<string, unknown>;
  } catch {
    return {};
  }
}

export function MaterialsList({
  onSelect,
}: {
  onSelect: (id: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const [selectedKinds, setSelectedKinds] = useState<string[]>([]);
  const { data, isLoading, isError, error } = useMaterialsList({
    asset_kind: selectedKinds.length > 0 ? selectedKinds.join(",") : undefined,
    only_mine: false,
    page_size: 200,
  });
  const { data: kinds } = useAssetKindsList({
    resource_type: "MATERIAL",
    media_type: "IMAGE",
    page_size: 200,
  });
  const deleteMaterial = useDeleteMaterial();
  const [search, setSearch] = useState("");
  const toggleKind = (kind: string, checked: boolean) => {
    setSelectedKinds((current) =>
      checked
        ? [...current, kind]
        : current.filter((selected) => selected !== kind),
    );
  };
  const items = data?.items ?? [];
  const visibleItems = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase();
    if (!keyword) return items;
    return items.filter((item) =>
      [item.name, item.asset_kind, item.scope]
        .filter(Boolean)
        .some((value) => value.toLocaleLowerCase().includes(keyword)),
    );
  }, [items, search]);

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="flex flex-1 items-center justify-center p-4 text-sm text-destructive">
        {t("materialsList.error")}:{" "}
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
              {t("materialsList.title")}
            </h2>
            <span className="rounded-full border border-border/70 bg-muted/40 px-2 py-0.5 text-xs text-muted-foreground">
              {data?.total ?? items.length}
            </span>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("materialsList.description")}
          </p>
        </div>
        <div className="flex w-full flex-col gap-3 sm:w-auto sm:items-end">
          <fieldset className="flex max-w-3xl flex-wrap items-center justify-end gap-2">
            <legend className="sr-only">{t("materialsList.type")}</legend>
            <div className="flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1.5">
              <Checkbox
                id="material-kind-all"
                checked={selectedKinds.length === 0}
                onCheckedChange={() => setSelectedKinds([])}
              />
              <Label
                htmlFor="material-kind-all"
                className="cursor-pointer whitespace-nowrap text-xs font-medium"
              >
                {t("materialsList.allTypes")}
              </Label>
            </div>
            {(kinds?.items ?? []).map((kind) => {
              const checked = selectedKinds.includes(kind.name);
              return (
                <div
                  key={kind.name}
                  className="flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1.5 transition hover:border-foreground/25"
                >
                  <Checkbox
                    id={`material-kind-${kind.name}`}
                    checked={checked}
                    onCheckedChange={(value) =>
                      toggleKind(kind.name, value === true)
                    }
                  />
                  <Label
                    htmlFor={`material-kind-${kind.name}`}
                    className="cursor-pointer whitespace-nowrap text-xs font-medium"
                  >
                    {t(`materialsList.kind.${kind.name}`, {
                      defaultValue: kind.name,
                    })}
                  </Label>
                </div>
              );
            })}
          </fieldset>
          <div className="relative w-full sm:w-64">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t("materialsList.search")}
              className="bg-background/70 pl-9"
            />
          </div>
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        {visibleItems.length === 0 ? (
          <div className="flex h-56 flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <ImageIcon className="h-7 w-7 opacity-60" />
            <span>
              {items.length === 0 && !search && selectedKinds.length === 0
                ? t("materialsList.empty")
                : t("materialsList.noMatch")}
            </span>
            {items.length === 0 && !search && selectedKinds.length === 0 && (
              <span>{t("materialsList.emptyHint")}</span>
            )}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 p-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
            {visibleItems.map((material) => {
              const metadata = metadataOf(material);
              const source =
                typeof metadata.source_system === "string"
                  ? metadata.source_system
                  : undefined;
              return (
                <article
                  key={material.id}
                  className="group relative overflow-hidden rounded-2xl border border-border/70 bg-card/80 shadow-sm transition duration-300 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-xl"
                >
                  <button
                    type="button"
                    className="block w-full text-left"
                    onClick={() => onSelect(material.id)}
                  >
                    <div
                      className={`relative aspect-[4/5] overflow-hidden bg-gradient-to-br ${kindStyle(material.asset_kind)}`}
                    >
                      <div className="absolute inset-0 bg-[radial-gradient(circle_at_75%_20%,rgba(255,255,255,0.18),transparent_32%)]" />
                      <div className="absolute inset-0 overflow-hidden">
                        {material.asset_kind === "movement" &&
                        material.object_path ? (
                          <div className="flex h-full items-center justify-center">
                            <Video className="h-10 w-10 text-muted-foreground" />
                          </div>
                        ) : (
                          <LibraryImage
                            organizationId={material.organization_id}
                            objectPath={material.object_path}
                            alt={material.name}
                            className="object-contain p-2 transition duration-500 group-hover:scale-[1.02]"
                            fallbackClassName="bg-transparent"
                          />
                        )}
                      </div>
                      <div className="absolute left-3 top-3 rounded-full border border-white/10 bg-background/70 px-2.5 py-1 text-[11px] font-medium capitalize backdrop-blur-md">
                        {t(`materialsList.kind.${material.asset_kind}`, {
                          defaultValue:
                            material.asset_kind || t("nav.materials"),
                        })}
                      </div>
                      <div className="absolute right-3 top-3">
                        <VisibilityBadge visibility={material.visibility} />
                      </div>
                    </div>

                    <div className="p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <h3 className="truncate font-medium text-foreground">
                            {material.name}
                          </h3>
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            {material.description ||
                              material.object_path ||
                              t("materialsList.fallback")}
                          </p>
                        </div>
                      </div>
                      <div className="mt-4 flex items-center justify-between gap-3 border-t border-border/50 pt-3 text-xs text-muted-foreground">
                        <span className="flex min-w-0 items-center gap-1.5 truncate">
                          <Box className="h-3.5 w-3.5 shrink-0" />
                          {source ||
                            material.scope ||
                            t("materialsList.library")}
                        </span>
                        <span className="flex shrink-0 items-center gap-1.5">
                          <Clock3 className="h-3.5 w-3.5" />
                          {formatTime(material.updated_at, i18n.language)}
                        </span>
                      </div>
                    </div>
                  </button>

                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t("materialsList.delete", {
                      name: material.name,
                    })}
                    className="absolute bottom-2 right-2 h-8 w-8 opacity-0 transition group-hover:opacity-100 focus:opacity-100"
                    disabled={deleteMaterial.isPending}
                    onClick={() => deleteMaterial.mutate(material.id)}
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
