import { ResourcePageScaffold } from "@/foundation/components/resource-page-scaffold";

export function VirtualIpListPage() {
  return (
    <ResourcePageScaffold
      resourceKey="virtualIp"
      mode="list"
      listPath="/virtual-ips"
      createPath="/virtual-ips/new"
    />
  );
}
