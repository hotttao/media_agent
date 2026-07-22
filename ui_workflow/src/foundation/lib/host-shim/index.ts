import { pluginShim } from "./plugin";
import { standaloneShim } from "./standalone";
import type { HostShim } from "./types";

/**
 * Active host shim. Build-time selected via VITE_TARGET so the unused branch
 * is dead-code-eliminated:
 *  - standalone build drops plugin/tos deps (getTos, window.tos)
 *  - plugin build drops shell/auth-provider deps (AppWindow, AuthProvider)
 *
 * VITE_TARGET is declared in vite-env.d.ts.
 */
export const hostShim: HostShim =
  import.meta.env.VITE_TARGET === "plugin" ? pluginShim : standaloneShim;

export type { HostShim } from "./types";
