import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig, type UserConfig } from "vite";

const analyze = process.env.ANALYZE === "true";
const target = process.env.VITE_TARGET ?? "standalone";
const plugins = [
  react(),
  ...(analyze ? [visualizer({ filename: "stats.html", sourcemap: true })] : []),
];
const common: UserConfig = {
  plugins,
  resolve: { alias: { "@": resolve(__dirname, "./src") } },
};

const standalone: UserConfig = {
  ...common,
  build: {
    outDir: "dist/standalone",
    sourcemap: analyze,
    chunkSizeWarningLimit: 1024,
  },
};

const plugin: UserConfig = {
  ...common,
  publicDir: false,
  build: {
    outDir: "dist/plugin",
    lib: {
      entry: resolve(__dirname, "src/plugin.tsx"),
      formats: ["iife"],
      name: "MediaMaterialsPlugin",
      fileName: () => "media-materials.js",
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

export default defineConfig(target === "plugin" ? plugin : standalone);
