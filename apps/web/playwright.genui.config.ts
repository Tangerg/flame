import { defineConfig } from "@playwright/test";
import production from "./playwright.perf.config";

export default defineConfig({
  ...production,
  testMatch: ["**/genuiReading.browser.spec.ts", "**/genuiSandbox.browser.spec.ts"],
  use: { ...production.use, reducedMotion: "reduce" },
});
