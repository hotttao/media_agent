import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function VirtualIpCreatePage() {
  return (
    <ResourcePageScaffold
      resourceKey="virtualIp"
      mode="create"
      listPath="/virtual-ips"
    />
  );
}
