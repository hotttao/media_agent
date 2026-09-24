import { ArrowLeft, Database, Edit3, Layers3, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind";
import { useDeleteAssetKind } from "@/domains/asset-kinds/hooks/use-asset-kind-mutations";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
} from "@/foundation/components/detail-layout";

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
  const remove = useDeleteAssetKind();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!assetKind)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        没有找到这个素材类别。
      </div>
    );
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex items-center justify-between rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          返回类别列表
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1.5 h-4 w-4" />
          编辑类别
        </Button>
      </div>
      <section className="rounded-2xl border border-border/70 bg-gradient-to-br from-primary/10 via-card to-card p-6 shadow-sm sm:p-8">
        <div className="flex items-start gap-4">
          <div className="rounded-2xl bg-primary/10 p-3 text-primary">
            <Layers3 className="h-7 w-7" />
          </div>
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap gap-2">
              <span className="rounded-full bg-background/80 px-2.5 py-1 text-xs font-medium">
                {assetKind.resource_type}
              </span>
              <span className="rounded-full bg-background/80 px-2.5 py-1 text-xs font-medium">
                {assetKind.media_type}
              </span>
              <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
                {assetKind.status}
              </span>
            </div>
            <h1 className="break-words text-3xl font-semibold tracking-tight sm:text-4xl">
              {assetKind.name}
            </h1>
            <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground sm:text-base">
              {assetKind.description || "这个素材类别还没有填写说明。"}
            </p>
          </div>
        </div>
      </section>
      <DetailSection
        title="类别定义"
        description="该类别决定资源落表位置和媒体处理方式"
        icon={Database}
      >
        <DetailFieldGrid>
          <DetailField label="资源类型">{assetKind.resource_type}</DetailField>
          <DetailField label="媒体类型">{assetKind.media_type}</DetailField>
          <DetailField label="来源表">
            {assetKind.source_table || "无独立数据表"}
          </DetailField>
          <DetailField label="状态">{assetKind.status}</DetailField>
          <DetailField label="创建时间">
            {formatDetailDate(assetKind.created_at)}
          </DetailField>
          <DetailField label="最近更新">
            {formatDetailDate(assetKind.updated_at)}
          </DetailField>
        </DetailFieldGrid>
      </DetailSection>
      <DetailSection
        title="类别管理"
        description={
          assetKind.status === "SYSTEM"
            ? "系统类别由平台维护，不能删除。"
            : "使用中的类别可能会被服务端拒绝删除。"
        }
        icon={Layers3}
        className="border-destructive/20"
      >
        <Button
          variant="destructive"
          size="sm"
          disabled={remove.isPending || assetKind.status === "SYSTEM"}
          onClick={() => remove.mutate(name, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          删除类别
        </Button>
      </DetailSection>
    </DetailPage>
  );
}
