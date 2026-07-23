import { useTemplatesList } from "@/domains/templates/hooks/use-templates-list";
import { VisibilityBadge } from "@/domains/materials/components/visibility-badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";

export function TemplateList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading } = useTemplatesList();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  const items = data?.items ?? [];
  if (items.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        No templates yet.
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-card">
          <tr className="border-b text-left text-xs uppercase text-muted-foreground">
            <th className="px-3 py-2">Name</th>
            <th className="px-3 py-2">Scope</th>
            <th className="px-3 py-2">Platform</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2">Visibility</th>
            <th className="px-3 py-2">Version</th>
            <th className="px-3 py-2">Updated</th>
          </tr>
        </thead>
        <tbody>
          {items.map((t) => (
            <tr key={t.id} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(t.id)}
                >
                  {t.name}
                </button>
              </td>
              <td className="px-3 py-2 text-muted-foreground">{t.scope}</td>
              <td className="px-3 py-2 text-muted-foreground">{t.platform}</td>
              <td className="px-3 py-2 text-muted-foreground">{t.status}</td>
              <td className="px-3 py-2">
                <VisibilityBadge visibility={t.visibility} />
              </td>
              <td className="px-3 py-2 text-muted-foreground">v{t.version}</td>
              <td className="px-3 py-2 text-muted-foreground">{t.updated_at}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </ScrollArea>
  );
}
