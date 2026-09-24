import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useAssetKinds } from "@/domains/materials/hooks/use-asset-kinds";
import type { MaterialInput, Visibility } from "@/domains/materials/types";
import { LibraryObjectField } from "@/foundation/components/library-object-field";
import { hostShim } from "@/foundation/lib/host-shim";

function jsonText(value?: Record<string, unknown>) {
  return value && Object.keys(value).length
    ? JSON.stringify(value, null, 2)
    : "";
}

export function MaterialForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  isEdit,
}: {
  value: MaterialInput;
  onChange: (v: MaterialInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  isEdit?: boolean;
}) {
  const { organization } = hostShim.useShellContext();
  const { data: kinds } = useAssetKinds("MATERIAL");
  const [uploading, setUploading] = useState(false);
  const [metadataText, setMetadataText] = useState(() =>
    jsonText(value.metadata),
  );
  const [metadataError, setMetadataError] = useState<string>();

  function changeMetadata(text: string) {
    setMetadataText(text);
    if (!text.trim()) {
      setMetadataError(undefined);
      onChange({ ...value, metadata: undefined });
      return;
    }
    try {
      const parsed = JSON.parse(text) as Record<string, unknown>;
      setMetadataError(undefined);
      onChange({ ...value, metadata: parsed });
    } catch (error) {
      setMetadataError(
        error instanceof Error ? error.message : "JSON 格式错误",
      );
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">基础信息</h3>
          <p className="text-xs text-muted-foreground">
            用于检索、分类和描述素材。
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label>Scope</Label>
            <Input
              value={value.scope}
              disabled={isEdit}
              onChange={(e) => onChange({ ...value, scope: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>资源类别</Label>
            <Select
              disabled={isEdit}
              value={value.asset_kind}
              onValueChange={(asset_kind) => onChange({ ...value, asset_kind })}
            >
              <SelectTrigger>
                <SelectValue placeholder="选择类别" />
              </SelectTrigger>
              <SelectContent>
                {(kinds ?? []).map((kind) => (
                  <SelectItem key={kind.name} value={kind.name}>
                    {kind.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="grid gap-2">
          <Label>名称</Label>
          <Input
            value={value.name}
            onChange={(e) => onChange({ ...value, name: e.target.value })}
          />
        </div>
        <div className="grid gap-2">
          <Label>描述</Label>
          <Textarea
            value={value.description ?? ""}
            onChange={(e) =>
              onChange({ ...value, description: e.target.value })
            }
          />
        </div>
        <div className="grid gap-2">
          <Label>提示词</Label>
          <Textarea
            value={value.prompt ?? ""}
            onChange={(e) => onChange({ ...value, prompt: e.target.value })}
            placeholder="生成或使用该素材时的提示词"
          />
        </div>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">素材内容</h3>
          <p className="text-xs text-muted-foreground">
            图片直接上传；文本类素材填写内容。至少提供其中一种。
          </p>
        </div>
        <LibraryObjectField
          organizationId={organization.id}
          scope={value.scope}
          purpose="material"
          label="素材图片"
          value={value.object_path}
          onChange={(object_path) => onChange({ ...value, object_path })}
          onBusyChange={setUploading}
          className="max-w-xl"
        />
        <div className="grid gap-2">
          <Label>文本内容</Label>
          <Textarea
            rows={6}
            value={value.content ?? ""}
            onChange={(e) => onChange({ ...value, content: e.target.value })}
          />
        </div>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <h3 className="font-medium">共享与扩展信息</h3>
        <div className="grid gap-2">
          <Label>可见范围</Label>
          <Select
            value={value.visibility ?? "private"}
            onValueChange={(visibility) =>
              onChange({ ...value, visibility: visibility as Visibility })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="private">仅自己</SelectItem>
              <SelectItem value="team">部门</SelectItem>
              <SelectItem value="public">组织内公开</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label>Metadata (JSON)</Label>
          <Textarea
            className="font-mono text-xs"
            rows={6}
            value={metadataText}
            onChange={(e) => changeMetadata(e.target.value)}
            placeholder="{}"
          />
          {metadataError ? (
            <p className="text-xs text-destructive">{metadataError}</p>
          ) : null}
        </div>
      </section>
      <div className="flex gap-2">
        <Button
          onClick={onSubmit}
          disabled={
            isPending ||
            uploading ||
            Boolean(metadataError) ||
            !value.name.trim() ||
            (!value.object_path && !value.content?.trim())
          }
        >
          {uploading ? "图片上传中…" : submitLabel}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          取消
        </Button>
      </div>
    </div>
  );
}
