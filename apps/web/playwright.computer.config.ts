import { defineConfig } from "@playwright/test";
import production from "./playwright.perf.config";

export default defineConfig({
  ...production,
  testMatch: "**/computerConsole.browser.spec.ts",
  use: { ...production.use, reducedMotion: "reduce" },
});
