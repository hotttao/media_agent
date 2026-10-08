/**
 * WorkflowApp - reusable app unit for media_workflow (standalone + plugin).
 * One panel, resource Tabs switches Template / Workflow / Job. Job is
 * submit+monitor (no edit); Template/Workflow have full CRUD + Validate.
 */

import { Plus } from "lucide-react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
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

const RESOURCES: { id: Resource; labelKey: string }[] = [
  { id: "workflow", labelKey: "nav.workflows" },
  { id: "template", labelKey: "nav.templates" },
  { id: "job", labelKey: "nav.jobs" },
];

export function WorkflowApp({ instanceId }: { instanceId: string }) {
  const { t } = useTranslation();
  const {
    useAppHeaderSlot,
    AppHeaderButton,
    useInstancePersistentState,
    useInstanceState,
  } = hostShim;
  const headerSlot = useAppHeaderSlot();
  const [resource, setResource] = useInstancePersistentState<Resource>(
    instanceId,
    "workflow_resource",
    () => "workflow",
  );
  const [selectedId, setSelectedId] = useInstancePersistentState<string | null>(
    instanceId,
    "workflow_selectedId",
    () => null,
  );
  const [view, setView] = useInstanceState<View>(
    instanceId,
    "workflow_view",
    () => "list",
  );
  const [launchWorkflowID, setLaunchWorkflowID] = useInstanceState<
    string | undefined
  >(instanceId, "workflow_launchId", () => undefined);

  function create() {
    setLaunchWorkflowID(undefined);
    setSelectedId(null);
    setView("create");
  }

  function changeResource(r: Resource) {
    setLaunchWorkflowID(undefined);
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
        view === "list" &&
        createPortal(
          <span className="media-workflow-ui">
            <span className="media-header-actions">
              <AppHeaderButton
                icon={Plus}
                label={t("common.new")}
                title={t("common.new")}
                onClick={create}
              />
            </span>
          </span>,
          headerSlot,
        )}
      <div className="media-app media-surface flex h-full min-h-0 flex-col">
        {!(resource === "workflow" && view !== "list") && (
          <div className="media-subnav">
            <Tabs
              value={resource}
              onValueChange={(v) => changeResource(v as Resource)}
            >
              <TabsList>
                {RESOURCES.map((r) => (
                  <TabsTrigger key={r.id} value={r.id}>
                    {t(r.labelKey)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </div>
        )}
        <div className="min-h-0 flex-1">
          {resource === "template" && view === "list" && (
            <TemplateList
              instanceId={instanceId}
              onSelect={select}
              onCreate={create}
            />
          )}
          {resource === "template" && view === "show" && selectedId && (
            <TemplateShow
              id={selectedId}
              onBack={() => setView("list")}
              onEdit={() => setView("edit")}
            />
          )}
          {resource === "template" && view === "create" && (
            <TemplateCreate onDone={() => setView("list")} />
          )}
          {resource === "template" && view === "edit" && selectedId && (
            <TemplateEdit id={selectedId} onDone={() => setView("show")} />
          )}

          {resource === "workflow" && view === "list" && (
            <WorkflowList
              instanceId={instanceId}
              onSelect={select}
              onCreate={() => setView("create")}
            />
          )}
          {resource === "workflow" && view === "show" && selectedId && (
            <WorkflowShow
              id={selectedId}
              onBack={() => setView("list")}
              onEdit={() => setView("edit")}
              onOpenJob={(jobId) => {
                setResource("job");
                setSelectedId(jobId);
                setView("show");
              }}
              onRun={() => {
                setLaunchWorkflowID(selectedId);
                setResource("job");
                setSelectedId(null);
                setView("create");
              }}
            />
          )}
          {resource === "workflow" && view === "create" && (
            <WorkflowCreate
              instanceId={instanceId}
              onDone={(id) => {
                if (id) {
                  setSelectedId(id);
                  setView("show");
                } else setView("list");
              }}
            />
          )}
          {resource === "workflow" && view === "edit" && selectedId && (
            <WorkflowEdit
              key={selectedId}
              instanceId={instanceId}
              id={selectedId}
              onDone={() => setView("show")}
            />
          )}

          {resource === "job" && view === "list" && (
            <JobList
              instanceId={instanceId}
              onSelect={select}
              onCreate={create}
            />
          )}
          {resource === "job" && view === "show" && selectedId && (
            <JobShow id={selectedId} onBack={() => setView("list")} />
          )}
          {resource === "job" && view === "create" && (
            <JobCreate
              preferredWorkflowId={launchWorkflowID}
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
