export const WORKFLOW_LAUNCH_KEY = "host_workflow_launch_intent";

/** UI intent only. No credentials, input snapshots or execution command. */
export interface WorkflowLaunchIntent {
  kind: "workflow_run";
  requestId: string;
  organizationId: string;
  workflowId: string;
  productId: string;
}

export interface AppNavigationPort {
  openWorkflowRun(
    request: Omit<WorkflowLaunchIntent, "kind" | "requestId">,
  ): void;
}
