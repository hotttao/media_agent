import { ArrowLeft, Edit3, Trash2 } from "lucide-react";
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
import { Spinner } from "@/components/ui/spinner";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { useVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip";
import {
  useArchiveVirtualIp,
  useChangeVirtualIpVisibility,
  useDeleteVirtualIp,
  useGrantVirtualIp,
  useRevokeVirtualIp,
} from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import type { PersonaProfile, Visibility } from "@/domains/virtual-ips/types";
import { LibraryImage } from "@/foundation/components/library-image";
import { hostShim } from "@/foundation/lib/host-shim";

function profileOf(value: PersonaProfile | string | undefined): PersonaProfile {
  if (!value) return {};
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as PersonaProfile;
  } catch {
    return {};
  }
}
const profileLabels: Array<[keyof PersonaProfile, string, string?]> = [
  ["gender", "性别"],
  ["age", "年龄", "岁"],
  ["height", "身高", "cm"],
  ["weight", "体重", "kg"],
  ["bust", "胸围", "cm"],
  ["waist", "腰围", "cm"],
  ["hip", "臀围", "cm"],
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

export function VirtualIpShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: virtualIp, isLoading } = useVirtualIp(id);
  const remove = useDeleteVirtualIp();
  const archive = useArchiveVirtualIp();
  const changeVisibility = useChangeVirtualIpVisibility();
  const grant = useGrantVirtualIp();
  const revoke = useRevokeVirtualIp();
  const { organization } = hostShim.useShellContext();
  const [teamId, setTeamId] = useState("");
  const [grantedTeams, setGrantedTeams] = useState<string[]>([]);
  if (isLoading)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!virtualIp)
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  const profile = profileOf(virtualIp.persona_profile);
  const pictures = [
    [virtualIp.avatar_object_path, "头像"],
    [virtualIp.full_body_object_path, "全身照"],
    [virtualIp.three_view_object_path, "三视图"],
    [virtualIp.nine_view_object_path, "九宫格"],
  ] as const;
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
      <div className="grid gap-6 lg:grid-cols-[minmax(320px,520px)_1fr]">
        <div className="grid grid-cols-2 gap-2">
          {pictures.map(([path, label], index) => (
            <div
              key={label}
              className={`relative overflow-hidden rounded-xl border bg-muted/20 ${index === 0 ? "col-span-2 aspect-[4/3]" : "aspect-square"}`}
            >
              <LibraryImage
                organizationId={virtualIp.organization_id ?? organization.id}
                objectPath={path}
                alt={`${virtualIp.name} ${label}`}
                className="object-contain"
              />
              <span className="absolute bottom-2 left-2 rounded-full bg-background/80 px-2 py-1 text-xs backdrop-blur">
                {label}
              </span>
            </div>
          ))}
        </div>
        <div className="grid content-start gap-4">
          <div className="flex gap-2">
            <span className="rounded-full bg-muted px-2 py-1 text-xs">
              {virtualIp.status}
            </span>
            <VisibilityBadge visibility={virtualIp.visibility} />
          </div>
          <div>
            <h2 className="text-2xl font-semibold">{virtualIp.name}</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {virtualIp.description || "暂无描述"}
            </p>
          </div>
          <dl className="grid grid-cols-[90px_1fr] gap-2 text-sm">
            <dt className="text-muted-foreground">Scope</dt>
            <dd>{virtualIp.scope}</dd>
            <dt className="text-muted-foreground">所有者</dt>
            <dd className="break-all">{virtualIp.owner_user_id}</dd>
            <dt className="text-muted-foreground">创建时间</dt>
            <dd>{String(virtualIp.created_at)}</dd>
            <dt className="text-muted-foreground">更新时间</dt>
            <dd>{String(virtualIp.updated_at)}</dd>
          </dl>
        </div>
      </div>
      <section className="rounded-xl border p-4">
        <h3 className="mb-4 font-medium">人物画像</h3>
        <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
          {profileLabels.map(([field, label, unit]) =>
            profile[field] !== undefined && profile[field] !== "" ? (
              <div key={field}>
                <dt className="text-xs text-muted-foreground">{label}</dt>
                <dd className="mt-1 whitespace-pre-wrap text-sm">
                  {String(profile[field])}
                  {unit ? ` ${unit}` : ""}
                </dd>
              </div>
            ) : null,
          )}
        </dl>
      </section>
      <section className="grid gap-4 rounded-xl border p-4">
        <div>
          <h3 className="font-medium">权限与生命周期</h3>
          <p className="text-xs text-muted-foreground">
            共享范围仍使用现有 visibility 与部门授权模型。
          </p>
        </div>
        <div className="grid gap-2">
          <Label>可见范围</Label>
          <Select
            value={virtualIp.visibility}
            onValueChange={(visibility) =>
              changeVisibility.mutate({
                id,
                input: { visibility: visibility as Visibility },
              })
            }
          >
            <SelectTrigger className="max-w-xs">
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
          <Label>部门授权</Label>
          <div className="flex max-w-xl gap-2">
            <Input
              placeholder="team_id"
              value={teamId}
              onChange={(e) => setTeamId(e.target.value)}
            />
            <Button
              disabled={!teamId || grant.isPending}
              onClick={() =>
                grant.mutate(
                  { id, grant: { team_id: teamId } },
                  {
                    onSuccess: () => {
                      setGrantedTeams((old) =>
                        old.includes(teamId) ? old : [...old, teamId],
                      );
                      setTeamId("");
                    },
                  },
                )
              }
            >
              授权
            </Button>
          </div>
          {grantedTeams.map((team) => (
            <div
              key={team}
              className="flex max-w-xl items-center justify-between rounded border px-3 py-2 text-sm"
            >
              <span className="font-mono">{team}</span>
              <Button
                variant="ghost"
                size="icon"
                onClick={() =>
                  revoke.mutate(
                    { id, teamId: team },
                    {
                      onSuccess: () =>
                        setGrantedTeams((old) => old.filter((x) => x !== team)),
                    },
                  )
                }
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
        {virtualIp.status !== "archived" ? (
          <Button
            className="self-start"
            variant="outline"
            onClick={() => archive.mutate(id)}
            disabled={archive.isPending}
          >
            归档 Virtual IP
          </Button>
        ) : null}
      </section>
      <Button
        className="self-start"
        variant="destructive"
        size="sm"
        onClick={() => remove.mutate(id, { onSuccess: onBack })}
      >
        <Trash2 className="mr-1 h-4 w-4" />
        删除 Virtual IP
      </Button>
    </div>
  );
}
