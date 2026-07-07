import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import { defineConfig } from "vite";

const analyze = process.env.ANALYZE === "true";

export default defineConfig({
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
  resolve: {
    alias: {
      "@": resolve(__dirname, "./src"),
    },
  },
});
