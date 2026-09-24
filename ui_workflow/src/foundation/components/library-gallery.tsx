import { Images } from "lucide-react";
import { useState } from "react";
import { cn } from "@/foundation/lib/utils";
import { LibraryImage } from "./library-image";

export interface LibraryGalleryItem {
  objectPath?: string;
  label: string;
  badge?: string;
}

export function LibraryGallery({
  organizationId,
  items,
  alt,
  className,
  showEmptyItems = false,
}: {
  organizationId?: string;
  items: LibraryGalleryItem[];
  alt: string;
  className?: string;
  showEmptyItems?: boolean;
}) {
  const available = items.filter((item) => item.objectPath);
  const displayItems = showEmptyItems
    ? items
    : available.length
      ? available
      : items.slice(0, 1);
  const [selected, setSelected] = useState(0);
  const safeSelected = Math.min(selected, Math.max(displayItems.length - 1, 0));
  const active = displayItems[safeSelected];
  return (
    <div className={cn("grid gap-3", className)}>
      <div className="group relative aspect-[4/3] overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-br from-muted/70 via-muted/30 to-background shadow-sm">
        <LibraryImage
          organizationId={organizationId}
          objectPath={active?.objectPath}
          alt={`${alt} ${active?.label ?? "图片"}`}
          className="object-contain p-2 transition duration-500 group-hover:scale-[1.015]"
          fallbackClassName="bg-transparent"
        />
        <div className="absolute inset-x-0 bottom-0 flex items-end justify-between bg-gradient-to-t from-black/60 to-transparent px-4 pb-3 pt-10 text-white">
          <div>
            <p className="text-sm font-medium">{active?.label ?? "暂无图片"}</p>
            {active?.badge ? (
              <p className="mt-0.5 text-xs text-white/70">{active.badge}</p>
            ) : null}
          </div>
          <Images className="h-4 w-4 text-white/70" />
        </div>
      </div>
      {displayItems.length > 1 ? (
        <div className="grid grid-cols-4 gap-2">
          {displayItems.map((item, index) => (
            <button
              type="button"
              key={`${item.label}-${item.objectPath}`}
              className={cn(
                "relative aspect-square overflow-hidden rounded-xl border-2 bg-muted/30 transition",
                index === safeSelected
                  ? "border-primary shadow-sm"
                  : "border-transparent opacity-75 hover:opacity-100",
              )}
              onClick={() => setSelected(index)}
            >
              <LibraryImage
                organizationId={organizationId}
                objectPath={item.objectPath}
                alt={`${alt} ${item.label}`}
                className="object-cover"
              />
              <span className="absolute inset-x-1 bottom-1 truncate rounded bg-black/55 px-1.5 py-0.5 text-[10px] text-white">
                {item.label}
              </span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
