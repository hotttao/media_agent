import { hostShim } from "@/foundation/lib/host-shim";
import { useTranslation } from "react-i18next";
import { ListState } from "@/foundation/components/resource-list-layout";
import { useEffect } from "react";
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
  instanceId,
}: {
  id: string;
  onDone: () => void;
  instanceId: string;
}) {
  const { t } = useTranslation();
  const {
    data: virtualIp,
    isLoading,
    isError,
    error,
    refetch,
  } = useVirtualIp(id);
  const updateVirtualIp = useUpdateVirtualIp();
  const [value, setValue] = hostShim.useInstanceState<VirtualIpInput | null>(
    instanceId,
    "materials_virtual-ip_draft_" + id,
    () => null,
  );

  useEffect(() => {
    if (virtualIp) {
      setValue(
        (current) =>
          current ?? {
            scope: virtualIp.scope,
            name: virtualIp.name,
            description: virtualIp.description,
            avatar_object_path: virtualIp.avatar_object_path,
            full_body_object_path: virtualIp.full_body_object_path,
            three_view_object_path: virtualIp.three_view_object_path,
            nine_view_object_path: virtualIp.nine_view_object_path,
            persona_profile: editableProfile(virtualIp.persona_profile),
            visibility: virtualIp.visibility,
          },
      );
    }
  }, [virtualIp, setValue]);

  if (isError)
    return <ListState error={error.message} onRetry={() => void refetch()} />;
  if (isLoading || !value) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      {updateVirtualIp.error ? (
        <p role="alert" className="text-sm text-destructive">
          {updateVirtualIp.error.message}
        </p>
      ) : null}
      <h2 className="text-lg font-semibold">{t("nav.virtualIps")}</h2>
      <VirtualIpForm
        value={value}
        onChange={setValue}
        onSubmit={() =>
          updateVirtualIp.mutate(
            { id, patch: value },
            {
              onSuccess: () => {
                setValue(null);
                onDone();
              },
            },
          )
        }
        submitLabel={t("common.save")}
        isPending={updateVirtualIp.isPending}
        onCancel={onDone}
        isEdit
      />
    </div>
  );
}
