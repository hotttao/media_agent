import { hostShim } from "@/foundation/lib/host-shim";
import { useTranslation } from "react-i18next";
import { VirtualIpForm } from "@/domains/virtual-ips/components/virtual-ip-form";
import { useCreateVirtualIp } from "@/domains/virtual-ips/hooks/use-virtual-ip-mutations";
import type { VirtualIpInput } from "@/domains/virtual-ips/types";

export function VirtualIpCreate({
  onDone,
  instanceId,
}: {
  onDone: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const createVirtualIp = useCreateVirtualIp();
  const [value, setValue] = hostShim.useInstanceState<VirtualIpInput>(
    instanceId,
    "materials_virtual-ip_create",
    () => ({
      scope: "",
      name: "",
      visibility: "private",
    }),
  );

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      {createVirtualIp.error ? (
        <p role="alert" className="text-sm text-destructive">
          {createVirtualIp.error.message}
        </p>
      ) : null}
      <h2 className="text-lg font-semibold">{t("nav.virtualIps")}</h2>
      <VirtualIpForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          createVirtualIp.mutate(value, {
            onSuccess: () => {
              setValue({
                scope: "",
                name: "",
                visibility: "private",
              });
              onDone();
            },
          })
        }
        submitLabel={t("common.create")}
        isPending={createVirtualIp.isPending}
        onCancel={onDone}
      />
    </div>
  );
}
