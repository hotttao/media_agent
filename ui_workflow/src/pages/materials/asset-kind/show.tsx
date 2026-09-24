import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind";
import { useDeleteAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";

export function AssetKindShow({
  name,
  onBack,
  onEdit,
}: {
  name: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: assetKind, isLoading } = useAssetKind(name);
  const deleteAssetKind = useDeleteAssetKind();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!assetKind) {
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
        <dd className="col-span-2 font-medium">{assetKind.name}</dd>
        <dt className="text-muted-foreground">Resource Type</dt>
        <dd className="col-span-2">{assetKind.resource_type}</dd>
        <dt className="text-muted-foreground">Media Type</dt>
        <dd className="col-span-2">{assetKind.media_type}</dd>
        <dt className="text-muted-foreground">Status</dt>
        <dd className="col-span-2">{assetKind.status}</dd>
        {assetKind.source_table ? (
          <>
            <dt className="text-muted-foreground">Source Table</dt>
            <dd className="col-span-2">{assetKind.source_table}</dd>
          </>
        ) : null}
        {assetKind.description ? (
          <>
            <dt className="text-muted-foreground">Description</dt>
            <dd className="col-span-2">{assetKind.description}</dd>
          </>
        ) : null}
        <dt className="text-muted-foreground">Created At</dt>
        <dd className="col-span-2">{assetKind.created_at}</dd>
        <dt className="text-muted-foreground">Updated At</dt>
        <dd className="col-span-2">{assetKind.updated_at}</dd>
      </dl>
      <Button
        variant="destructive"
        size="sm"
        onClick={() => {
          deleteAssetKind.mutate(name);
          onBack();
        }}
      >
        Delete
      </Button>
    </div>
  );
}
