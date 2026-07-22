import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { useDeleteGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset-mutations";
import { useGeneratedAssetsList } from "@/domains/generated-assets/hooks/use-generated-assets-list";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";

// GeneratedAsset is read-only (created by workflow execution); UI only lists /
// inspects / deletes. No create/edit affordance.
export function GeneratedAssetList({
  onSelect,
}: {
  onSelect: (id: string) => void;
}) {
  const { data, isLoading } = useGeneratedAssetsList();
  const deleteGeneratedAsset = useDeleteGeneratedAsset();

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
        No generated assets yet.
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
            <th className="px-3 py-2">Job</th>
            <th className="px-3 py-2">Created</th>
            <th className="px-3 py-2" aria-label="actions" />
          </tr>
        </thead>
        <tbody>
          {items.map((a) => (
            <tr key={a.id} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(a.id)}
                >
                  {a.name}
                </button>
              </td>
              <td className="px-3 py-2 text-muted-foreground">
                {a.asset_kind}
              </td>
              <td className="px-3 py-2">
                <VisibilityBadge visibility={a.visibility} />
              </td>
              <td className="px-3 py-2 text-muted-foreground">{a.job_id}</td>
              <td className="px-3 py-2 text-muted-foreground">
                {a.created_at}
              </td>
              <td className="px-3 py-2">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="delete"
                  onClick={() => deleteGeneratedAsset.mutate(a.id)}
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
