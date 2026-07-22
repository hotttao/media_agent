import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig, type UserConfig } from "vite";

const analyze = process.env.ANALYZE === "true";

// Two build targets (see docs/plan/ui_workflow.md §4.5):
//  - standalone (default): SPA served from index.html -> src/main.tsx
//  - plugin (VITE_TARGET=plugin): IIFE bundle for agent-platform host injection.
//    React/ReactDOM/jsx-runtime are externalized to window.tos (rules of hooks
//    require a single React instance). react-router/react-query/react-i18next
//    are bundled (v7 vs host v6 not shareable; query/i18n use plugin-local
//    Provider per §4.3 方案 C).
const target = process.env.VITE_TARGET ?? "standalone";

// Dev proxy: route /api to the local materials/workflow gateway (or a service
// port). Override per local env with VITE_DEV_API_TARGET. The /api/v1/auth/*
// OAuth endpoints live on materials/workflow (shared auth middleware, same
// JWT_SECRET + mw_token cookie domain).
const DEV_API_TARGET = process.env.VITE_DEV_API_TARGET ?? "http://127.0.0.1:8002";

const plugins = [react(), ...(analyze ? [visualizer({ filename: "stats.html", sourcemap: true })] : [])];
const alias = { "@": resolve(__dirname, "./src") };

const pluginConfig: UserConfig = {
  plugins,
  worker: { format: "es" },
  resolve: { alias },
  build: {
    lib: {
      entry: resolve(__dirname, "src/plugin.tsx"),
      formats: ["iife"],
      name: "MediaPlugin",
      fileName: () => "media-plugin.js",
    },
    rollupOptions: {
      external: ["react", "react-dom", "react-dom/client", "react/jsx-runtime"],
      output: {
        globals: {
          react: "window.tos.React",
          "react-dom": "window.tos.ReactDOM",
          "react-dom/client": "window.tos.ReactDOM",
          "react/jsx-runtime": "window.tos.ReactJSXRuntime",
        },
      },
    },
    sourcemap: analyze,
    chunkSizeWarningLimit: 1024,
  },
};

const standaloneConfig: UserConfig = {
  plugins,
  worker: { format: "es" },
  resolve: { alias },
  build: {
    sourcemap: analyze,
    chunkSizeWarningLimit: 1024,
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      "/api": {
        target: DEV_API_TARGET,
        changeOrigin: true,
      },
    },
  },
};

export default defineConfig(target === "plugin" ? pluginConfig : standaloneConfig);
