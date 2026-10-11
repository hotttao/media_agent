export const WORKFLOW_LAUNCH_KEY = "host_workflow_launch_intent";
export const PRODUCT_OPEN_KEY = "host_product_open_intent";

export interface ProductOpenIntent {
  organizationId: string;
  productId: string;
  requestId: string;
}

/** UI intent only. No credentials, input snapshots or execution command. */
export interface WorkflowLaunchIntent {
  kind: "workflow_run";
  requestId: string;
  organizationId: string;
  workflowId: string;
  productId: string;
}

export interface AppNavigationPort {
  openProduct?(request: Omit<ProductOpenIntent, "requestId">): void;
  openWorkflowRun(
    request: Omit<WorkflowLaunchIntent, "kind" | "requestId">,
  ): void;
}
