import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  archiveVirtualIp,
  changeVirtualIpVisibility,
  createVirtualIp,
  deleteVirtualIp,
  grantVirtualIp,
  revokeVirtualIp,
  updateVirtualIp,
} from "@/domains/virtual-ips/lib/api";
import type {
  ChangeVisibilityInput,
  GrantInput,
  VirtualIpInput,
} from "@/domains/virtual-ips/types";

export function useCreateVirtualIp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: VirtualIpInput) => createVirtualIp(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
    },
  });
}

export function useUpdateVirtualIp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      patch,
    }: {
      id: string;
      patch: Partial<VirtualIpInput>;
    }) => updateVirtualIp(id, patch),
    onSuccess: (virtualIp) => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
      qc.invalidateQueries({ queryKey: ["virtual-ip", virtualIp.id] });
    },
  });
}

export function useDeleteVirtualIp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteVirtualIp(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
    },
  });
}

export function useArchiveVirtualIp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveVirtualIp(id),
    onSuccess: (virtualIp) => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
      qc.invalidateQueries({ queryKey: ["virtual-ip", virtualIp.id] });
    },
  });
}

export function useGrantVirtualIp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, grant }: { id: string; grant: GrantInput }) =>
      grantVirtualIp(id, grant),
    onSuccess: (virtualIp) => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
      qc.invalidateQueries({ queryKey: ["virtual-ip", virtualIp.id] });
    },
  });
}

export function useRevokeVirtualIp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, teamId }: { id: string; teamId: string }) =>
      revokeVirtualIp(id, teamId),
    onSuccess: (virtualIp) => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
      qc.invalidateQueries({ queryKey: ["virtual-ip", virtualIp.id] });
    },
  });
}

export function useChangeVirtualIpVisibility() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: ChangeVisibilityInput }) =>
      changeVirtualIpVisibility(id, input),
    onSuccess: (virtualIp) => {
      qc.invalidateQueries({ queryKey: ["virtual-ips"] });
      qc.invalidateQueries({ queryKey: ["virtual-ip", virtualIp.id] });
    },
  });
}
