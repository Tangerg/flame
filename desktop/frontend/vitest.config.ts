/// <reference types="vitest" />
import { defineConfig } from "vitest/config";
import path from "node:path";
import { stylexBabel } from "./stylex.vite.mjs";

// We don't extend vite.config.ts here because that config pulls in the Wails
// runtime + several browser-only plugins; tests run in happy-dom and only
// need the path alias.
export default defineConfig({
  // Same transform the app and the fixtures get: without it `stylex.defineVars` reaches the
  // test runtime uncompiled and throws at import, taking every file that touches a migrated
  // component with it.
  plugins: [stylexBabel()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  test: {
    environment: "happy-dom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    // `visual/` holds the fixtures the goldens are taken from. Playwright proves what they
    // LOOK like; whether the Runtime could have sent them is a pure assertion, and it has no
    // business costing a browser.
    include: ["src/**/*.test.{ts,tsx}", "visual/**/*.test.ts"],
  },
});
