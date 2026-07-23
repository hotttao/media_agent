import { useQuery } from "@tanstack/react-query";
import { listTemplates } from "@/domains/templates/lib/api";
import type { TemplateListParams } from "@/domains/templates/types";

export function useTemplatesList(params: TemplateListParams = {}) {
  return useQuery({
    queryKey: ["templates", params],
    queryFn: () => listTemplates(params),
  });
}
