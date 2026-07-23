import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  archiveWorkflow,
  createWorkflow,
  updateWorkflow,
  validateWorkflow,
} from "@/domains/workflows/lib/api";
import type { ValidationResult, WorkflowInput } from "@/domains/workflows/types";

export function useCreateWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: WorkflowInput) => createWorkflow(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workflows"] }),
  });
}

export function useUpdateWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<WorkflowInput> }) =>
      updateWorkflow(id, patch),
    onSuccess: (workflow) => {
      qc.invalidateQueries({ queryKey: ["workflows"] });
      qc.invalidateQueries({ queryKey: ["workflow", workflow.id] });
    },
  });
}

export function useArchiveWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveWorkflow(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workflows"] }),
  });
}

export function useValidateWorkflow() {
  return useMutation({
    mutationFn: (input: WorkflowInput) => validateWorkflow(input),
  });
}
