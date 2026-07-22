import type { ComponentType } from "react";

/**
 * An app is a self-contained UI unit hosted by a layout slot. Apps don't know
 * their position; SlotContext binds them to slots.
 *
 * Component must be a stable reference (module-level export). Per-instance
 * state (view mode, selection, drafts) lives in the instance state store keyed
 * by `instanceId`, NOT in component-local useState - that way slot moves /
 * fill-mode toggles don't reset the user's view.
 *
 * Adapted from agent-platform/web/src/lib/app-registry.ts.
 */
export interface AppDefinition {
  id: string;
  label: string;
  Component: ComponentType<AppComponentProps>;
  /** Greyed in dock and inert when picked. */
  disabled?: boolean;
  /** Picker grouping. Defaults to 'tool' when omitted. */
  group?: "agent" | "tool" | "capability" | "connection" | "extension" | "system";
  /** Hidden apps don't appear in dock/picker - only created programmatically. */
  hidden?: boolean;
  /** Optional small badge next to the label (e.g. "alpha"). */
  badge?: string;
  /** Per-instance tab label derived from persistent state. */
  instanceLabel?: (persistent: Record<string, unknown>) => string | null;
}

export interface AppComponentProps {
  /** Stable id assigned when the instance is created. */
  instanceId: string;
}

/** One opened-app cell inside a slot. */
export interface AppInstance {
  id: string;
  appId: string;
}
