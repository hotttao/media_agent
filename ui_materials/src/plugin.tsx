import React, { type ComponentType } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import i18n, { syncHostLanguage } from "@/foundation/lib/i18n";
import {
  getHost,
  type PluginPanelProps,
  registerPanel,
} from "@/foundation/lib/plugin-host";
import App from "./App";
import pluginCss from "./index.css?inline";

const STYLE_ID = "media-materials-plugin-styles";

function installStyles(): void {
  if (document.getElementById(STYLE_ID)) return;
  const style = document.createElement("style");
  style.id = STYLE_ID;
  style.textContent = pluginCss;
  document.head.appendChild(style);
}

function PersistPluginPath({ instanceId }: { instanceId: string }) {
  const location = useLocation();
  const [, setPath] = getHost().workspace.useInstancePersistentState(
    instanceId,
    "materials.path",
    () => "/materials",
  );

  React.useEffect(() => {
    setPath(`${location.pathname}${location.search}${location.hash}`);
  }, [location.hash, location.pathname, location.search, setPath]);

  return null;
}

function MaterialsPanel({ instanceId }: PluginPanelProps) {
  const host = getHost();
  const [initialPath] = host.workspace.useInstancePersistentState(
    instanceId,
    "materials.path",
    () => "/materials",
  );
  const theme = host.ui.useResolvedTheme();

  return (
    <div className={theme === "dark" ? "dark h-full" : "h-full"}>
      <MemoryRouter initialEntries={[initialPath]}>
        <PersistPluginPath instanceId={instanceId} />
        <App />
      </MemoryRouter>
    </div>
  );
}

installStyles();
syncHostLanguage(getHost().events);

registerPanel({
  id: "media_materials",
  label: () => i18n.t("app.title"),
  component: MaterialsPanel as ComponentType<PluginPanelProps>,
});
