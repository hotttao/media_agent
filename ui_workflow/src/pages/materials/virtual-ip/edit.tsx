import { useEffect, useState } from "react";
import { Spinner } from "@/components/ui/spinner";
import { VirtualIpForm } from "@/domains/virtual-ips/components/virtual-ip-form";
import { useVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip";
import { useUpdateVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import type { VirtualIpInput } from "@/domains/virtual-ips/types";

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
        avatar_url: virtualIp.avatar_url,
        full_body_url: virtualIp.full_body_url,
        three_view_url: virtualIp.three_view_url,
        nine_view_url: virtualIp.nine_view_url,
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
    <div className="flex flex-col gap-4 p-4">
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
      />
    </div>
  );
}
