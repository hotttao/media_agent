import { ArrowLeft, Boxes, GitBranch, ImageIcon, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset";
import { useDeleteGeneratedAsset } from "@/domains/generated-assets/hooks/use-generated-asset-mutations";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
} from "@/foundation/components/detail-layout";

export function GeneratedAssetShow({
  id,
  onBack,
}: {
  id: string;
  onBack: () => void;
}) {
  const { data: asset, isLoading } = useGeneratedAsset(id);
  const remove = useDeleteGeneratedAsset();
  if (isLoading)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!asset)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        没有找到这个生成产物。
      </div>
    );
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex items-center rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          返回生成产物列表
        </Button>
      </div>
      <section className="grid gap-6 lg:grid-cols-[minmax(300px,480px)_1fr] lg:items-start">
        <div className="relative aspect-[4/3] overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-br from-muted/70 via-muted/30 to-background shadow-sm">
          {asset.url ? (
            <img
              src={asset.url}
              alt={asset.name}
              className="h-full w-full object-contain p-2"
            />
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
              <ImageIcon className="h-10 w-10" />
              <span className="text-sm">暂无可预览媒体</span>
            </div>
          )}
          <span className="absolute bottom-3 left-3 rounded-full bg-black/60 px-2.5 py-1 text-xs text-white backdrop-blur">
            {asset.asset_kind}
          </span>
        </div>
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
              Generated
            </span>
            <VisibilityBadge visibility={asset.visibility} />
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              Generated Asset
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {asset.name}
            </h1>
            <p className="mt-3 text-sm leading-7 text-muted-foreground">
              由工作流执行生成的只读产物。
            </p>
          </div>
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label="业务范围">{asset.scope}</DetailField>
            <DetailField label="所有者">{asset.owner_user_id}</DetailField>
            <DetailField label="素材类别">{asset.asset_kind}</DetailField>
            <DetailField label="生成时间">
              {formatDetailDate(asset.created_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      {asset.content ? (
        <DetailSection
          title="文本内容"
          description="工作流生成的文本产物"
          icon={Boxes}
        >
          <p className="whitespace-pre-wrap text-sm leading-7">
            {asset.content}
          </p>
        </DetailSection>
      ) : null}
      <DetailSection
        title="生成来源"
        description="用于追溯产物对应的工作流、步骤和输入对象"
        icon={GitBranch}
      >
        <DetailFieldGrid>
          <DetailField label="Job">{asset.job_id}</DetailField>
          <DetailField label="Step Run">{asset.step_run_id}</DetailField>
          <DetailField label="Template">{asset.template_id}</DetailField>
          <DetailField label="模板版本">v{asset.template_version}</DetailField>
          <DetailField label="来源 Virtual IP">
            {asset.source_virtual_ip_id}
          </DetailField>
          <DetailField label="来源 Product">
            {asset.source_product_id}
          </DetailField>
        </DetailFieldGrid>
      </DetailSection>
      <DetailSection
        title="产物管理"
        description="删除只会移除产物记录，不会修改原始工作流执行记录"
        icon={Boxes}
        className="border-destructive/20"
      >
        <Button
          variant="destructive"
          size="sm"
          disabled={remove.isPending}
          onClick={() => remove.mutate(id, { onSuccess: onBack })}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          删除产物
        </Button>
      </DetailSection>
    </DetailPage>
  );
}
