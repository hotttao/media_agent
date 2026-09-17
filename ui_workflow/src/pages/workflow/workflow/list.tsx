import { useArchiveWorkflow } from "@/domains/workflows/hooks/use-workflow-mutations";
import { useWorkflowsList } from "@/domains/workflows/hooks/use-workflows-list";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";

export function WorkflowList({ onSelect }: { onSelect: (id: string) => void }) {
  const { data, isLoading, isError, error } = useWorkflowsList();
  const archiveWorkflow = useArchiveWorkflow();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }

  if (isError) {
    return <div className="flex flex-1 items-center justify-center p-4 text-sm text-destructive">
      Unable to load workflows: {error instanceof Error ? error.message : "request failed"}
    </div>;
  }

  const items = data?.items ?? [];
  if (items.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        No workflows yet.
      </div>
    );
  }

  return (
    <ScrollArea className="flex-1">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-card">
          <tr className="border-b text-left text-xs uppercase text-muted-foreground">
            <th className="px-3 py-2">Name</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2">Version</th>
            <th className="px-3 py-2">Updated</th>
            <th className="px-3 py-2" aria-label="actions" />
          </tr>
        </thead>
        <tbody>
          {items.map((w) => (
            <tr key={w.id} className="border-b hover:bg-foreground/[0.04]">
              <td className="px-3 py-2">
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => onSelect(w.id)}
                >
                  {w.name}
                </button>
              </td>
              <td className="px-3 py-2 text-muted-foreground">{w.status}</td>
              <td className="px-3 py-2 text-muted-foreground">v{w.version}</td>
              <td className="px-3 py-2 text-muted-foreground">{w.updated_at}</td>
              <td className="px-3 py-2">
                {w.status !== "archived" && (
                  <Button variant="ghost" size="sm" onClick={() => archiveWorkflow.mutate(w.id)}>
                    Archive
                  </Button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </ScrollArea>
  );
}
