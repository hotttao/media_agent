import { useTranslation } from "react-i18next";
import { AppStudioHeader } from "@/foundation/components/app-studio-header";
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
  const { t } = useTranslation();
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
      <AppStudioHeader>
        <h1 className="truncate text-base font-semibold">
          {value.name || submitLabel}
        </h1>{" "}
        <div className="flex gap-2">
          <Button
            onClick={onSubmit}
            disabled={
              isPending ||
              uploadBusy ||
              !value.name.trim() ||
              !value.scope.trim()
            }
          >
            {uploadBusy ? t("materialForm.uploading") : submitLabel}
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
        </div>
      </AppStudioHeader>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">{t("materialForm.basic")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("library.virtualHint")}
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <Label>{t("v12.scope")}</Label>
            <Input
              disabled={isEdit}
              value={value.scope}
              onChange={(e) => onChange({ ...value, scope: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("materialForm.name")}</Label>
            <Input
              aria-label={t("materialForm.name")}
              value={value.name}
              onChange={(e) => onChange({ ...value, name: e.target.value })}
            />
          </div>
        </div>
        <div className="grid gap-2">
          <Label>{t("materialForm.description")}</Label>
          <Textarea
            value={value.description ?? ""}
            onChange={(e) =>
              onChange({ ...value, description: e.target.value })
            }
          />
        </div>
        {!isEdit ? (
          <div className="grid gap-2">
            <Label>{t("materialForm.visibility")}</Label>
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
                <SelectItem value="private">
                  {t("common.visibility.private")}
                </SelectItem>
                <SelectItem value="team">
                  {t("common.visibility.team")}
                </SelectItem>
                <SelectItem value="public">
                  {t("common.visibility.public")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        ) : null}
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">{t("virtualProfile.images")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("virtualProfile.storageHint")}
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {imageFields.map(([field, purpose, label, hint]) => (
            <LibraryObjectField
              key={field}
              organizationId={organization.id}
              scope={value.scope}
              purpose={purpose}
              label={t(`virtualProfile.${field}`, { defaultValue: label })}
              hint={t(`virtualProfile.${field}Hint`, { defaultValue: hint })}
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
          <h3 className="font-medium">{t("virtualProfile.profile")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("virtualProfile.profileHint")}
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div className="grid gap-2">
            <Label>{t("virtualProfile.gender")}</Label>
            <Select
              value={profile.gender}
              onValueChange={(gender) =>
                changeProfile({ gender: gender as PersonaProfile["gender"] })
              }
            >
              <SelectTrigger>
                <SelectValue placeholder={t("virtualProfile.choose")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="FEMALE">
                  {t("virtualProfile.FEMALE")}
                </SelectItem>
                <SelectItem value="MALE">{t("virtualProfile.MALE")}</SelectItem>
                <SelectItem value="OTHER">
                  {t("virtualProfile.OTHER")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          {numberFields.map(([field, label, unit]) => (
            <div key={field} className="grid gap-2">
              <Label>
                {t(`virtualProfile.${field}`, { defaultValue: label })} (
                {unit === "岁" ? t("virtualProfile.years") : unit})
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
              <Label>
                {t(`virtualProfile.${field}`, { defaultValue: label })}
              </Label>
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
    </div>
  );
}
