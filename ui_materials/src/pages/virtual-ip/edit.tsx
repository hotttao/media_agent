import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function VirtualIpEditPage() {
  return (
    <ResourcePageScaffold
      resourceKey="virtualIp"
      mode="edit"
      listPath="/virtual-ips"
    />
  );
}
