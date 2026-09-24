import {
  Archive,
  ArrowLeft,
  BriefcaseBusiness,
  Edit3,
  Heart,
  KeyRound,
  Ruler,
  ShieldCheck,
  Trash2,
  UserRound,
} from "lucide-react";
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
import {
  DetailField,
  DetailFieldGrid,
  DetailPage,
  DetailSection,
  formatDetailDate,
} from "@/foundation/components/detail-layout";
import { LibraryGallery } from "@/foundation/components/library-gallery";
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
function genderLabel(value?: PersonaProfile["gender"]) {
  return value === "FEMALE"
    ? "女"
    : value === "MALE"
      ? "男"
      : value === "OTHER"
        ? "其他"
        : undefined;
}

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
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  if (!virtualIp)
    return (
      <div className="p-6 text-sm text-muted-foreground">
        没有找到这个 Virtual IP。
      </div>
    );
  const profile = profileOf(virtualIp.persona_profile);
  return (
    <DetailPage>
      <div className="sticky top-0 z-10 -mx-1 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border/60 bg-background/90 p-2 shadow-sm backdrop-blur-xl">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          返回 Virtual IP 列表
        </Button>
        <Button size="sm" onClick={onEdit}>
          <Edit3 className="mr-1.5 h-4 w-4" />
          编辑形象
        </Button>
      </div>
      <section className="grid gap-6 lg:grid-cols-[minmax(320px,500px)_1fr] lg:items-start">
        <LibraryGallery
          organizationId={virtualIp.organization_id ?? organization.id}
          items={[
            {
              objectPath: virtualIp.avatar_object_path,
              label: "头像",
              badge: virtualIp.avatar_object_path ? "Avatar" : "尚未上传",
            },
            {
              objectPath: virtualIp.full_body_object_path,
              label: "全身照",
              badge: virtualIp.full_body_object_path ? "Full body" : "尚未上传",
            },
            {
              objectPath: virtualIp.three_view_object_path,
              label: "三视图",
              badge: virtualIp.three_view_object_path
                ? "Three view"
                : "尚未上传",
            },
            {
              objectPath: virtualIp.nine_view_object_path,
              label: "九宫格",
              badge: virtualIp.nine_view_object_path ? "Nine view" : "尚未上传",
            },
          ]}
          alt={virtualIp.name}
          showEmptyItems
        />
        <div className="flex min-w-0 flex-col gap-5 lg:pt-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium capitalize text-primary">
              {virtualIp.status}
            </span>
            <VisibilityBadge visibility={virtualIp.visibility} />
          </div>
          <div>
            <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
              Virtual IP
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              {virtualIp.name}
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
              {virtualIp.description ||
                profile.basic_setting ||
                "这个形象还没有填写简介。"}
            </p>
          </div>
          <DetailFieldGrid className="xl:grid-cols-2">
            <DetailField label="业务范围">{virtualIp.scope}</DetailField>
            <DetailField label="所有者">{virtualIp.owner_user_id}</DetailField>
            <DetailField label="创建时间">
              {formatDetailDate(virtualIp.created_at)}
            </DetailField>
            <DetailField label="最近更新">
              {formatDetailDate(virtualIp.updated_at)}
            </DetailField>
          </DetailFieldGrid>
        </div>
      </section>
      <div className="grid gap-5 xl:grid-cols-2">
        <DetailSection
          title="基本档案"
          description="人物的基础身份与所在地信息"
          icon={UserRound}
        >
          <DetailFieldGrid>
            <DetailField label="性别">
              {genderLabel(profile.gender)}
            </DetailField>
            <DetailField label="年龄">
              {profile.age === undefined ? undefined : `${profile.age} 岁`}
            </DetailField>
            <DetailField label="城市">{profile.city}</DetailField>
            <DetailField label="职业">{profile.occupation}</DetailField>
            <DetailField label="学历">{profile.education}</DetailField>
            <DetailField label="专业">{profile.major}</DetailField>
          </DetailFieldGrid>
        </DetailSection>
        <DetailSection
          title="外形参数"
          description="形象生成和服装适配可参考的数据"
          icon={Ruler}
        >
          <DetailFieldGrid>
            <DetailField label="身高">
              {profile.height === undefined
                ? undefined
                : `${profile.height} cm`}
            </DetailField>
            <DetailField label="体重">
              {profile.weight === undefined
                ? undefined
                : `${profile.weight} kg`}
            </DetailField>
            <DetailField label="胸围">
              {profile.bust === undefined ? undefined : `${profile.bust} cm`}
            </DetailField>
            <DetailField label="腰围">
              {profile.waist === undefined ? undefined : `${profile.waist} cm`}
            </DetailField>
            <DetailField label="臀围">
              {profile.hip === undefined ? undefined : `${profile.hip} cm`}
            </DetailField>
          </DetailFieldGrid>
        </DetailSection>
        <DetailSection
          title="背景设定"
          description="人物经历、家庭与社会背景"
          icon={BriefcaseBusiness}
        >
          <DetailFieldGrid className="sm:grid-cols-1">
            <DetailField label="基础设定">{profile.basic_setting}</DetailField>
            <DetailField label="家庭背景">
              {profile.family_background}
            </DetailField>
            <DetailField label="收入水平">{profile.income_level}</DetailField>
          </DetailFieldGrid>
        </DetailSection>
        <DetailSection
          title="性格与表达"
          description="用于保持角色表达和内容风格一致"
          icon={Heart}
        >
          <DetailFieldGrid className="sm:grid-cols-1">
            <DetailField label="性格">{profile.personality}</DetailField>
            <DetailField label="口头禅">{profile.catchphrase}</DetailField>
            <DetailField label="小习惯">{profile.small_habit}</DetailField>
            <DetailField label="兴趣爱好">{profile.hobbies}</DetailField>
          </DetailFieldGrid>
        </DetailSection>
      </div>
      <DetailSection
        title="权限与共享"
        description="控制该形象在企业和部门内的可见范围"
        icon={ShieldCheck}
      >
        <div className="grid gap-5 lg:grid-cols-2">
          <div className="grid content-start gap-2">
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
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="private">仅自己</SelectItem>
                <SelectItem value="team">部门</SelectItem>
                <SelectItem value="public">组织内公开</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">修改后立即生效。</p>
          </div>
          <div className="grid gap-2">
            <Label>添加部门授权</Label>
            <div className="flex gap-2">
              <Input
                placeholder="输入部门 ID"
                value={teamId}
                onChange={(event) => setTeamId(event.target.value)}
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
                <KeyRound className="mr-1.5 h-4 w-4" />
                授权
              </Button>
            </div>
            {grantedTeams.length ? (
              <div className="mt-2 grid gap-2">
                {grantedTeams.map((team) => (
                  <div
                    key={team}
                    className="flex items-center justify-between rounded-xl border border-border/60 bg-muted/25 px-3 py-2"
                  >
                    <span className="truncate text-sm">{team}</span>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() =>
                        revoke.mutate(
                          { id, teamId: team },
                          {
                            onSuccess: () =>
                              setGrantedTeams((old) =>
                                old.filter((item) => item !== team),
                              ),
                          },
                        )
                      }
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                当前页面还没有新增部门授权。
              </p>
            )}
          </div>
        </div>
      </DetailSection>
      <DetailSection
        title="生命周期管理"
        description="归档会停止继续使用；删除会移除这条形象记录"
        icon={Archive}
        className="border-destructive/20"
      >
        <div className="flex flex-wrap gap-2">
          {virtualIp.status !== "archived" ? (
            <Button
              variant="outline"
              disabled={archive.isPending}
              onClick={() => archive.mutate(id)}
            >
              <Archive className="mr-1.5 h-4 w-4" />
              归档 Virtual IP
            </Button>
          ) : null}
          <Button
            variant="destructive"
            disabled={remove.isPending}
            onClick={() => remove.mutate(id, { onSuccess: onBack })}
          >
            <Trash2 className="mr-1.5 h-4 w-4" />
            删除 Virtual IP
          </Button>
        </div>
      </DetailSection>
    </DetailPage>
  );
}
