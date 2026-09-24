import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { useDeleteAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import { useAssetKindsList } from "@/domains/asset-kinds/hooks/use-asset-kinds-list";

export function AssetKindList({
  onSelect,
}: {
  onSelect: (name: string) => void;
}) {
  const { data, isLoading } = useAssetKindsList();
  const deleteAssetKind = useDeleteAssetKind();

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
        No asset kinds yet.
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-card">
          <tr className="border-b text-left text-xs uppercase text-muted-foreground">
            <th className="px-3 py-2">Name</th>
            <th className="px-3 py-2">Resource Type</th>
            <th className="px-3 py-2">Media Type</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2">Updated</th>
            <th className="px-3 py-2" aria-label="actions" />
          </tr>
        </thead>
        <tbody>
          {items.map((k) => (
            <tr key={k.name} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(k.name)}
                >
                  {k.name}
                </button>
              </td>
              <td className="px-3 py-2 text-muted-foreground">
                {k.resource_type}
              </td>
              <td className="px-3 py-2 text-muted-foreground">
                {k.media_type}
              </td>
              <td className="px-3 py-2 text-muted-foreground">{k.status}</td>
              <td className="px-3 py-2 text-muted-foreground">
                {k.updated_at}
              </td>
              <td className="px-3 py-2">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="delete"
                  onClick={() => deleteAssetKind.mutate(k.name)}
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
