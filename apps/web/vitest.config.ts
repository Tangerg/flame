/// <reference types="vitest" />
import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { configDefaults, defineConfig } from "vitest/config";
import path from "node:path";
import { STYLEX_CONDITIONS, stylexTransform } from "./stylex.vite.mjs";

const runtimeLifecycleTest = "tests/integration/runtime-http.e2e.test.ts";

// The application config starts Wails/browser lifetimes that neither test host owns.
export default defineConfig({
  // Same transform the app and the fixtures get: without it `stylex.defineVars` reaches the
  // test runtime uncompiled and throws when logic tests import those modules.
  plugins: [stylexTransform()],
  resolve: {
    conditions: STYLEX_CONDITIONS,
    dedupe: sharedRuntimePackages,
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  test: {
    globals: true,
    projects: [
      {
        extends: true,
        test: {
          name: "contracts",
          environment: "node",
          include: ["tests/integration/**/*.test.ts"],
          exclude: [...configDefaults.exclude, runtimeLifecycleTest],
          sequence: { groupOrder: 0 },
        },
      },
      {
        extends: true,
        test: {
          name: "runtime",
          environment: "node",
          include: [runtimeLifecycleTest],
          globalSetup: ["./tests/integration/runtime-http.setup.ts"],
          sequence: { groupOrder: 1 },
        },
      },
    ],
  },
});
