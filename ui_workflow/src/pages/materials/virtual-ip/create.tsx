import { useState } from "react";
import { VirtualIpForm } from "@/domains/virtual-ips/components/virtual-ip-form";
import { useCreateVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import type { VirtualIpInput } from "@/domains/virtual-ips/types";

export function VirtualIpCreate({ onDone }: { onDone: () => void }) {
  const createVirtualIp = useCreateVirtualIp();
  const [value, setValue] = useState<VirtualIpInput>({
    scope: "",
    name: "",
    visibility: "private",
  });

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      <h2 className="text-lg font-semibold">New Virtual IP</h2>
      <VirtualIpForm
        value={value}
        onChange={setValue}
        onSubmit={() => createVirtualIp.mutate(value, { onSuccess: onDone })}
        submitLabel="Create"
        isPending={createVirtualIp.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
