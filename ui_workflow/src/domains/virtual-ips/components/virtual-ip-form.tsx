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
import { LibraryObjectField } from "@/foundation/components/library-object-field";
import { hostShim } from "@/foundation/lib/host-shim";
import type { PersonaProfile, VirtualIpInput, Visibility } from "../types";

const textFields: Array<[keyof PersonaProfile, string, string?]> = [
  ["education", "学历"],
  ["major", "专业"],
  ["city", "城市"],
  ["occupation", "职业"],
  ["income_level", "收入水平"],
  ["basic_setting", "基础设定"],
  ["family_background", "家庭背景"],
  ["personality", "性格"],
  ["catchphrase", "口头禅"],
  ["small_habit", "小习惯"],
  ["hobbies", "兴趣爱好"],
];
const numberFields: Array<[keyof PersonaProfile, string, string]> = [
  ["age", "年龄", "岁"],
  ["height", "身高", "cm"],
  ["weight", "体重", "kg"],
  ["bust", "胸围", "cm"],
  ["waist", "腰围", "cm"],
  ["hip", "臀围", "cm"],
];

export function VirtualIpForm({
  value,
  onChange,
  onSubmit,
  submitLabel,
  isPending,
  onCancel,
  isEdit,
}: {
  value: VirtualIpInput;
  onChange: (v: VirtualIpInput) => void;
  onSubmit: () => void;
  submitLabel: string;
  isPending: boolean;
  onCancel: () => void;
  isEdit?: boolean;
}) {
  const { organization } = hostShim.useShellContext();
  const [uploads, setUploads] = useState<Record<string, boolean>>({});
  const profile = value.persona_profile ?? {};
  const changeProfile = (patch: Partial<PersonaProfile>) =>
    onChange({ ...value, persona_profile: { ...profile, ...patch } });
  const uploadBusy = Object.values(uploads).some(Boolean);
  const imageFields = [
    [
      "avatar_object_path",
      "virtual_ip_avatar",
      "头像",
      "角色面部识别图，建议使用正方形图片",
    ],
    [
      "full_body_object_path",
      "virtual_ip_full_body",
      "全身照",
      "展示完整人物体态和服饰",
    ],
    [
      "three_view_object_path",
      "virtual_ip_three_view",
      "三视图",
      "正面、侧面、背面参考图",
    ],
    [
      "nine_view_object_path",
      "virtual_ip_nine_view",
      "九宫格",
      "多角度表情与姿态参考图",
    ],
  ] as const;
  return (
    <div className="flex flex-col gap-6">
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">基础信息</h3>
          <p className="text-xs text-muted-foreground">
            Virtual IP 的识别信息和组织内共享范围。
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label>Scope</Label>
            <Input
              disabled={isEdit}
              value={value.scope}
              onChange={(e) => onChange({ ...value, scope: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>名称</Label>
            <Input
              value={value.name}
              onChange={(e) => onChange({ ...value, name: e.target.value })}
            />
          </div>
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
        {!isEdit ? (
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
        ) : null}
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">形象图片</h3>
          <p className="text-xs text-muted-foreground">
            图片会上传到素材存储，业务数据只保存对象路径。
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {imageFields.map(([field, purpose, label, hint]) => (
            <LibraryObjectField
              key={field}
              organizationId={organization.id}
              scope={value.scope}
              purpose={purpose}
              label={label}
              hint={hint}
              value={value[field]}
              onChange={(objectPath) =>
                onChange({ ...value, [field]: objectPath })
              }
              onBusyChange={(busy) =>
                setUploads((old) => ({ ...old, [field]: busy }))
              }
            />
          ))}
        </div>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">人物画像</h3>
          <p className="text-xs text-muted-foreground">
            稳定字段单独录入，供工作流准确读取。
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div className="grid gap-2">
            <Label>性别</Label>
            <Select
              value={profile.gender}
              onValueChange={(gender) =>
                changeProfile({ gender: gender as PersonaProfile["gender"] })
              }
            >
              <SelectTrigger>
                <SelectValue placeholder="选择" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="FEMALE">女</SelectItem>
                <SelectItem value="MALE">男</SelectItem>
                <SelectItem value="OTHER">其他</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {numberFields.map(([field, label, unit]) => (
            <div key={field} className="grid gap-2">
              <Label>
                {label} ({unit})
              </Label>
              <Input
                type="number"
                value={
                  profile[field] === undefined ? "" : String(profile[field])
                }
                onChange={(e) =>
                  changeProfile({
                    [field]:
                      e.target.value === ""
                        ? undefined
                        : Number(e.target.value),
                  })
                }
              />
            </div>
          ))}
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {textFields.map(([field, label]) => (
            <div key={field} className="grid gap-2">
              <Label>{label}</Label>
              <Textarea
                rows={
                  field === "basic_setting" ||
                  field === "family_background" ||
                  field === "personality"
                    ? 4
                    : 2
                }
                value={String(profile[field] ?? "")}
                onChange={(e) => changeProfile({ [field]: e.target.value })}
              />
            </div>
          ))}
        </div>
      </section>
      <div className="flex gap-2">
        <Button
          onClick={onSubmit}
          disabled={isPending || uploadBusy || !value.name.trim()}
        >
          {uploadBusy ? "图片上传中…" : submitLabel}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          取消
        </Button>
      </div>
    </div>
  );
}
