/// <reference types="vitest" />
import { configDefaults, defineConfig } from "vitest/config";
import path from "node:path";
import { stylexBabel } from "./stylex.vite.mjs";

const runtimeLifecycleTest = "src/rpc/runtime-http.e2e.test.ts";

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
    projects: [
      {
        extends: true,
        test: {
          name: "frontend",
          // `visual/` holds the fixtures the goldens are taken from. Playwright proves what they
          // LOOK like; whether the Runtime could have sent them is a pure assertion, and it has no
          // business costing a browser.
          include: ["src/**/*.test.{ts,tsx}", "visual/**/*.test.ts"],
          exclude: [...configDefaults.exclude, runtimeLifecycleTest],
          sequence: { groupOrder: 0 },
        },
      },
      {
        extends: true,
        test: {
          name: "runtime",
          include: [runtimeLifecycleTest],
          sequence: { groupOrder: 1 },
        },
      },
    ],
  },
});
