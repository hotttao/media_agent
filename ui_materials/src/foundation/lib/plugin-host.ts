import type * as React from "react";

export interface PluginPanelProps {
  workspaceId: string;
  instanceId: string;
}

interface PluginPanel {
  id: string;
  label: () => string;
  component: React.ComponentType<PluginPanelProps>;
}

type StateSetter<T> = (next: T | ((previous: T) => T)) => void;

export interface HostEvents {
  on: (
    event: "lang.change",
    callback: (language: string) => void,
  ) => () => void;
}

interface MaterialsPluginHost {
  version: string;
  React: typeof React;
  ReactDOM: typeof import("react-dom");
  ReactJSXRuntime: typeof import("react/jsx-runtime");
  registerPanel: (panel: PluginPanel) => void;
  events: HostEvents;
  ui: {
    useResolvedTheme: () => "light" | "dark";
  };
  workspace: {
    useInstancePersistentState: <T>(
      instanceId: string,
      key: string,
      defaultValue: () => T,
    ) => [T, StateSetter<T>];
  };
}

declare global {
  interface Window {
    tos?: MaterialsPluginHost;
  }
}

export function getHost(): MaterialsPluginHost {
  if (!window.tos) {
    throw new Error("media_materials plugin requires window.tos");
  }
  return window.tos;
}

export function registerPanel(panel: PluginPanel): void {
  getHost().registerPanel(panel);
}
