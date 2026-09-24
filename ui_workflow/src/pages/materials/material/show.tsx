import {
  ArrowLeft,
  Braces,
  Edit3,
  FileImage,
  Info,
  Sparkles,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useMaterial } from "@/domains/materials/hooks/use-material";
import { useDeleteMaterial } from "@/domains/materials/hooks/use-material-mutations";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
  StructuredDetails,
} from "@/foundation/components/detail-layout";
import { LibraryGallery } from "@/foundation/components/library-gallery";
import { hostShim } from "@/foundation/lib/host-shim";

function recordOf(
  value: Record<string, unknown> | string | undefined,
): Record<string, unknown> {
  if (!value) return {};
  if (typeof value !== "string") return value;
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return { value };
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
  const remove = useDeleteMaterial();
  const { organization } = hostShim.useShellContext();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!material)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        没有找到这个素材。
      </div>
    );
  const metadata = recordOf(material.metadata);
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex items-center justify-between rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          返回素材列表
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1.5 h-4 w-4" />
          编辑素材
        </Button>
      </div>
      <section className="grid gap-6 lg:grid-cols-[minmax(300px,460px)_1fr] lg:items-start">
        <LibraryGallery
          organizationId={material.organization_id ?? organization.id}
          items={[
            {
              objectPath: material.object_path,
              label: material.asset_kind || "素材图片",
            },
          ]}
          alt={material.name}
        />
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
              {material.asset_kind}
            </span>
            <VisibilityBadge visibility={material.visibility} />
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              Material
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {material.name}
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
              {material.description || "这个素材还没有填写描述。"}
            </p>
          </div>
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label="业务范围">{material.scope}</DetailField>
            <DetailField label="所有者">{material.owner_user_id}</DetailField>
            <DetailField label="创建时间">
              {formatDetailDate(material.created_at)}
            </DetailField>
            <DetailField label="最近更新">
              {formatDetailDate(material.updated_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      <div className="grid gap-5 lg:grid-cols-2">
        {material.prompt ? (
          <DetailSection
            title="生成提示词"
            description="工作流引用素材时可使用的创作说明"
            icon={Sparkles}
          >
            <p className="whitespace-pre-wrap text-sm leading-7">
              {material.prompt}
            </p>
          </DetailSection>
        ) : null}
        {material.content ? (
          <DetailSection
            title="文本内容"
            description="素材直接承载的文本信息"
            icon={Info}
          >
            <p className="whitespace-pre-wrap text-sm leading-7">
              {material.content}
            </p>
          </DetailSection>
        ) : null}
      </div>
      {Object.keys(metadata).length ? (
        <DetailSection
          title="扩展属性"
          description="尚未进入稳定业务字段的补充信息"
          icon={Braces}
        >
          <StructuredDetails value={metadata} />
        </DetailSection>
      ) : null}
      <DetailSection
        title="素材管理"
        description="删除后该素材将不再能被新的工作流选择"
        icon={FileImage}
        className="border-destructive/20"
      >
        <Button
          variant="destructive"
          size="sm"
          disabled={remove.isPending}
          onClick={() => remove.mutate(id, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          删除素材
        </Button>
      </DetailSection>
    </DetailPage>
  );
}
