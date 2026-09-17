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
const gatewayTarget = process.env.VITE_GATEWAY_URL ?? "http://192.168.2.41:18080";

const plugins = [
  react(),
  ...(analyze ? [visualizer({ filename: "stats.html", sourcemap: true })] : []),
];
const alias = { "@": resolve(__dirname, "./src") };

const pluginConfig: UserConfig = {
  plugins,
  publicDir: false,
  worker: { format: "es" },
  resolve: { alias },
  build: {
    outDir: "dist/plugin",
    lib: {
      entry: resolve(__dirname, "src/plugin.tsx"),
      formats: ["iife"],
      name: "MediaWorkflowPlugin",
      fileName: () => "media-workflow.js",
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
    outDir: "dist/standalone",
    sourcemap: analyze,
    chunkSizeWarningLimit: 1024,
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      // media_materials (8003): materials resources CRUD
      "/api/v1/materials": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/v1/products": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/v1/virtual-ips": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/v1/asset-kinds": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/v1/generated-assets": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      // media_workflow (8002): workflow/template/job + auth (OAuth via media_auth 8001)
      "/api/v1/workflows": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/v1/templates": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/v1/jobs": { target: gatewayTarget, changeOrigin: true },
      "/api/v1/step-runs": {
        target: gatewayTarget,
        changeOrigin: true,
      },
      "/api/auth": { target: gatewayTarget, changeOrigin: true },
    },
  },
};

export default defineConfig(
  target === "plugin" ? pluginConfig : standaloneConfig,
);
