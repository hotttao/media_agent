import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig } from "vite";

const analyze = process.env.ANALYZE === "true";

export default defineConfig({
  base: process.env.AGENT_UI_BASE || "/",
  plugins: [
    react(),
    analyze && visualizer({ filename: "stats.html", sourcemap: true }),
  ],
  // Emit web workers as ES modules so code-splitting works
  // (Rollup rejects the default iife format for code-split builds).
  worker: {
    format: "es",
  },
  build: {
    sourcemap: analyze,
    chunkSizeWarningLimit: 1024,
  },
  server: {
    proxy: {
      "^/api/v1/workspaces/[^/]+/mcp-bindings": {
        target: "http://127.0.0.1:8011",
        changeOrigin: true,
      },
      "/api/v1/prompts": {
        target: "http://127.0.0.1:8007",
        changeOrigin: true,
      },
      "/api/v1/skills": { target: "http://127.0.0.1:8007", changeOrigin: true },
      "/api/v1/mcp-definitions": {
        target: "http://127.0.0.1:8006",
        changeOrigin: true,
      },
      "/api/v1/workspaces": {
        target: "http://127.0.0.1:8007",
        changeOrigin: true,
      },
      "/api/chat": {
        target: "http://127.0.0.1:8006",
        changeOrigin: true,
      },
      "/api": {
        target: "http://127.0.0.1:8001",
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
});
