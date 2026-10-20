/// <reference types="vitest" />
import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { configDefaults, defineConfig } from "vitest/config";
import path from "node:path";
import { stylexTransform } from "./stylex.vite.mjs";

const runtimeLifecycleTest = "tests/integration/runtime-http.e2e.test.ts";

// We don't extend vite.config.ts here because that config pulls in the Wails
// runtime + several browser-only plugins; tests run in happy-dom and only
// need the path alias.
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
    environment: "happy-dom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    projects: [
      {
        extends: true,
        test: {
          name: "frontend",
          // Fixture validity and portable component contracts do not require a browser;
          // Playwright owns rendered geometry and appearance.
          include: [
            "src/**/*.test.{ts,tsx}",
            "visual/**/*.test.{ts,tsx}",
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
          include: [runtimeLifecycleTest],
          sequence: { groupOrder: 1 },
        },
      },
    ],
  },
});
