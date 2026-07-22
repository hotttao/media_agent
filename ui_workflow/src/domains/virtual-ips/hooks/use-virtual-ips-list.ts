import { useQuery } from "@tanstack/react-query";
import { listVirtualIps } from "@/domains/virtual-ips/lib/api";
import type { VirtualIpListParams } from "@/domains/virtual-ips/types";

export function useVirtualIpsList(params: VirtualIpListParams = {}) {
  return useQuery({
    queryKey: ["virtual-ips", params],
    queryFn: () => listVirtualIps(params),
  });
}
