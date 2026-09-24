import { useEffect, useState } from "react";
import { Spinner } from "@/components/ui/spinner";
import { VirtualIpForm } from "@/domains/virtual-ips/components/virtual-ip-form";
import { useVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip";
import { useUpdateVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import type {
  PersonaProfile,
  VirtualIpInput,
} from "@/domains/virtual-ips/types";

function editableProfile(
  value: PersonaProfile | string | undefined,
): PersonaProfile | undefined {
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as PersonaProfile;
  } catch {
    return undefined;
  }
}

export function VirtualIpEdit({
  id,
  onDone,
}: {
  id: string;
  onDone: () => void;
}) {
  const { data: virtualIp, isLoading } = useVirtualIp(id);
  const updateVirtualIp = useUpdateVirtualIp();
  const [value, setValue] = useState<VirtualIpInput | null>(null);

  useEffect(() => {
    if (virtualIp) {
      setValue({
        scope: virtualIp.scope,
        name: virtualIp.name,
        description: virtualIp.description,
        avatar_object_path: virtualIp.avatar_object_path,
        full_body_object_path: virtualIp.full_body_object_path,
        three_view_object_path: virtualIp.three_view_object_path,
        nine_view_object_path: virtualIp.nine_view_object_path,
        persona_profile: editableProfile(virtualIp.persona_profile),
        visibility: virtualIp.visibility,
      });
    }
  }, [virtualIp]);

  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      <h2 className="text-lg font-semibold">Edit Virtual IP</h2>
      <VirtualIpForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateVirtualIp.mutate({ id, patch: value }, { onSuccess: onDone })
        }
        submitLabel="Save"
        isPending={updateVirtualIp.isPending}
        onCancel={onDone}
        isEdit
      />
    </div>
  );
}
