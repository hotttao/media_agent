import { useQuery } from "@tanstack/react-query";
import { getVirtualIp } from "@/domains/virtual-ips/lib/api";

export function useVirtualIp(id: string | null | undefined) {
  return useQuery({
    queryKey: ["virtual-ip", id],
    queryFn: () => getVirtualIp(id as string),
    enabled: !!id,
  });
}
