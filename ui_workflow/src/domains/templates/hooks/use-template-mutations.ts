import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  archiveTemplate,
  changeTemplateVisibility,
  createTemplate,
  grantTemplate,
  revokeTemplate,
  updateTemplate,
} from "@/domains/templates/lib/api";
import type {
  ChangeVisibilityInput,
  GrantTemplateInput,
  TemplateInput,
} from "@/domains/templates/types";

export function useCreateTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: TemplateInput) => createTemplate(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["templates"] });
    },
  });
}

export function useUpdateTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<TemplateInput> }) =>
      updateTemplate(id, patch),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["template", id] });
    },
  });
}

export function useArchiveTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveTemplate(id),
    onSuccess: (template) => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["template", template.id] });
    },
  });
}

export function useGrantTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: GrantTemplateInput }) =>
      grantTemplate(id, input),
    onSuccess: (template) => {
      qc.invalidateQueries({ queryKey: ["template", template.id] });
    },
  });
}

export function useRevokeTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, teamId }: { id: string; teamId: string }) =>
      revokeTemplate(id, teamId),
    onSuccess: (template) => {
      qc.invalidateQueries({ queryKey: ["template", template.id] });
    },
  });
}

export function useChangeTemplateVisibility() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: ChangeVisibilityInput }) =>
      changeTemplateVisibility(id, input),
    onSuccess: (template) => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["template", template.id] });
    },
  });
}
