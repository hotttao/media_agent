import { useMaterialsList } from "@/domains/materials/hooks/use-materials-list";
import { useDeleteMaterial } from "@/domains/materials/hooks/use-material-mutations";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { Trash2 } from "lucide-react";

export function MaterialsList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading } = useMaterialsList();
  const deleteMaterial = useDeleteMaterial();

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
        No materials yet.
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-card">
          <tr className="border-b text-left text-xs uppercase text-muted-foreground">
            <th className="px-3 py-2">Name</th>
            <th className="px-3 py-2">Kind</th>
            <th className="px-3 py-2">Visibility</th>
            <th className="px-3 py-2">Updated</th>
            <th className="px-3 py-2" aria-label="actions" />
          </tr>
        </thead>
        <tbody>
          {items.map((m) => (
            <tr key={m.id} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(m.id)}
                >
                  {m.name}
                </button>
              </td>
              <td className="px-3 py-2 text-muted-foreground">{m.asset_kind}</td>
              <td className="px-3 py-2">
                <VisibilityBadge visibility={m.visibility} />
              </td>
              <td className="px-3 py-2 text-muted-foreground">{m.updated_at}</td>
              <td className="px-3 py-2">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="delete"
                  onClick={() => deleteMaterial.mutate(m.id)}
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
