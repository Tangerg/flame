import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { stylexBabel } from "./stylex.vite.mjs";
import path from "node:path";

/**
 * Test-only visual fixture entry.
 *
 * It deliberately lives outside the production router and Wails bootstrap:
 * fixtures may freeze clocks, content, appearance, and viewport without adding
 * a debug branch to the shipped application. They still import the production
 * CSS and components, so a screenshot exercises the same visual implementation.
 */
export default defineConfig({
  plugins: [react(), stylexBabel(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    host: "127.0.0.1",
    port: 4174,
    strictPort: true,
  },
});
