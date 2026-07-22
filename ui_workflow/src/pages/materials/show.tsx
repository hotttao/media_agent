import { useDeleteMaterial } from "@/domains/materials/hooks/use-material-mutations";
import { useMaterial } from "@/domains/materials/hooks/use-material";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

export function MaterialsShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: material, isLoading } = useMaterial(id);
  const deleteMaterial = useDeleteMaterial();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!material) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
        <Button size="sm" onClick={onEdit}>
          Edit
        </Button>
      </div>
      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="col-span-2 font-medium">{material.name}</dd>
        <dt className="text-muted-foreground">Kind</dt>
        <dd className="col-span-2">{material.asset_kind}</dd>
        <dt className="text-muted-foreground">Visibility</dt>
        <dd className="col-span-2">
          <VisibilityBadge visibility={material.visibility} />
        </dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{material.scope}</dd>
        <dt className="text-muted-foreground">URL</dt>
        <dd className="col-span-2 break-all">{material.url}</dd>
        {material.content ? (
          <>
            <dt className="text-muted-foreground">Content</dt>
            <dd className="col-span-2">{material.content}</dd>
          </>
        ) : null}
      </dl>
      <Button
        variant="destructive"
        size="sm"
        onClick={() => {
          deleteMaterial.mutate(id);
          onBack();
        }}
      >
        Delete
      </Button>
    </div>
  );
}
