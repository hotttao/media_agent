/**
 * window.tos host surface types + registerPanel helper for plugin mode.
 *
 * Adapted from agent-platform/web/src/lib/host.ts:41-73 (NapHost interface)
 * and lib/panel-registry.ts (PluginPanel). Trimmed to what ui_workflow's plugin
 * bundle consumes: React singletons, registerPanel, events (lang.change),
 * workspace instance-state hooks, windows header-slot hook + AppHeaderButton,
 * ui.useResolvedTheme. react-query / react-i18next are NOT exposed here - the
 * plugin carries its own copy and mounts a local Provider per
 * docs/plan/ui_workflow.md §4.3 方案 C.
 */
import type * as React from "react";

export interface PluginPanelProps {
  workspaceId: string;
  instanceId: string;
}

export interface PluginPanel {
  /** Must match `ui_panel` on an mcp_catalog entry for the tab to show. */
  id: string;
  /** Localized label, called every render so plugin i18n can react. */
  label: () => string;
  component: React.ComponentType<PluginPanelProps>;
}

export interface AppHeaderButtonProps {
  icon?: React.ComponentType<{ className?: string }>;
  label?: string;
  tone?: "default" | "destructive";
  onClick?: () => void;
  disabled?: boolean;
  title?: string;
  className?: string;
  [key: string]: unknown;
}

export interface InstanceStateSetter<T> {
  (next: T | ((prev: T) => T)): void;
}

export interface NapHost {
  version: string;
  React: typeof React;
  ReactDOM: typeof import("react-dom");
  ReactJSXRuntime: typeof import("react/jsx-runtime");
  registerPanel: (panel: PluginPanel) => void;
  registerPlugin: (plugin: unknown) => void;
  events: {
    on: (event: "lang.change", cb: (lng: string) => void) => () => void;
    emit: (event: string, ...args: unknown[]) => void;
  };
  ui: {
    useResolvedTheme: () => "light" | "dark";
  };
  workspace: {
    useSlotContext: () => unknown;
    useInstanceState: <T>(
      instanceId: string,
      key: string,
      defaultFn: () => T,
    ) => [T, InstanceStateSetter<T>];
    useInstancePersistentState: <T>(
      instanceId: string,
      key: string,
      defaultFn: () => T,
    ) => [T, InstanceStateSetter<T>];
  };
  windows: {
    useAppHeaderSlot: () => HTMLDivElement | null;
    AppHeaderButton: React.ComponentType<AppHeaderButtonProps>;
  };
}

declare global {
  interface Window {
    tos?: NapHost;
  }
}

/** True when running inside the agent-platform host (plugin mode). */
export function isPluginHost(): boolean {
  return typeof window !== "undefined" && !!window.tos;
}

/** Get the host surface; throws if accessed outside plugin mode. */
export function getTos(): NapHost {
  const tos = typeof window !== "undefined" ? window.tos : undefined;
  if (!tos) {
    throw new Error(
      "window.tos is not available - running outside agent-platform host (plugin mode only).",
    );
  }
  return tos;
}

/** Register a panel with the host. No-op outside plugin mode. */
export function registerPanel(panel: PluginPanel): void {
  if (isPluginHost()) {
    window.tos!.registerPanel(panel);
  }
}
