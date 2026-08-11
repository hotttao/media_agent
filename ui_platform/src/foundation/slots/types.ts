import type { ComponentType } from "react";

export interface AppDefinition {
  id: string;
  label: string;
  Component: ComponentType<{ instanceId: string }>;
  allowMultiple?: boolean;
  hidden?: boolean;
}

export interface AppInstance {
  id: string;
  appId: string;
}

export interface SlotConfig {
  id: string;
  defaultOpened: string[];
}

export interface SlotState {
  opened: AppInstance[];
  activeInstanceId: string | null;
}

export interface SlotProfile {
  version: 2;
  layoutId: "1col" | "3col";
  activeSlotId: string;
  slots: Record<string, SlotState>;
  instances: Record<string, Record<string, unknown>>;
}

export interface ProfileBackend {
  load(workspaceId: string): unknown;
  save(workspaceId: string, profile: SlotProfile): void;
}
