import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createAssetKind,
  deleteAssetKind,
  updateAssetKind,
} from "@/domains/asset-kinds/lib/api";
import type { AssetKindInput } from "@/domains/asset-kinds/types";

export function useCreateAssetKind() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AssetKindInput) => createAssetKind(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["asset-kinds"] });
    },
  });
}

export function useUpdateAssetKind() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, patch }: { name: string; patch: Partial<AssetKindInput> }) =>
      updateAssetKind(name, patch),
    onSuccess: (assetKind) => {
      qc.invalidateQueries({ queryKey: ["asset-kinds"] });
      qc.invalidateQueries({ queryKey: ["asset-kind", assetKind.name] });
    },
  });
}

export function useDeleteAssetKind() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => deleteAssetKind(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["asset-kinds"] });
    },
  });
}
