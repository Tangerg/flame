/// <reference types="vitest" />
import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { configDefaults, defineConfig } from "vitest/config";
import path from "node:path";
import { stylexTransform } from "./stylex.vite.mjs";

const runtimeLifecycleTest = "tests/integration/runtime-http.e2e.test.ts";

// The application config starts Wails/browser lifetimes that neither test host owns.
export default defineConfig({
  // Same transform the app and the fixtures get: without it `stylex.defineVars` reaches the
  // test runtime uncompiled and throws at import, taking every file that touches a migrated
  // component with it.
  plugins: [stylexTransform()],
  resolve: {
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
          name: "frontend",
          environment: "happy-dom",
          setupFiles: ["./src/test/setup.ts"],
          fileParallelism: false,
          // Fixture validity and portable component contracts do not require a browser;
          // Playwright owns rendered geometry and appearance.
          include: [
            "src/**/*.test.{ts,tsx}",
            "lab/**/*.test.{ts,tsx}",
            "tests/integration/**/*.test.{ts,tsx}",
          ],
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
