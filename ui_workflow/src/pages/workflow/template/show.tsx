import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTemplate } from "@/domains/templates/hooks/use-template";
import {
  useArchiveTemplate,
  useChangeTemplateVisibility,
} from "@/domains/templates/hooks/use-template-mutations";
import { getTemplateVersion } from "@/domains/templates/lib/api";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import type { Template, Visibility } from "@/domains/templates/types";

const VISIBILITY_OPTIONS: Visibility[] = ["private", "team", "public"];

export function TemplateShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: template, isLoading } = useTemplate(id);
  const archiveTemplate = useArchiveTemplate();
  const changeVisibility = useChangeTemplateVisibility();
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);

  // Fetch the historical snapshot only when a non-current version is selected.
  const { data: historicalVersion, isLoading: versionLoading } = useQuery({
    queryKey: ["template-version", id, selectedVersion],
    queryFn: () => getTemplateVersion(id, selectedVersion as number),
    enabled: selectedVersion !== null,
  });

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!template) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  const currentVersion = template.version;
  const versionNumbers = Array.from({ length: currentVersion }, (_, i) => currentVersion - i);
  const showingHistorical = selectedVersion !== null && selectedVersion !== currentVersion;
  const displayed: Template = showingHistorical && historicalVersion ? historicalVersion : template;

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
        <div className="flex gap-2">
          {template.status !== "archived" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => archiveTemplate.mutate(id)}
              disabled={archiveTemplate.isPending}
            >
              Archive
            </Button>
          ) : null}
          <Button size="sm" onClick={onEdit}>
            Edit
          </Button>
        </div>
      </div>

      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="col-span-2 font-medium">{template.name}</dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{template.scope}</dd>
        <dt className="text-muted-foreground">Status</dt>
        <dd className="col-span-2">{template.status}</dd>
        <dt className="text-muted-foreground">Visibility</dt>
        <dd className="col-span-2">
          <div className="flex items-center gap-2">
            <VisibilityBadge visibility={template.visibility} />
            <Select
              value={template.visibility}
              onValueChange={(v) =>
                changeVisibility.mutate({ id, input: { visibility: v as Visibility } })
              }
            >
              <SelectTrigger className="h-7 w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {VISIBILITY_OPTIONS.map((v) => (
                  <SelectItem key={v} value={v}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </dd>
        <dt className="text-muted-foreground">Version</dt>
        <dd className="col-span-2">
          <div className="flex items-center gap-2">
            <span>v{currentVersion}</span>
            {versionNumbers.length > 1 ? (
              <Select
                value={selectedVersion === null ? String(currentVersion) : String(selectedVersion)}
                onValueChange={(v) => setSelectedVersion(Number(v))}
              >
                <SelectTrigger className="h-7 w-32">
                  <SelectValue placeholder="View history" />
                </SelectTrigger>
                <SelectContent>
                  {versionNumbers.map((v) => (
                    <SelectItem key={v} value={String(v)}>
                      v{v}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
            {showingHistorical ? (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSelectedVersion(null)}
              >
                Back to current
              </Button>
            ) : null}
          </div>
        </dd>
      </dl>

      {showingHistorical && versionLoading ? (
        <div className="flex items-center justify-center py-4">
          <Spinner className="h-5 w-5" />
        </div>
      ) : null}

      {showingHistorical ? (
        <p className="text-xs text-muted-foreground">
          Viewing historical snapshot v{selectedVersion}.
        </p>
      ) : null}

      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Description</dt>
        <dd className="col-span-2">{displayed.description || "-"}</dd>
        <dt className="text-muted-foreground">Prompt Body</dt>
        <dd className="col-span-2">
          <pre className="max-h-80 overflow-auto rounded bg-muted p-2 text-xs whitespace-pre-wrap break-all">
            {displayed.prompt_body}
          </pre>
        </dd>
      </dl>

      <div className="grid gap-2 text-sm">
        <p className="text-xs uppercase text-muted-foreground">Inputs</p>
        {displayed.inputs.length === 0 ? (
          <p className="text-xs text-muted-foreground">No input slots.</p>
        ) : (
          <table className="w-full text-sm">
            <thead className="border-b text-left text-xs uppercase text-muted-foreground">
              <tr>
                <th className="px-2 py-1">Name</th>
                <th className="px-2 py-1">Resource Type</th>
                <th className="px-2 py-1">Required</th>
              </tr>
            </thead>
            <tbody>
              {displayed.inputs.map((s, i) => (
                <tr key={i} className="border-b">
                  <td className="px-2 py-1 font-mono">{s.name}</td>
                  <td className="px-2 py-1 text-muted-foreground">{s.resource_type}</td>
                  <td className="px-2 py-1 text-muted-foreground">
                    {s.required ? "yes" : "no"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Output Asset Kind</dt>
        <dd className="col-span-2">{displayed.output_asset_kind}</dd>
        <dt className="text-muted-foreground">Platform</dt>
        <dd className="col-span-2">{displayed.platform}</dd>
        <dt className="text-muted-foreground">Gen Way</dt>
        <dd className="col-span-2">{displayed.gen_way}</dd>
        <dt className="text-muted-foreground">Gen Type</dt>
        <dd className="col-span-2">{displayed.gen_type || "-"}</dd>
        <dt className="text-muted-foreground">Needs Confirm</dt>
        <dd className="col-span-2">{displayed.needs_confirm ? "yes" : "no"}</dd>
        <dt className="text-muted-foreground">Candidate Count</dt>
        <dd className="col-span-2">{displayed.candidate_count}</dd>
      </dl>

      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Created By</dt>
        <dd className="col-span-2">{template.created_by}</dd>
        <dt className="text-muted-foreground">Created At</dt>
        <dd className="col-span-2">{template.created_at}</dd>
        <dt className="text-muted-foreground">Updated At</dt>
        <dd className="col-span-2">{template.updated_at}</dd>
      </dl>
    </div>
  );
}
