/**
 * WorkflowApp - reusable app unit for media_workflow (standalone + plugin).
 * One panel, resource Tabs switches Template / Workflow / Job. Job is
 * submit+monitor (no edit); Template/Workflow have full CRUD + Validate.
 */
import { createPortal } from "react-dom";
import { Plus } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hostShim } from "@/foundation/lib/host-shim";
import { JobCreate } from "./job/create";
import { JobList } from "./job/list";
import { JobShow } from "./job/show";
import { TemplateCreate } from "./template/create";
import { TemplateEdit } from "./template/edit";
import { TemplateList } from "./template/list";
import { TemplateShow } from "./template/show";
import { WorkflowCreate } from "./workflow/create";
import { WorkflowEdit } from "./workflow/edit";
import { WorkflowList } from "./workflow/list";
import { WorkflowShow } from "./workflow/show";

type Resource = "template" | "workflow" | "job";
type View = "list" | "show" | "create" | "edit";

const RESOURCES: { id: Resource; label: string }[] = [
  { id: "template", label: "Templates" },
  { id: "workflow", label: "Workflows" },
  { id: "job", label: "Jobs" },
];

export function WorkflowApp({ instanceId }: { instanceId: string }) {
  const { useAppHeaderSlot, AppHeaderButton, useInstancePersistentState, useInstanceState } =
    hostShim;
  const headerSlot = useAppHeaderSlot();
  const [resource, setResource] = useInstancePersistentState<Resource>(
    instanceId,
    "resource",
    () => "template",
  );
  const [selectedId, setSelectedId] = useInstancePersistentState<string | null>(
    instanceId,
    "selectedId",
    () => null,
  );
  const [view, setView] = useInstanceState<View>(instanceId, "view", () => "list");

  function changeResource(r: Resource) {
    setResource(r);
    setSelectedId(null);
    setView("list");
  }

  function select(id: string) {
    setSelectedId(id);
    setView("show");
  }

  return (
    <>
      {headerSlot &&
        createPortal(
          <AppHeaderButton
            icon={Plus}
            label="New"
            title="New"
            onClick={() => {
              setSelectedId(null);
              setView("create");
            }}
          />,
          headerSlot,
        )}
      <div className="flex h-full flex-col">
        <div className="border-b border-foreground/[0.06] px-3 py-2">
          <Tabs value={resource} onValueChange={(v) => changeResource(v as Resource)}>
            <TabsList>
              {RESOURCES.map((r) => (
                <TabsTrigger key={r.id} value={r.id}>
                  {r.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
        <div className="min-h-0 flex-1">
          {resource === "template" && view === "list" && <TemplateList onSelect={select} />}
          {resource === "template" && view === "show" && selectedId && (
            <TemplateShow id={selectedId} onBack={() => setView("list")} onEdit={() => setView("edit")} />
          )}
          {resource === "template" && view === "create" && (
            <TemplateCreate onDone={() => setView("list")} />
          )}
          {resource === "template" && view === "edit" && selectedId && (
            <TemplateEdit id={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "workflow" && view === "list" && <WorkflowList onSelect={select} />}
          {resource === "workflow" && view === "show" && selectedId && (
            <WorkflowShow id={selectedId} onBack={() => setView("list")} onEdit={() => setView("edit")} />
          )}
          {resource === "workflow" && view === "create" && (
            <WorkflowCreate onDone={() => setView("list")} />
          )}
          {resource === "workflow" && view === "edit" && selectedId && (
            <WorkflowEdit id={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "job" && view === "list" && <JobList onSelect={select} />}
          {resource === "job" && view === "show" && selectedId && (
            <JobShow id={selectedId} onBack={() => setView("list")} />
          )}
          {resource === "job" && view === "create" && (
            <JobCreate
              onDone={() => setView("list")}
              onSubmitted={(jobId) => {
                setSelectedId(jobId);
                setView("show");
              }}
            />
          )}
        </div>
      </div>
    </>
  );
}
