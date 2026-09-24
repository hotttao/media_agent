import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset";
import { useDeleteGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset-mutations";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";

// GeneratedAsset is read-only (created by workflow execution); show view only
// has Back + Delete (no Edit). Generation provenance (job_id / step_run_id /
// template_id / template_version / source_virtual_ip_id / source_product_id)
// is rendered in a dedicated section.
export function GeneratedAssetShow({
  id,
  onBack,
}: {
  id: string;
  onBack: () => void;
}) {
  const { data: asset, isLoading } = useGeneratedAsset(id);
  const deleteGeneratedAsset = useDeleteGeneratedAsset();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!asset) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
      </div>
      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="col-span-2 font-medium">{asset.name}</dd>
        <dt className="text-muted-foreground">Kind</dt>
        <dd className="col-span-2">{asset.asset_kind}</dd>
        <dt className="text-muted-foreground">Visibility</dt>
        <dd className="col-span-2">
          <VisibilityBadge visibility={asset.visibility} />
        </dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{asset.scope}</dd>
        <dt className="text-muted-foreground">URL</dt>
        <dd className="col-span-2 break-all">{asset.url}</dd>
        {asset.content ? (
          <>
            <dt className="text-muted-foreground">Content</dt>
            <dd className="col-span-2">{asset.content}</dd>
          </>
        ) : null}
        <dt className="text-muted-foreground">Owner</dt>
        <dd className="col-span-2">{asset.owner_user_id}</dd>
        <dt className="text-muted-foreground">Created</dt>
        <dd className="col-span-2">{asset.created_at}</dd>
      </dl>
      {/* Generation provenance (immutable; populated by workflow execution) */}
      <div className="rounded-md border border-foreground/[0.06] p-3">
        <h3 className="mb-2 text-xs uppercase text-muted-foreground">
          Generation Source
        </h3>
        <dl className="grid grid-cols-3 gap-2 text-sm">
          <dt className="text-muted-foreground">Job</dt>
          <dd className="col-span-2">{asset.job_id}</dd>
          <dt className="text-muted-foreground">Step Run</dt>
          <dd className="col-span-2">{asset.step_run_id}</dd>
          <dt className="text-muted-foreground">Template</dt>
          <dd className="col-span-2">
            {asset.template_id}
            <span className="text-muted-foreground">
              {" "}
              (v{asset.template_version})
            </span>
          </dd>
          {asset.source_virtual_ip_id ? (
            <>
              <dt className="text-muted-foreground">Source Virtual IP</dt>
              <dd className="col-span-2">{asset.source_virtual_ip_id}</dd>
            </>
          ) : null}
          {asset.source_product_id ? (
            <>
              <dt className="text-muted-foreground">Source Product</dt>
              <dd className="col-span-2">{asset.source_product_id}</dd>
            </>
          ) : null}
        </dl>
      </div>
      <Button
        variant="destructive"
        size="sm"
        onClick={() => {
          deleteGeneratedAsset.mutate(id);
          onBack();
        }}
      >
        Delete
      </Button>
    </div>
  );
}
