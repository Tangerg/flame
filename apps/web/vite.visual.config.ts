import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { stylexPlugin } from "./stylex.vite.mjs";
import path from "node:path";

/**
 * Test-only visual fixture entry.
 *
 * It deliberately lives outside the production router and Wails bootstrap:
 * fixtures may freeze clocks, content, appearance, and viewport without adding
 * a debug branch to the shipped application. They still import the production
 * CSS and components for behavioral checks and manual visual review.
 */
export default defineConfig({
  plugins: [react(), stylexPlugin()],
  resolve: {
    dedupe: sharedRuntimePackages,
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    host: "127.0.0.1",
    port: 4174,
    strictPort: true,
  },
  build: {
    outDir: "../../out/web/visual",
    emptyOutDir: true,
    rollupOptions: { input: path.resolve(import.meta.dirname, "visual/index.html") },
  },
  preview: {
    host: "127.0.0.1",
    port: 4175,
    strictPort: true,
  },
});
