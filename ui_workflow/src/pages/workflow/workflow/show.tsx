import { useArchiveWorkflow } from "@/domains/workflows/hooks/use-workflow-mutations";
import { useWorkflow } from "@/domains/workflows/hooks/use-workflow";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

export function WorkflowShow({
  id,
  onBack,
  onEdit,
}: {
  id: string;
  onBack: () => void;
  onEdit: () => void;
}) {
  const { data: workflow, isLoading } = useWorkflow(id);
  const archiveWorkflow = useArchiveWorkflow();

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="h-5 w-5" />
      </div>
    );
  }
  if (!workflow) {
    return <div className="p-4 text-sm text-muted-foreground">Not found.</div>;
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back
        </Button>
        <Button size="sm" onClick={onEdit}>
          Edit
        </Button>
      </div>
      <dl className="grid grid-cols-3 gap-2 text-sm">
        <dt className="text-muted-foreground">Name</dt>
        <dd className="col-span-2 font-medium">{workflow.name}</dd>
        <dt className="text-muted-foreground">Status</dt>
        <dd className="col-span-2">{workflow.status}</dd>
        <dt className="text-muted-foreground">Version</dt>
        <dd className="col-span-2">v{workflow.version}</dd>
        <dt className="text-muted-foreground">Output Step</dt>
        <dd className="col-span-2">{workflow.output_step_key}</dd>
        <dt className="text-muted-foreground">Visibility</dt>
        <dd className="col-span-2">{workflow.visibility}</dd>
        <dt className="text-muted-foreground">Scope</dt>
        <dd className="col-span-2">{workflow.scope}</dd>
      </dl>
      <div className="grid gap-2">
        <div className="text-xs uppercase text-muted-foreground">Steps</div>
        <pre className="overflow-auto rounded bg-muted p-2 text-xs">
          {JSON.stringify(workflow.steps, null, 2)}
        </pre>
      </div>
      {workflow.status !== "archived" && (
        <Button
          variant="destructive"
          size="sm"
          onClick={() => {
            archiveWorkflow.mutate(id);
            onBack();
          }}
        >
          Archive
        </Button>
      )}
    </div>
  );
}
