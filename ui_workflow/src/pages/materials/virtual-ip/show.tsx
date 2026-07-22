import { Trash2 } from "lucide-react";
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
import type { Visibility } from "@/domains/virtual-ips/types";

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
  const deleteVirtualIp = useDeleteVirtualIp();
  const archiveVirtualIp = useArchiveVirtualIp();
  const changeVisibility = useChangeVirtualIpVisibility();
  const grantVirtualIp = useGrantVirtualIp();
  const revokeVirtualIp = useRevokeVirtualIp();
  const [teamId, setTeamId] = useState("");
  // Backend toView does not populate grants on the VirtualIp response; track
  // granted team_ids locally so the revoke UI is functional within a session.
  const [grantedTeams, setGrantedTeams] = useState<string[]>([]);

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!virtualIp) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
        <Button size="sm" onClick={onEdit}>
          Edit
        </Button>
      </div>
      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="col-span-2 font-medium">{virtualIp.name}</dd>
        <dt className="text-muted-foreground">Status</dt>
        <dd className="col-span-2">{virtualIp.status}</dd>
        <dt className="text-muted-foreground">Visibility</dt>
        <dd className="col-span-2">
          <VisibilityBadge visibility={virtualIp.visibility} />
        </dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{virtualIp.scope}</dd>
        {virtualIp.description ? (
          <>
            <dt className="text-muted-foreground">Description</dt>
            <dd className="col-span-2">{virtualIp.description}</dd>
          </>
        ) : null}
        {virtualIp.avatar_url ? (
          <>
            <dt className="text-muted-foreground">Avatar URL</dt>
            <dd className="col-span-2 break-all">{virtualIp.avatar_url}</dd>
          </>
        ) : null}
        {virtualIp.full_body_url ? (
          <>
            <dt className="text-muted-foreground">Full Body URL</dt>
            <dd className="col-span-2 break-all">{virtualIp.full_body_url}</dd>
          </>
        ) : null}
        {virtualIp.three_view_url ? (
          <>
            <dt className="text-muted-foreground">Three View URL</dt>
            <dd className="col-span-2 break-all">{virtualIp.three_view_url}</dd>
          </>
        ) : null}
        {virtualIp.nine_view_url ? (
          <>
            <dt className="text-muted-foreground">Nine View URL</dt>
            <dd className="col-span-2 break-all">{virtualIp.nine_view_url}</dd>
          </>
        ) : null}
      </dl>

      {/* Archive (only when active; archived is terminal) */}
      {virtualIp.status !== "archived" ? (
        <Button
          variant="secondary"
          size="sm"
          onClick={() => archiveVirtualIp.mutate(id)}
          disabled={archiveVirtualIp.isPending}
        >
          Archive
        </Button>
      ) : null}

      {/* Visibility switch (owner-only by backend; UI offers the control) */}
      <div className="grid gap-2">
        <Label>Change Visibility</Label>
        <Select
          value={virtualIp.visibility}
          onValueChange={(v) =>
            changeVisibility.mutate({
              id,
              input: { visibility: v as Visibility },
            })
          }
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="private">private</SelectItem>
            <SelectItem value="team">team</SelectItem>
            <SelectItem value="public">public</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Grant / Revoke team management */}
      <div className="flex flex-col gap-2">
        <Label>Team Grants</Label>
        <div className="flex gap-2">
          <Input
            placeholder="team_id"
            value={teamId}
            onChange={(e) => setTeamId(e.target.value)}
          />
          <Button
            size="sm"
            onClick={() => {
              if (!teamId) return;
              grantVirtualIp.mutate(
                { id, grant: { team_id: teamId } },
                {
                  onSuccess: () => {
                    setGrantedTeams((prev) =>
                      prev.includes(teamId) ? prev : [...prev, teamId],
                    );
                    setTeamId("");
                  },
                },
              );
            }}
            disabled={!teamId || grantVirtualIp.isPending}
          >
            Grant
          </Button>
        </div>
        {grantedTeams.length > 0 ? (
          <ul className="flex flex-col gap-1 text-sm">
            {grantedTeams.map((tid) => (
              <li
                key={tid}
                className="flex items-center justify-between rounded border px-2 py-1"
              >
                <span className="font-mono">{tid}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="revoke"
                  onClick={() =>
                    revokeVirtualIp.mutate(
                      { id, teamId: tid },
                      {
                        onSuccess: () => {
                          setGrantedTeams((prev) =>
                            prev.filter((t) => t !== tid),
                          );
                        },
                      },
                    )
                  }
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-xs text-muted-foreground">No grants.</p>
        )}
      </div>

      <Button
        variant="destructive"
        size="sm"
        onClick={() => {
          deleteVirtualIp.mutate(id);
          onBack();
        }}
      >
        Delete
      </Button>
    </div>
  );
}
