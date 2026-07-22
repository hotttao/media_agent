import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteGeneratedAsset } from "@/domains/generated-assets/lib/api";

// GeneratedAsset is read-only on the frontend (created by media_workflow
// execution), so the only mutation exposed is delete.
export function useDeleteGeneratedAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteGeneratedAsset(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["generated-assets"] });
    },
  });
}
