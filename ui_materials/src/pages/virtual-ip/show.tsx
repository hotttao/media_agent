import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function VirtualIpShowPage() {
  return (
    <ResourcePageScaffold
      resourceKey="virtualIp"
      mode="show"
      listPath="/virtual-ips"
    />
  );
}
