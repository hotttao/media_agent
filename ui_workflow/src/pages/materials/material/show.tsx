import { ArrowLeft, Edit3, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useMaterial } from "@/domains/materials/hooks/use-material";
import { useDeleteMaterial } from "@/domains/materials/hooks/use-material-mutations";
import { LibraryImage } from "@/foundation/components/library-image";
import { hostShim } from "@/foundation/lib/host-shim";

function displayJSON(value: Record<string, unknown> | string | undefined) {
  if (!value) return "";
  if (typeof value !== "string") return JSON.stringify(value, null, 2);
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

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
  const { organization } = hostShim.useShellContext();
  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!material)
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  const metadata = displayJSON(material.metadata);
  return (
    <div className="flex h-full flex-col gap-6 overflow-auto p-5">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1 h-4 w-4" />
          返回
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1 h-4 w-4" />
          编辑
        </Button>
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(260px,420px)_1fr]">
        <div className="overflow-hidden rounded-2xl border bg-muted/20">
          <div className="aspect-square">
            <LibraryImage
              organizationId={material.organization_id ?? organization.id}
              objectPath={material.object_path}
              alt={material.name}
              className="object-contain"
            />
          </div>
        </div>
        <div className="grid content-start gap-5">
          <div>
            <div className="mb-2 flex items-center gap-2">
              <span className="rounded-full bg-muted px-2 py-1 text-xs">
                {material.asset_kind}
              </span>
              <VisibilityBadge visibility={material.visibility} />
            </div>
            <h2 className="text-2xl font-semibold">{material.name}</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {material.description || "暂无描述"}
            </p>
          </div>
          <dl className="grid grid-cols-[100px_1fr] gap-x-4 gap-y-3 text-sm">
            <dt className="text-muted-foreground">Scope</dt>
            <dd>{material.scope}</dd>
            <dt className="text-muted-foreground">所有者</dt>
            <dd className="break-all">{material.owner_user_id}</dd>
            <dt className="text-muted-foreground">创建时间</dt>
            <dd>{String(material.created_at)}</dd>
            <dt className="text-muted-foreground">更新时间</dt>
            <dd>{String(material.updated_at)}</dd>
          </dl>
        </div>
      </div>
      {material.prompt ? (
        <section className="rounded-xl border p-4">
          <h3 className="mb-2 font-medium">提示词</h3>
          <p className="whitespace-pre-wrap text-sm">{material.prompt}</p>
        </section>
      ) : null}
      {material.content ? (
        <section className="rounded-xl border p-4">
          <h3 className="mb-2 font-medium">文本内容</h3>
          <p className="whitespace-pre-wrap text-sm">{material.content}</p>
        </section>
      ) : null}
      {metadata && metadata !== "{}" ? (
        <section className="rounded-xl border p-4">
          <h3 className="mb-2 font-medium">Metadata</h3>
          <pre className="overflow-auto rounded-lg bg-muted p-3 text-xs">
            {metadata}
          </pre>
        </section>
      ) : null}
      <Button
        className="self-start"
        variant="destructive"
        size="sm"
        onClick={() => deleteMaterial.mutate(id, { onSuccess: onBack })}
      >
        <Trash2 className="mr-1 h-4 w-4" />
        删除素材
      </Button>
    </div>
  );
}
